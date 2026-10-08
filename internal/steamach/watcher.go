package steamach

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Config tunes a Watcher. Zero values resolve to sensible defaults.
type Config struct {
	SteamRoot string            // default: DefaultRoot()
	AccountID uint32            // default: FindAccountID(root)
	FFNxLogs  map[string]uint32 // FFNx.log path -> appid, tailed for instant unlocks
	GameNames map[uint32]string // overrides for the game title
	Logger    *slog.Logger

	Debounce     time.Duration // fsnotify debounce, default 150ms
	PollEvery    time.Duration // re-read interval after a progress trigger, default 250ms
	PollFor      time.Duration // how long to poll before falling back, default 10s
	DedupeWindow time.Duration // same (appid, apiName) within this window is one toast, default 10m
	Now          func() time.Time
}

// Event is published for every detected unlock.
type Event struct {
	AppID    uint32            `json:"appid"`
	Game     string            `json:"game"`
	APIName  string            `json:"apiName"`
	Names    map[string]string `json:"names"`
	Descs    map[string]string `json:"descs"`
	Icon     string            `json:"icon"`
	IconGray string            `json:"iconGray,omitempty"`
	Hidden   bool              `json:"hidden"`
	Unlocked int               `json:"unlocked"`
	Total    int               `json:"total"`
	Time     int64             `json:"time"`
	Source   string            `json:"source"` // stats | ffnx | librarycache | progress | test
	Generic  bool              `json:"generic,omitempty"`
}

// AchievementStatus is one row of AppStatus.
type AchievementStatus struct {
	Achievement
	Unlocked bool  `json:"unlocked"`
	Time     int64 `json:"time,omitempty"`
}

// AppStatus is the current achievement state of one app.
type AppStatus struct {
	AppID        uint32              `json:"appid"`
	Game         string              `json:"game"`
	Unlocked     int                 `json:"unlocked"`
	Total        int                 `json:"total"`
	Achievements []AchievementStatus `json:"achievements"`
}

// AppSummary is a known app with its counters.
type AppSummary struct {
	AppID    uint32 `json:"appid"`
	Game     string `json:"game"`
	Unlocked int    `json:"unlocked"`
	Total    int    `json:"total"`
}

type schemaEntry struct {
	schema *Schema
	mtime  time.Time
	size   int64
}

// Watcher detects unlocks and reports them through emit.
type Watcher struct {
	cfg  Config
	emit func(Event)
	log  *slog.Logger

	initOnce sync.Once
	initErr  error
	root     string
	account  uint32
	started  time.Time

	mu        sync.Mutex
	snap      map[uint32]map[string]uint32 // appid -> statID -> packed bits
	cacheSeen map[uint32]map[string]bool   // appid -> achieved API names per librarycache json
	progress  map[uint32]Progress
	names     map[uint32]string
	recent    map[string]time.Time
	polling   map[uint32]bool

	smu     sync.Mutex
	schemas map[uint32]*schemaEntry
}

var userStatsFileRE = regexp.MustCompile(`^UserGameStats_(\d+)_(\d+)\.bin$`)

