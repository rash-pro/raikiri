package app

import (
	"context"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"
	"sync"

	"raikiri/internal/steamach"
)

// DefaultAchievementsConfig enables the watcher with everything auto-detected
// and tails FFNx.log for FINAL FANTASY VIII (39150) and VII (39140) when installed.
func DefaultAchievementsConfig() AchievementsConfig {
	return AchievementsConfig{Enabled: true, FFNxApps: []uint32{39150, 39140}, TestAppID: 39150}
}

// AchievementsAdapter runs the Steam achievement watcher as an App adapter.
type AchievementsAdapter struct {
	cfg     AchievementsConfig
	logger  *slog.Logger
	publish func(steamach.Event)

	mu      sync.Mutex
	cancel  context.CancelFunc
	done    chan struct{}
	watcher *steamach.Watcher
}

func NewAchievementsAdapter(cfg AchievementsConfig, logger *slog.Logger, publish func(steamach.Event)) *AchievementsAdapter {
	return &AchievementsAdapter{cfg: cfg, logger: logger, publish: publish}
}

func (ad *AchievementsAdapter) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	defer close(done)

	probe := steamach.New(steamach.Config{SteamRoot: ad.cfg.SteamRoot, AccountID: ad.cfg.AccountID, Logger: ad.logger}, nil)
	if err := probe.Init(); err != nil {
		cancel()
		ad.logger.Warn("steam achievements disabled", "error", err)
		return
	}
	watcher := steamach.New(steamach.Config{
		SteamRoot: probe.Root(),
		AccountID: probe.Account(),
		FFNxLogs:  resolveFFNxLogs(probe.Root(), ad.cfg),
		Logger:    ad.logger,
	}, ad.publish)

	ad.mu.Lock()
	ad.cancel, ad.done, ad.watcher = cancel, done, watcher
	ad.mu.Unlock()

	if err := watcher.Run(ctx); err != nil {
		ad.logger.Warn("steam achievements watcher stopped", "error", err)
	}
}

func (ad *AchievementsAdapter) Stop() {
	ad.mu.Lock()
	cancel, done := ad.cancel, ad.done
	ad.cancel, ad.done = nil, nil
	ad.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

// Watcher returns the running watcher, or nil before Start resolved Steam.
func (ad *AchievementsAdapter) Watcher() *steamach.Watcher {
	if ad == nil {
		return nil
	}
	ad.mu.Lock()
	defer ad.mu.Unlock()
	return ad.watcher
}

// resolveFFNxLogs merges explicit log paths with <install dir>/FFNx.log for
// each configured FFNx app that is installed.
func resolveFFNxLogs(root string, cfg AchievementsConfig) map[string]uint32 {
	logs := map[string]uint32{}
	for path, appid := range cfg.FFNxLogs {
		if path != "" && appid != 0 {
			logs[path] = appid
		}
	}
	for _, appid := range cfg.FFNxApps {
		m, ok := steamach.FindManifest(root, appid)
		if !ok || m.InstallPath() == "" {
			continue
		}
		path := filepath.Join(m.InstallPath(), "FFNx.log")
		if _, taken := logs[path]; !taken {
			logs[path] = appid
		}
	}
	return logs
}

func (a *App) publishAchievement(ev steamach.Event) {
	a.logger.Info("achievement unlocked", "appid", ev.AppID, "game", ev.Game, "achievement", ev.APIName,
		"progress", strconv.Itoa(ev.Unlocked)+"/"+strconv.Itoa(ev.Total), "source", ev.Source)
	a.hub.Publish("widgets", "achievement_unlocked", ev)
}

// achievementsReader prefers the running watcher (warm caches) and otherwise
// reads Steam's files directly, so the API works even with the watcher disabled.
func (a *App) achievementsReader() *steamach.Watcher {
	a.mu.RLock()
	adapter := a.achievements
	a.mu.RUnlock()
	if w := adapter.Watcher(); w != nil {
		return w
	}
	cfg := a.config().Achievements
	return steamach.New(steamach.Config{SteamRoot: cfg.SteamRoot, AccountID: cfg.AccountID, Logger: a.logger}, nil)
}

// handleAchievements serves GET /api/achievements[?appid=]: one app's
// achievement list with unlock state, or every known app's counters.
func (a *App) handleAchievements(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	reader := a.achievementsReader()
	if raw := r.URL.Query().Get("appid"); raw != "" {
		appid, err := strconv.ParseUint(raw, 10, 32)
		if err != nil || appid == 0 {
			http.Error(w, "invalid appid", http.StatusBadRequest)
			return
		}
		status, err := reader.Status(uint32(appid))
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, status)
		return
	}
	apps, err := reader.Apps()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	if apps == nil {
		apps = []steamach.AppSummary{}
	}
	writeJSON(w, map[string]any{"root": reader.Root(), "account": reader.Account(), "apps": apps})
}

// handleAchievementsTest fires a realistic toast built from the real schema:
// POST /api/widgets/achievements/test?appid=39150&api=UNLOCK_GF_SHIVA (both optional).
func (a *App) handleAchievementsTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	_ = r.ParseForm()
	cfg := a.config().Achievements
	appid := uint64(cfg.TestAppID)
	if raw := r.FormValue("appid"); raw != "" {
		parsed, err := strconv.ParseUint(raw, 10, 32)
		if err != nil || parsed == 0 {
			http.Error(w, "invalid appid", http.StatusBadRequest)
			return
		}
		appid = parsed
	}
	if appid == 0 {
		appid = 39150
	}
	ev, err := a.achievementsReader().MakeEvent(uint32(appid), r.FormValue("api"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	a.publishAchievement(ev)
	writeJSON(w, map[string]any{"success": true, "event": ev, "widgetClients": a.hub.Count("widgets")})
}
