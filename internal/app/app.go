package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	assets "raikiri"
)

type Options struct {
	Host    string
	Port    int
	DataDir string
	Version string
	Logger  *slog.Logger
}

type App struct {
	opts     Options
	logger   *slog.Logger
	store    *Store
	hub      *Hub
	tts      *TTSEngine
	mu       sync.RWMutex
	cfg      AppConfig
	adapters []Adapter
	cancel   context.CancelFunc
}

func Serve(ctx context.Context, opts Options) error {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	store, err := OpenStore(opts.DataDir)
	if err != nil {
		return err
	}
	defer store.Close()

	cfg, err := store.LoadConfig(ctx)
	if err != nil {
		return err
	}
	app := &App{
		opts: opts, logger: opts.Logger, store: store, hub: NewHub(opts.Logger),
		cfg: cfg,
	}
	app.tts = NewTTSEngine(opts.Logger, func(audio AudioPayload) {
		app.hub.Publish("audio", "play_audio", audio)
	})

	mux := http.NewServeMux()
	app.routes(mux)
	server := &http.Server{
		Addr:              net.JoinHostPort(opts.Host, fmt.Sprint(opts.Port)),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	runCtx, cancel := context.WithCancel(ctx)
	app.cancel = cancel
	app.restartAdapters(runCtx)
	defer app.stopAdapters()

	go func() {
		<-ctx.Done()
		cancel()
		shutdownCtx, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		_ = server.Shutdown(shutdownCtx)
	}()

	opts.Logger.Info("raikiri native server listening", "url", "http://"+server.Addr, "dataDir", opts.DataDir)
	err = server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (a *App) routes(mux *http.ServeMux) {
	mux.HandleFunc("/healthz", a.handleHealth)
	mux.HandleFunc("/api/runtime", a.handleHealth)
	mux.HandleFunc("/api/config", a.handleConfig)
	mux.HandleFunc("/api/config/form", a.handleConfigForm)
	mux.HandleFunc("/api/auth/twitch/device-code", a.handleTwitchDeviceCode)
	mux.HandleFunc("/api/alerts/test", a.handleAlertTest)
	mux.HandleFunc("/api/tts/test", a.handleTTSTest)
	mux.HandleFunc("/api/chat/test", a.handleChatTest)
	mux.HandleFunc("/ws/chat", a.hub.Serve("chat"))
	mux.HandleFunc("/ws/alerts", a.hub.Serve("alerts"))
	mux.HandleFunc("/ws/audio", a.hub.Serve("audio"))
	mux.HandleFunc("/ws/events", a.hub.Serve("events"))
	mux.Handle("/media/", http.StripPrefix("/media/", http.FileServer(http.Dir(filepath.Join(a.opts.DataDir, "media")))))
	mux.Handle("/dashboard/", http.StripPrefix("/dashboard/", http.FileServer(http.FS(assets.DashboardAssets()))))
	mux.Handle("/shared/", http.StripPrefix("/shared/", http.FileServer(http.FS(assets.SharedAssets()))))
	mux.Handle("/overlay/chat/", http.StripPrefix("/overlay/chat/", http.FileServer(http.FS(assets.ChatAssets()))))
	mux.Handle("/overlay/alerts/", http.StripPrefix("/overlay/alerts/", http.FileServer(http.FS(assets.AlertsAssets()))))
	mux.Handle("/audio/", http.StripPrefix("/audio/", http.FileServer(http.FS(assets.AudioAssets()))))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/dashboard/", http.StatusFound)
			return
		}
		http.NotFound(w, r)
	})
}

func (a *App) config() AppConfig {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.cfg
}

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"ok": true, "version": a.opts.Version, "dataDir": a.opts.DataDir})
}