// New creates a Watcher. emit may be nil for read-only use (Status/Apps).
func New(cfg Config, emit func(Event)) *Watcher {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Debounce <= 0 {
		cfg.Debounce = 150 * time.Millisecond
	}
	if cfg.PollEvery <= 0 {
		cfg.PollEvery = 250 * time.Millisecond
	}
	if cfg.PollFor <= 0 {
		cfg.PollFor = 10 * time.Second
	}
	if cfg.DedupeWindow <= 0 {
		cfg.DedupeWindow = 10 * time.Minute
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if emit == nil {
		emit = func(Event) {}
	}
	return &Watcher{
		cfg: cfg, emit: emit, log: cfg.Logger,
		snap: map[uint32]map[string]uint32{}, cacheSeen: map[uint32]map[string]bool{},
		progress: map[uint32]Progress{}, names: map[uint32]string{}, recent: map[string]time.Time{},
		polling: map[uint32]bool{}, schemas: map[uint32]*schemaEntry{},
	}
}

// Init resolves the Steam root and account id. Run calls it; read-only users
// (Status, Apps, MakeEvent) may call it directly.
func (w *Watcher) Init() error {
	w.initOnce.Do(func() {
		root := w.cfg.SteamRoot
		if root == "" {
			root = DefaultRoot()
		}
		if root == "" {
			w.initErr = errors.New("steamach: steam root not found")
			return
		}
		if st, err := os.Stat(root); err != nil || !st.IsDir() {
			w.initErr = fmt.Errorf("steamach: steam root %s is not a directory", root)
			return
		}
		account := w.cfg.AccountID
		if account == 0 {
			id, err := FindAccountID(root)
			if err != nil {
				w.initErr = err
				return
			}
			account = id
		}
		w.root, w.account = root, account
	})
	return w.initErr
}

// Root is the resolved Steam root (after Init).
func (w *Watcher) Root() string { return w.root }

// Account is the resolved account id (after Init).
func (w *Watcher) Account() uint32 { return w.account }

// Run watches until ctx is done.
func (w *Watcher) Run(ctx context.Context) error {
	if err := w.Init(); err != nil {
		return err
	}
	w.started = w.cfg.Now()
	w.snapshotAll()
	w.log.Info("steam achievements watcher started", "root", w.root, "account", w.account,
		"apps", len(w.snap), "ffnxLogs", len(w.cfg.FFNxLogs))

	for path, appid := range w.cfg.FFNxLogs {
		tailer := &Tailer{Path: path, OnLine: func(line string) {
			if api, ok := ParseFFNxLine(line); ok {
				w.handleFFNx(appid, api)
			}
		}}
		go tailer.Run(ctx)
	}

	fire := make(chan string, 64)
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		w.log.Warn("fsnotify unavailable, polling only", "error", err)
	} else {
		defer fsw.Close()
		for _, dir := range []string{libraryCacheDir(w.root, w.account), statsDir(w.root)} {
			if err := fsw.Add(dir); err != nil {
				w.log.Warn("cannot watch steam directory", "dir", dir, "error", err)
			}
		}
	}

	var timersMu sync.Mutex
	timers := map[string]*time.Timer{}
	schedule := func(path string) {
		timersMu.Lock()
		defer timersMu.Unlock()
		if t, ok := timers[path]; ok {
			t.Reset(w.cfg.Debounce)
			return
		}
		timers[path] = time.AfterFunc(w.cfg.Debounce, func() {
			timersMu.Lock()
			delete(timers, path)
			timersMu.Unlock()
			select {
			case fire <- path:
			case <-ctx.Done():
			}
		})
	}

	var events <-chan fsnotify.Event
	var errs <-chan error
	if fsw != nil {
		events, errs = fsw.Events, fsw.Errors
	}
	// Belt and braces: fsnotify can miss renames into the directory and the
	// progress file is tiny, so stat it once a second as well.
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	progressFile := progressPath(w.root, w.account)
	var lastProgress time.Time
	var lastProgressSize int64
	if st, err := os.Stat(progressFile); err == nil {
		lastProgress, lastProgressSize = st.ModTime(), st.Size()
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case ev := <-events:
			if ev.Has(fsnotify.Write) || ev.Has(fsnotify.Create) || ev.Has(fsnotify.Rename) {
				schedule(ev.Name)
			}
		case err := <-errs:
			if err != nil {
				w.log.Warn("fsnotify error", "error", err)
			}
		case path := <-fire:
			w.dispatch(path)
		case <-ticker.C:
			if st, err := os.Stat(progressFile); err == nil && (!st.ModTime().Equal(lastProgress) || st.Size() != lastProgressSize) {
				lastProgress, lastProgressSize = st.ModTime(), st.Size()
				w.handleProgress()
			}
		}
	}
}

func (w *Watcher) dispatch(path string) {
	dir, name := filepath.Split(path)
	dir = filepath.Clean(dir)
	switch dir {
	case libraryCacheDir(w.root, w.account):
		if name == "achievement_progress.json" {
			w.handleProgress()
		}
	case statsDir(w.root):
		m := userStatsFileRE.FindStringSubmatch(name)
		if m == nil {
			return
		}
		acc, _ := strconv.ParseUint(m[1], 10, 32)
		appid, _ := strconv.ParseUint(m[2], 10, 32)
		if uint32(acc) != w.account || appid == 0 {
			return
		}
		w.identify(uint32(appid))
	}
}

