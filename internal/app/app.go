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
	"regexp"
	"strconv"
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
	normalizeWidgetsConfig(&cfg.WidgetsConfig)
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
	mux.HandleFunc("/api/widgets/state", a.handleWidgetState)
	mux.HandleFunc("/api/widgets/test", a.handleWidgetTest)
	mux.HandleFunc("/api/widgets/support-goal/reset", a.handleSupportGoalReset)
	mux.HandleFunc("/ws/chat", a.hub.Serve("chat"))
	mux.HandleFunc("/ws/alerts", a.hub.Serve("alerts"))
	mux.HandleFunc("/ws/audio", a.hub.Serve("audio"))
	mux.HandleFunc("/ws/events", a.hub.Serve("events"))
	mux.HandleFunc("/ws/widgets", a.hub.Serve("widgets"))
	mux.Handle("/media/", http.StripPrefix("/media/", http.FileServer(http.Dir(filepath.Join(a.opts.DataDir, "media")))))
	mux.Handle("/dashboard/", http.StripPrefix("/dashboard/", http.FileServer(http.FS(assets.DashboardAssets()))))
	mux.Handle("/shared/", http.StripPrefix("/shared/", http.FileServer(http.FS(assets.SharedAssets()))))
	mux.Handle("/overlay/chat/", http.StripPrefix("/overlay/chat/", http.FileServer(http.FS(assets.ChatAssets()))))
	mux.Handle("/overlay/alerts/", http.StripPrefix("/overlay/alerts/", http.FileServer(http.FS(assets.AlertsAssets()))))
	mux.Handle("/overlay/widgets/support-goal/", http.StripPrefix("/overlay/widgets/support-goal/", http.FileServer(http.FS(assets.SupportGoalAssets()))))
	mux.Handle("/overlay/widgets/recent-events/", http.StripPrefix("/overlay/widgets/recent-events/", http.FileServer(http.FS(assets.RecentEventsAssets()))))
	mux.Handle("/overlay/widgets/custom/", http.StripPrefix("/overlay/widgets/custom/", http.FileServer(http.FS(assets.CustomWidgetAssets()))))
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
		if key == "alertsConfig" || key == "widgetsConfig" {
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
	normalizeWidgetsConfig(&cfg.WidgetsConfig)
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
	a.publishWidgetState(r.Context())
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
		normalizeWidgetsConfig(&cfg.WidgetsConfig)
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
		a.publishWidgetState(r.Context())
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

func (a *App) handleWidgetState(w http.ResponseWriter, r *http.Request) {
	state, err := a.widgetState(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, state)
}

func (a *App) handleWidgetTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Kind     string `json:"kind"`
		WidgetID string `json:"widgetId"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	evt := Event{
		Type:     "superchat",
		Platform: PlatformYouTube,
		User:     "WidgetTester",
		Amount:   25,
		Currency: "USD",
		Message:  "Widget test event from Raikiri.",
	}
	switch body.Kind {
	case "recent":
		evt.Type = "follow"
		evt.Amount = nil
		evt.Currency = ""
		evt.Message = "Recent event widget test."
	case "custom":
		evt = a.testEventForCustomWidget(body.WidgetID)
	}
	a.routeEvent(evt)
	state, err := a.widgetState(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"success": true, "mocked": evt, "state": state})
}

func (a *App) handleSupportGoalReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := a.store.SaveWidgetTime(r.Context(), "support_goal_reset_at", time.Now()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	state, err := a.widgetState(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.hub.Publish("widgets", "state", state)
	writeJSON(w, state)
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
	if !cfg.TTSCmdEnabled {
		return
	}
	text, ok := ttsCommandMessageText(msg, cfg)
	if !ok {
		return
	}
	if !ttsCommandAllowed(msg, cfg) {
		return
	}
	if text != "" {
		a.tts.Enqueue(context.Background(), msg.DisplayName+" dice: "+text, cfg)
	}
}

func ttsCommandMessageText(msg ChatMessage, cfg AppConfig) (string, bool) {
	if strings.TrimSpace(msg.CustomRewardID) != "" {
		return "", false
	}
	return ttsCommandText(msg.Content, cfg.TTSCmdPrefix)
}

func ttsCommandText(content, prefix string) (string, bool) {
	content = strings.TrimSpace(content)
	prefix = strings.TrimSpace(prefix)
	if content == "" || prefix == "" || len(content) < len(prefix) {
		return "", false
	}
	if !strings.EqualFold(content[:len(prefix)], prefix) {
		return "", false
	}
	if len(content) > len(prefix) && !isSpace(content[len(prefix)]) {
		return "", false
	}
	return strings.TrimSpace(content[len(prefix):]), true
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

func ttsCommandAllowed(msg ChatMessage, cfg AppConfig) bool {
	for _, badge := range msg.Badges {
		switch strings.ToLower(badge.Type) {
		case "owner":
			if cfg.TTSCmdHost {
				return true
			}
		case "moderator":
			if cfg.TTSCmdMod {
				return true
			}
		case "subscriber":
			if cfg.TTSCmdSub {
				return true
			}
		case "vip":
			if cfg.TTSCmdVip {
				return true
			}
		}
	}
	return false
}

func (a *App) routeEvent(evt Event) {
	a.store.SaveEvent(context.Background(), evt)
	a.hub.Publish("alerts", "alert", evt)
	a.publishWidgetState(context.Background())
	a.publishCustomWidgetEvent(evt)
	cfg := a.config()
	if evt.Type == "superchat" || evt.Type == "supersticker" || evt.Type == "membership" || evt.Type == "subscription" || evt.Type == "bits" || evt.Type == "gift" {
		a.tts.EnqueueEvent(context.Background(), evt, cfg)
	}
	if text, ok := ttsRewardText(evt, cfg); ok {
		a.tts.Enqueue(context.Background(), text, cfg)
	}
}

func ttsRewardText(evt Event, cfg AppConfig) (string, bool) {
	if evt.Type != PlatformEventChannelPoints || !cfg.TTSRewardEnabled {
		return "", false
	}
	if cfg.TTSRewardName != "" && !strings.EqualFold(strings.TrimSpace(evt.RewardName), strings.TrimSpace(cfg.TTSRewardName)) {
		return "", false
	}
	message := strings.TrimSpace(evt.Message)
	if message == "" {
		return "", false
	}
	if text, ok := ttsCommandText(message, cfg.TTSCmdPrefix); ok {
		message = text
	}
	if message == "" {
		return "", false
	}
	user := strings.TrimSpace(evt.User)
	if user == "" {
		user = "Alguien"
	}
	return user + " dice: " + message, true
}

func (a *App) publishCustomWidgetEvent(evt Event) {
	cfg := a.config().WidgetsConfig
	var ids []string
	for _, widget := range cfg.Custom {
		if customWidgetMatchesEvent(widget, evt) {
			ids = append(ids, widget.ID)
		}
	}
	if len(ids) == 0 {
		return
	}
	a.hub.Publish("widgets", "custom_event", map[string]any{"event": evt, "widgetIds": ids})
}

func customWidgetMatchesEvent(widget CustomWidgetConfig, evt Event) bool {
	if !widget.Enabled {
		return false
	}
	trigger := widget.Activation
	if trigger.EventType == "" {
		trigger.EventType = "any"
	}
	if trigger.EventType != "any" && trigger.EventType != evt.Type {
		return false
	}
	if trigger.MinAmount > 0 && amountFloat(evt.Amount) < trigger.MinAmount {
		return false
	}
	if trigger.RewardName != "" && !strings.EqualFold(strings.TrimSpace(trigger.RewardName), strings.TrimSpace(evt.RewardName)) {
		return false
	}
	return true
}

func (a *App) testEventForCustomWidget(widgetID string) Event {
	cfg := a.config().WidgetsConfig
	var trigger CustomWidgetActivation
	for _, widget := range cfg.Custom {
		if widget.ID == widgetID {
			trigger = widget.Activation
			break
		}
	}
	eventType := trigger.EventType
	if eventType == "" || eventType == "any" {
		eventType = "bits"
	}
	amount := any(25)
	if trigger.MinAmount > 0 {
		amount = trigger.MinAmount
	}
	evt := Event{
		Type: eventType, Platform: PlatformTwitch, User: "WidgetTester", Amount: amount,
		Currency: "USD", Message: "Custom widget activation test.",
	}
	if eventType == PlatformEventChannelPoints {
		evt.Amount = nil
		evt.Currency = ""
		evt.RewardName = trigger.RewardName
		if evt.RewardName == "" {
			evt.RewardName = "Raikiri Alert"
		}
		evt.Message = "Channel points custom widget test."
	}
	return evt
}

func (a *App) widgetState(ctx context.Context) (WidgetState, error) {
	cfg := a.config().WidgetsConfig
	resetAt, err := a.store.WidgetTime(ctx, "support_goal_reset_at")
	if err != nil {
		return WidgetState{}, err
	}
	if resetAt.IsZero() {
		resetAt = time.Unix(0, 0).UTC()
	}
	current, err := a.store.SumSupportSince(ctx, resetAt)
	if err != nil {
		return WidgetState{}, err
	}
	recent, err := a.store.RecentEvents(ctx, cfg.RecentEvents.Types, cfg.RecentEvents.Limit)
	if err != nil {
		return WidgetState{}, err
	}
	if !cfg.RecentEvents.Enabled {
		recent = nil
	}
	return WidgetState{
		Config:       cfg,
		SupportGoal:  SupportGoalState{SupportGoalConfig: cfg.SupportGoal, CurrentAmount: current, ResetAt: resetAt},
		RecentEvents: recent,
	}, nil
}

func (a *App) publishWidgetState(ctx context.Context) {
	state, err := a.widgetState(ctx)
	if err != nil {
		a.logger.Warn("widget state failed", "error", err)
		return
	}
	a.hub.Publish("widgets", "state", state)
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
	if incoming.TTSBlockedWords == "" {
		incoming.TTSBlockedWords = old.TTSBlockedWords
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
		{"ttsBlockedWords", &cfg.TTSBlockedWords},
		{"ttsCmdMod", &cfg.TTSCmdMod},
		{"ttsCmdSub", &cfg.TTSCmdSub},
		{"ttsCmdVip", &cfg.TTSCmdVip},
		{"ttsCmdHost", &cfg.TTSCmdHost},
		{"chatTheme", &cfg.ChatTheme},
		{"chatFontSize", &cfg.ChatFontSize},
		{"chatHideAfter", &cfg.ChatHideAfter},
		{"chatAnimations", &cfg.ChatAnimations},
		{"alertsConfig", &cfg.AlertsConfig},
		{"widgetsConfig", &cfg.WidgetsConfig},
	}
	for _, field := range fields {
		if err := set(field.key, field.dest); err != nil {
			return fmt.Errorf("invalid %s: %w", field.key, err)
		}
	}
	return nil
}

func normalizeWidgetsConfig(cfg *WidgetsConfig) {
	defaults := DefaultConfig().WidgetsConfig
	if cfg.SupportGoal.Title == "" {
		cfg.SupportGoal.Title = defaults.SupportGoal.Title
	}
	if cfg.SupportGoal.TargetAmount <= 0 {
		cfg.SupportGoal.TargetAmount = defaults.SupportGoal.TargetAmount
	}
	if cfg.SupportGoal.Currency == "" {
		cfg.SupportGoal.Currency = defaults.SupportGoal.Currency
	}
	if cfg.SupportGoal.Appearance == (WidgetAppearance{}) {
		cfg.SupportGoal.Enabled = defaults.SupportGoal.Enabled
	}
	normalizeWidgetAppearance(&cfg.SupportGoal.Appearance, defaults.SupportGoal.Appearance)

	if cfg.RecentEvents.Limit <= 0 {
		cfg.RecentEvents.Limit = defaults.RecentEvents.Limit
	}
	if len(cfg.RecentEvents.Types) == 0 {
		cfg.RecentEvents.Types = defaults.RecentEvents.Types
	}
	if cfg.RecentEvents.Appearance == (WidgetAppearance{}) {
		cfg.RecentEvents.Enabled = defaults.RecentEvents.Enabled
	}
	normalizeWidgetAppearance(&cfg.RecentEvents.Appearance, defaults.RecentEvents.Appearance)

	seen := map[string]bool{}
	for i := range cfg.Custom {
		widget := &cfg.Custom[i]
		widget.ID = widgetID(widget.ID, widget.Name, i+1)
		if seen[widget.ID] {
			widget.ID = fmt.Sprintf("%s-%d", widget.ID, i+1)
		}
		seen[widget.ID] = true
		if widget.Name == "" {
			widget.Name = widget.ID
		}
		if widget.Activation.EventType == "" {
			widget.Activation.EventType = "any"
		}
		normalizeWidgetAppearance(&widget.Appearance, DefaultWidgetAppearance(520, true))
	}
}

func normalizeWidgetAppearance(appearance *WidgetAppearance, defaults WidgetAppearance) {
	if appearance.Theme == "" {
		appearance.Theme = defaults.Theme
	}
	if appearance.AccentColor == "" {
		appearance.AccentColor = defaults.AccentColor
	}
	if appearance.FontFamily == "" {
		appearance.FontFamily = defaults.FontFamily
	}
	if appearance.BackgroundOpacity <= 0 {
		appearance.BackgroundOpacity = defaults.BackgroundOpacity
	}
	if appearance.BorderRadius < 0 {
		appearance.BorderRadius = defaults.BorderRadius
	}
	if appearance.Width <= 0 {
		appearance.Width = defaults.Width
	}
}

func widgetID(id, name string, fallback int) string {
	value := strings.ToLower(strings.TrimSpace(id))
	if value == "" {
		value = strings.ToLower(strings.TrimSpace(name))
	}
	value = regexp.MustCompile(`[^a-z0-9_-]+`).ReplaceAllString(value, "-")
	value = strings.Trim(value, "-_")
	if value == "" {
		return fmt.Sprintf("custom-%d", fallback)
	}
	return value
}

var amountPattern = regexp.MustCompile(`[-+]?\d[\d,.]*`)

func amountFloat(raw any) float64 {
	switch value := raw.(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int:
		return float64(value)
	case int64:
		return float64(value)
	case json.Number:
		f, _ := value.Float64()
		return f
	case string:
		match := amountPattern.FindString(value)
		if match == "" {
			return 0
		}
		if strings.Contains(match, ",") && strings.Contains(match, ".") {
			match = strings.ReplaceAll(match, ",", "")
		} else {
			match = strings.ReplaceAll(match, ",", ".")
		}
		f, _ := strconv.ParseFloat(match, 64)
		return f
	default:
		return 0
	}
}

func readAllAndClose(rc io.ReadCloser) []byte {
	defer rc.Close()
	b, _ := io.ReadAll(rc)
	return b
}

func ensureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}