func (a *App) handleConfigForm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	patch := map[string]json.RawMessage{}
	for key, values := range r.PostForm {
		if len(values) == 0 {
			continue
		}
		value := values[len(values)-1]
		if key == "alertsConfig" {
			patch[key] = json.RawMessage(value)
			continue
		}
		switch key {
		case "ttsEnabled", "ttsRewardEnabled", "ttsCmdEnabled", "ttsCmdMod", "ttsCmdSub", "ttsCmdVip", "ttsCmdHost", "chatAnimations":
			if value == "true" || value == "on" || value == "1" {
				patch[key] = json.RawMessage("true")
			} else {
				patch[key] = json.RawMessage("false")
			}
		case "ttsMinBits", "ttsSubTier", "audioVolume", "chatFontSize", "chatHideAfter":
			patch[key] = json.RawMessage(value)
		default:
			encoded, _ := json.Marshal(value)
			patch[key] = encoded
		}
	}
	cfg := a.config()
	if err := applyConfigPatch(&cfg, patch); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.store.SaveConfig(r.Context(), cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.mu.Lock()
	a.cfg = cfg
	a.mu.Unlock()
	a.hub.Publish("chat", "config", map[string]any{
		"chatTheme": cfg.ChatTheme, "chatFontSize": cfg.ChatFontSize,
		"chatHideAfter": cfg.ChatHideAfter, "chatAnimations": cfg.ChatAnimations,
	})
	a.restartAdapters(r.Context())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte("Configuration Saved!"))
}