func (w *Watcher) snapshotAll() {
	entries, err := os.ReadDir(statsDir(w.root))
	if err != nil {
		w.log.Warn("cannot list steam stats", "dir", statsDir(w.root), "error", err)
	}
	prefix := fmt.Sprintf("UserGameStats_%d_", w.account)
	for _, e := range entries {
		m := userStatsFileRE.FindStringSubmatch(e.Name())
		if m == nil || !strings.HasPrefix(e.Name(), prefix) {
			continue
		}
		appid, _ := strconv.ParseUint(m[2], 10, 32)
		u, err := w.readUserStats(uint32(appid))
		if err != nil {
			w.log.Debug("skipping unreadable user stats", "file", e.Name(), "error", err)
			continue
		}
		w.snap[uint32(appid)] = u.Stats
	}
	if data, err := readRetry(progressPath(w.root, w.account)); err == nil {
		if p, err := ParseProgress(data); err == nil {
			w.progress = p
		}
	}
	if entries, err := os.ReadDir(libraryCacheDir(w.root, w.account)); err == nil {
		for _, e := range entries {
			appid, err := strconv.ParseUint(strings.TrimSuffix(e.Name(), ".json"), 10, 32)
			if err != nil || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			if seen, ok := w.readCacheAchieved(uint32(appid)); ok {
				w.cacheSeen[uint32(appid)] = seen
			}
		}
	}
}

// handleProgress reacts to achievement_progress.json: an app whose unlocked
// count grew beyond what we know triggers identification.
func (w *Watcher) handleProgress() {
	data, err := readRetry(progressPath(w.root, w.account))
	if err != nil {
		return
	}
	current, err := ParseProgress(data)
	if err != nil {
		w.log.Debug("achievement_progress.json unreadable", "error", err)
		return
	}
	var triggers []uint32
	w.mu.Lock()
	prev := w.progress
	w.progress = current
	for appid, p := range current {
		known := -1
		if pp, ok := prev[appid]; ok {
			known = pp.Unlocked
		}
		if snap, ok := w.snap[appid]; ok {
			if sch, err := w.schema(appid); err == nil {
				if n := sch.CountUnlocked(snap); n > known {
					known = n
				}
			}
		}
		if known >= 0 && p.Unlocked > known {
			w.log.Debug("achievement progress grew", "appid", appid, "known", known, "now", p.Unlocked)
			triggers = append(triggers, appid)
		}
	}
	w.mu.Unlock()
	for _, appid := range triggers {
		go w.identifyWithPoll(appid)
	}
}

// identifyWithPoll re-reads the stats file for a while, then falls back to the
// librarycache json and finally to a generic event.
func (w *Watcher) identifyWithPoll(appid uint32) {
	w.mu.Lock()
	if w.polling[appid] {
		w.mu.Unlock()
		return
	}
	w.polling[appid] = true
	w.mu.Unlock()
	defer func() {
		w.mu.Lock()
		delete(w.polling, appid)
		w.mu.Unlock()
	}()

	deadline := w.cfg.Now().Add(w.cfg.PollFor)
	for {
		if w.identify(appid) || w.caughtUp(appid) {
			return
		}
		if w.cfg.Now().After(deadline) {
			break
		}
		time.Sleep(w.cfg.PollEvery)
	}
	w.log.Debug("stats file did not explain the unlock, trying librarycache", "appid", appid)
	if w.fallbackLibraryCache(appid) {
		return
	}
	w.emitGeneric(appid)
}

// caughtUp reports whether the snapshot already accounts for every unlock
// Steam's progress cache reports (e.g. a concurrent identify handled it).
func (w *Watcher) caughtUp(appid uint32) bool {
	sch, err := w.schema(appid)
	if err != nil {
		return false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	p, ok := w.progress[appid]
	return ok && p.Unlocked > 0 && sch.CountUnlocked(w.snap[appid]) >= p.Unlocked
}

// identify diffs the stats file against the snapshot. It returns true when
// the trigger is explained: new unlocks were emitted, or a reset was seen.
func (w *Watcher) identify(appid uint32) bool {
	sch, err := w.schema(appid)
	if err != nil {
		return false
	}
	u, err := w.readUserStats(appid)
	if err != nil {
		return false
	}
	now := w.cfg.Now()
	var events []Event
	w.mu.Lock()
	prev, known := w.snap[appid]
	w.snap[appid] = u.Stats
	if !known {
		// First sight of this app's stats: Steam just cached an old game. Snapshot silently.
		w.mu.Unlock()
		return false
	}
	unlocked, cleared := sch.Diff(prev, u.Stats)
	if cleared {
		w.log.Info("achievements reset detected", "appid", appid)
	}
	count := sch.CountUnlocked(u.Stats)
	for _, a := range unlocked {
		if !w.dedupeLocked(appid, a.APIName, now) {
			continue
		}
		t := u.UnlockTime(a.StatID, a.Bit)
		if t < w.started.Unix() {
			t = now.Unix()
		}
		events = append(events, w.eventLocked(appid, sch, a, count, t, "stats"))
	}
	w.mu.Unlock()
	for _, ev := range events {
		w.emit(ev)
	}
	return len(unlocked) > 0 || cleared
}

func (w *Watcher) fallbackLibraryCache(appid uint32) bool {
	seen, ok := w.readCacheAchieved(appid)
	if !ok {
		w.log.Debug("librarycache json unreadable", "appid", appid)
		return false
	}
	data, _ := readRetry(libraryCachePath(w.root, w.account, appid))
	list, _ := ParseLibraryCache(data)
	sch, _ := w.schema(appid)
	now := w.cfg.Now()
	var events []Event
	w.mu.Lock()
	prev, known := w.cacheSeen[appid]
	w.cacheSeen[appid] = seen
	explained := 0
	if known {
		count := len(seen)
		for _, a := range list {
			if !a.Achieved || prev[a.ID] {
				continue
			}
			explained++
			if !w.dedupeLocked(appid, a.ID, now) {
				continue // already toasted via FFNx or the stats file
			}
			t := a.Unlocked
			if t < w.started.Unix() {
				t = now.Unix()
			}
			if ach := sch.ByAPIName(a.ID); ach != nil {
				events = append(events, w.eventLocked(appid, sch, ach, count, t, "librarycache"))
				continue
			}
			events = append(events, Event{
				AppID: appid, Game: w.gameNameLocked(appid, sch), APIName: a.ID,
				Names: map[string]string{"english": a.Name}, Descs: map[string]string{"english": a.Desc},
				Icon: a.Image, Unlocked: count, Total: w.totalLocked(appid, sch), Time: t, Source: "librarycache",
			})
		}
	}
	w.mu.Unlock()
	for _, ev := range events {
		w.emit(ev)
	}
	return explained > 0
}

func (w *Watcher) emitGeneric(appid uint32) {
	sch, _ := w.schema(appid)
	w.mu.Lock()
	p := w.progress[appid]
	ev := Event{
		AppID: appid, Game: w.gameNameLocked(appid, sch), Unlocked: p.Unlocked, Total: p.Total,
		Names: map[string]string{}, Descs: map[string]string{},
		Time: w.cfg.Now().Unix(), Source: "progress", Generic: true,
	}
	if ev.Total == 0 {
		ev.Total = w.totalLocked(appid, sch)
	}
	w.mu.Unlock()
	w.emit(ev)
}

func (w *Watcher) handleFFNx(appid uint32, api string) {
	sch, _ := w.schema(appid)
	now := w.cfg.Now()
	w.mu.Lock()
	if !w.dedupeLocked(appid, api, now) {
		w.mu.Unlock()
		return
	}
	var ev Event
	if ach := sch.ByAPIName(api); ach != nil {
		// The stats file catches up a moment later; the dedupe window keeps that silent.
		count := sch.CountUnlocked(w.snap[appid])
		if !IsSet(w.snap[appid], ach.StatID, ach.Bit) {
			count++
		}
		ev = w.eventLocked(appid, sch, ach, count, now.Unix(), "ffnx")
	} else {
		ev = Event{
			AppID: appid, Game: w.gameNameLocked(appid, sch), APIName: api,
			Names: map[string]string{"english": humanize(api)}, Descs: map[string]string{},
			Unlocked: w.progress[appid].Unlocked + 1, Total: w.totalLocked(appid, sch), Time: now.Unix(), Source: "ffnx",
		}
	}
	w.mu.Unlock()
	w.emit(ev)
}

// MakeEvent builds a realistic event for an achievement, for test toasts. An
// empty api picks a random unlocked-or-not achievement from the schema.
func (w *Watcher) MakeEvent(appid uint32, api string) (Event, error) {
	if err := w.Init(); err != nil {
		return Event{}, err
	}
	sch, err := w.schema(appid)
	if err != nil {
		return Event{}, err
	}
	if sch.Total() == 0 {
		return Event{}, fmt.Errorf("steamach: app %d has no achievements in its schema", appid)
	}
	var ach *Achievement
	if api != "" {
		ach = sch.ByAPIName(api)
		if ach == nil {
			return Event{}, fmt.Errorf("steamach: unknown achievement %q for app %d", api, appid)
		}
	} else {
		ach = &sch.Achievements[int(w.cfg.Now().UnixNano()/1000)%sch.Total()]
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	stats := w.snap[appid]
	if stats == nil {
		if u, err := w.readUserStats(appid); err == nil {
			stats = u.Stats
		}
	}
	count := sch.CountUnlocked(stats)
	if !IsSet(stats, ach.StatID, ach.Bit) {
		count++
	}
	return w.eventLocked(appid, sch, ach, count, w.cfg.Now().Unix(), "test"), nil
}

// Status reads the current state of one app from disk.
func (w *Watcher) Status(appid uint32) (AppStatus, error) {
	if err := w.Init(); err != nil {
		return AppStatus{}, err
	}
	sch, err := w.schema(appid)
	if err != nil {
		return AppStatus{}, err
	}
	var stats map[string]uint32
	var times *UserStats
	if u, err := w.readUserStats(appid); err == nil {
		stats, times = u.Stats, u
	} else {
		w.mu.Lock()
		stats = w.snap[appid]
		w.mu.Unlock()
	}
	w.mu.Lock()
	game := w.gameNameLocked(appid, sch)
	w.mu.Unlock()
	status := AppStatus{AppID: appid, Game: game, Total: sch.Total()}
	for i := range sch.Achievements {
		a := sch.Achievements[i]
		a.Icon = IconURL(appid, a.Icon)
		a.IconGray = IconURL(appid, a.IconGray)
		row := AchievementStatus{Achievement: a, Unlocked: IsSet(stats, a.StatID, a.Bit)}
		if row.Unlocked {
			status.Unlocked++
			row.Time = times.UnlockTime(a.StatID, a.Bit)
		}
		status.Achievements = append(status.Achievements, row)
	}
	return status, nil
}

// Apps lists the apps with known achievement state.
func (w *Watcher) Apps() ([]AppSummary, error) {
	if err := w.Init(); err != nil {
		return nil, err
	}
	w.mu.Lock()
	ids := map[uint32]bool{}
	for id := range w.snap {
		ids[id] = true
	}
	for id := range w.progress {
		ids[id] = true
	}
	w.mu.Unlock()
	if len(ids) == 0 {
		w.snapshotAll()
		w.mu.Lock()
		for id := range w.snap {
			ids[id] = true
		}
		for id := range w.progress {
			ids[id] = true
		}
		w.mu.Unlock()
	}
	var out []AppSummary
	for id := range ids {
		sch, _ := w.schema(id)
		w.mu.Lock()
		s := AppSummary{AppID: id, Game: w.gameNameLocked(id, sch), Total: w.totalLocked(id, sch)}
		if sch != nil {
			s.Unlocked = sch.CountUnlocked(w.snap[id])
		} else {
			s.Unlocked = w.progress[id].Unlocked
		}
		w.mu.Unlock()
		if s.Total == 0 {
			continue
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AppID < out[j].AppID })
	return out, nil
}

func (w *Watcher) eventLocked(appid uint32, sch *Schema, a *Achievement, count int, t int64, source string) Event {
	return Event{
		AppID: appid, Game: w.gameNameLocked(appid, sch), APIName: a.APIName,
		Names: copyMap(a.Names), Descs: copyMap(a.Descs),
		Icon: IconURL(appid, a.Icon), IconGray: IconURL(appid, a.IconGray), Hidden: a.Hidden,
		Unlocked: count, Total: sch.Total(), Time: t, Source: source,
	}
}

func (w *Watcher) dedupeLocked(appid uint32, api string, now time.Time) bool {
	key := fmt.Sprintf("%d:%s", appid, strings.ToUpper(api))
	if last, ok := w.recent[key]; ok && now.Sub(last) < w.cfg.DedupeWindow {
		return false
	}
	if len(w.recent) > 512 {
		for k, t := range w.recent {
			if now.Sub(t) >= w.cfg.DedupeWindow {
				delete(w.recent, k)
			}
		}
	}
	w.recent[key] = now
	return true
}

func (w *Watcher) totalLocked(appid uint32, sch *Schema) int {
	if sch != nil && sch.Total() > 0 {
		return sch.Total()
	}
	return w.progress[appid].Total
}

func (w *Watcher) gameNameLocked(appid uint32, sch *Schema) string {
	if name := w.cfg.GameNames[appid]; name != "" {
		return name
	}
	if name, ok := w.names[appid]; ok {
		return name
	}
	name := ""
	if m, ok := FindManifest(w.root, appid); ok {
		name = m.Name
	}
	if name == "" && sch != nil {
		name = sch.GameName
	}
	if name == "" {
		name = fmt.Sprintf("Steam %d", appid)
	}
	w.names[appid] = name
	return name
}

func (w *Watcher) schema(appid uint32) (*Schema, error) {
	path := schemaPath(w.root, appid)
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	w.smu.Lock()
	defer w.smu.Unlock()
	if e, ok := w.schemas[appid]; ok && e.mtime.Equal(st.ModTime()) && e.size == st.Size() {
		return e.schema, nil
	}
	data, err := readRetry(path)
	if err != nil {
		return nil, err
	}
	sch, err := ParseSchema(data)
	if err != nil {
		return nil, err
	}
	w.schemas[appid] = &schemaEntry{schema: sch, mtime: st.ModTime(), size: st.Size()}
	return sch, nil
}

func (w *Watcher) readUserStats(appid uint32) (*UserStats, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		data, err := os.ReadFile(userStatsPath(w.root, w.account, appid))
		if err != nil {
			return nil, err
		}
		u, err := ParseUserStats(data)
		if err == nil {
			return u, nil
		}
		lastErr = err
		time.Sleep(100 * time.Millisecond)
	}
	return nil, lastErr
}

func (w *Watcher) readCacheAchieved(appid uint32) (map[string]bool, bool) {
	data, err := readRetry(libraryCachePath(w.root, w.account, appid))
	if err != nil {
		return nil, false
	}
	list, err := ParseLibraryCache(data)
	if err != nil {
		return nil, false
	}
	seen := map[string]bool{}
	for _, a := range list {
		if a.Achieved {
			seen[a.ID] = true
		}
	}
	return seen, true
}

func readRetry(path string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		data, err := os.ReadFile(path)
		if err == nil && len(data) > 0 {
			return data, nil
		}
		if err == nil {
			err = errors.New("empty file")
		}
		lastErr = err
		time.Sleep(50 * time.Millisecond)
	}
	return nil, lastErr
}

func copyMap(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func humanize(api string) string {
	words := strings.Fields(strings.ReplaceAll(strings.ToLower(api), "_", " "))
	for i, word := range words {
		if word != "" {
			words[i] = strings.ToUpper(word[:1]) + word[1:]
		}
	}
	return strings.Join(words, " ")
}