func (a *App) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, a.config())
	case http.MethodPost:
		var patch map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		cfg := a.config()
		if err := applyConfigPatch(&cfg, patch); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := a.store.SaveConfig(r.Context(), cfg); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		a.mu.Lock()
		a.cfg = cfg
		a.mu.Unlock()
		a.hub.Publish("chat", "config", map[string]any{
			"chatTheme": cfg.ChatTheme, "chatFontSize": cfg.ChatFontSize,
			"chatHideAfter": cfg.ChatHideAfter, "chatAnimations": cfg.ChatAnimations,
		})
		a.restartAdapters(r.Context())
		writeJSON(w, map[string]bool{"success": true})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *App) handleAlertTest(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Type string `json:"type"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Type == "" {
		body.Type = "follow"
	}
	evt := Event{Type: body.Type, Platform: PlatformTwitch, User: "TestUser", Amount: 100, Count: 5, Tier: 1, Message: "Este es un mensaje de prueba larguito.", GiftName: "Sub Tier 1"}
	a.routeEvent(evt)
	writeJSON(w, map[string]any{"success": true, "mocked": evt})
}

func (a *App) handleTTSTest(w http.ResponseWriter, r *http.Request) {
	a.tts.Enqueue(r.Context(), "Esta es una prueba de voz del sistema interactivo Raikiri.", a.config())
	writeJSON(w, map[string]bool{"success": true})
}

func (a *App) handleChatTest(w http.ResponseWriter, r *http.Request) {
	msg := ChatMessage{
		ID: fmt.Sprint(time.Now().UnixNano()), Platform: PlatformTwitch, User: "Rashpro0", DisplayName: "Rashpro0",
		Content: "¡Hola! Probando que el chat de Raikiri nativo funciona perfecto.", HTMLContent: "¡Hola! Probando que el chat de Raikiri nativo funciona perfecto.",
		Badges: []Badge{}, Timestamp: time.Now(),
	}
	a.routeChat(msg)
	writeJSON(w, map[string]bool{"success": true})
}

func (a *App) handleTwitchDeviceCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	cfg := a.config()
	if cfg.TwitchClientID == "" {
		http.Error(w, `{"error":"No Twitch Client ID configured"}`, http.StatusBadRequest)
		return
	}
	auth := NewTwitchAuth(a.store, cfg.TwitchClientID, a.logger)
	code, err := auth.RequestDeviceCode(r.Context(), []string{"chat:read", "channel:read:subscriptions", "channel:read:redemptions", "bits:read", "moderator:read:followers"})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	go func() {
		token, err := auth.PollForToken(context.Background(), code.DeviceCode, code.Interval)
		if err != nil {
			a.logger.Warn("twitch auth polling failed", "error", err)
			return
		}
		if username, err := auth.AuthenticatedUserName(context.Background(), token.AccessToken); err == nil && username != "" {
			cfg := a.config()
			cfg.TwitchChannel = username
			if err := a.store.SaveConfig(context.Background(), cfg); err == nil {
				a.mu.Lock()
				a.cfg = cfg
				a.mu.Unlock()
			}
		}
		a.restartAdapters(context.Background())
	}()
	writeJSON(w, code)
}

func (a *App) routeChat(msg ChatMessage) {
	a.hub.Publish("chat", "message", msg)
	cfg := a.config()
	if !cfg.TTSCmdEnabled || !strings.HasPrefix(strings.ToLower(strings.TrimSpace(msg.Content)), strings.ToLower(cfg.TTSCmdPrefix)) {
		return
	}
	text := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(msg.Content), cfg.TTSCmdPrefix))
	if text != "" {
		a.tts.Enqueue(context.Background(), msg.DisplayName+" dice: "+text, cfg)
	}
}

func (a *App) routeEvent(evt Event) {
	a.store.SaveEvent(context.Background(), evt)
	a.hub.Publish("alerts", "alert", evt)
	if evt.Type == "superchat" || evt.Type == "subscription" || evt.Type == "bits" || evt.Type == "gift" {
		a.tts.EnqueueEvent(context.Background(), evt, a.config())
	}
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func mergeConfig(old AppConfig, incoming AppConfig) AppConfig {
	if incoming.AlertsConfig == nil {
		incoming.AlertsConfig = old.AlertsConfig
	}
	if incoming.TTSVoice == "" {
		incoming.TTSVoice = old.TTSVoice
	}
	if incoming.AudioMode == "" {
		incoming.AudioMode = old.AudioMode
	}
	if incoming.ChatTheme == "" {
		incoming.ChatTheme = old.ChatTheme
	}
	if incoming.TTSCmdPrefix == "" {
		incoming.TTSCmdPrefix = old.TTSCmdPrefix
	}
	return incoming
}

func applyConfigPatch(cfg *AppConfig, patch map[string]json.RawMessage) error {
	set := func(key string, dest any) error {
		raw, ok := patch[key]
		if !ok {
			return nil
		}
		if len(raw) == 0 || string(raw) == "null" {
			return nil
		}
		return json.Unmarshal(raw, dest)
	}
	fields := []struct {
		key  string
		dest any
	}{
		{"twitchClientId", &cfg.TwitchClientID},
		{"twitchChannel", &cfg.TwitchChannel},
		{"youtubeChannelId", &cfg.YouTubeID},
		{"kickUsername", &cfg.KickUsername},
		{"tiktokUsername", &cfg.TikTokUsername},
		{"ttsEnabled", &cfg.TTSEnabled},
		{"ttsVoice", &cfg.TTSVoice},
		{"ttsMinBits", &cfg.TTSMinBits},
		{"ttsSubTier", &cfg.TTSSubTier},
		{"audioMode", &cfg.AudioMode},
		{"audioVolume", &cfg.AudioVol},
		{"ttsRewardEnabled", &cfg.TTSRewardEnabled},
		{"ttsRewardName", &cfg.TTSRewardName},
		{"ttsCmdEnabled", &cfg.TTSCmdEnabled},
		{"ttsCmdPrefix", &cfg.TTSCmdPrefix},
		{"ttsCmdMod", &cfg.TTSCmdMod},
		{"ttsCmdSub", &cfg.TTSCmdSub},
		{"ttsCmdVip", &cfg.TTSCmdVip},
		{"ttsCmdHost", &cfg.TTSCmdHost},
		{"chatTheme", &cfg.ChatTheme},
		{"chatFontSize", &cfg.ChatFontSize},
		{"chatHideAfter", &cfg.ChatHideAfter},
		{"chatAnimations", &cfg.ChatAnimations},
		{"alertsConfig", &cfg.AlertsConfig},
	}
	for _, field := range fields {
		if err := set(field.key, field.dest); err != nil {
			return fmt.Errorf("invalid %s: %w", field.key, err)
		}
	}
	return nil
}

func readAllAndClose(rc io.ReadCloser) []byte {
	defer rc.Close()
	b, _ := io.ReadAll(rc)
	return b
}

func ensureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}
