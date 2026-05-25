package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/coder/websocket"
	twitch "github.com/gempir/go-twitch-irc/v4"
)

type TwitchDeviceCode struct {
	DeviceCode      string `json:"deviceCode"`
	UserCode        string `json:"userCode"`
	VerificationURI string `json:"verificationUri"`
	Interval        int    `json:"interval"`
	ExpiresIn       int    `json:"expiresIn"`
}

type TwitchAuth struct {
	store    *Store
	clientID string
	logger   *slog.Logger
}

func NewTwitchAuth(store *Store, clientID string, logger *slog.Logger) *TwitchAuth {
	return &TwitchAuth{store: store, clientID: clientID, logger: logger}
}

func (t *TwitchAuth) RequestDeviceCode(ctx context.Context, scopes []string) (TwitchDeviceCode, error) {
	form := "client_id=" + t.clientID + "&scopes=" + strings.ReplaceAll(strings.Join(scopes, " "), " ", "+")
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://id.twitch.tv/oauth2/device", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return TwitchDeviceCode{}, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode/100 != 2 {
		return TwitchDeviceCode{}, fmt.Errorf("device code request failed: %s", body)
	}
	var raw struct {
		DeviceCode      string `json:"device_code"`
		UserCode        string `json:"user_code"`
		VerificationURI string `json:"verification_uri"`
		Interval        int    `json:"interval"`
		ExpiresIn       int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return TwitchDeviceCode{}, err
	}
	return TwitchDeviceCode{DeviceCode: raw.DeviceCode, UserCode: raw.UserCode, VerificationURI: raw.VerificationURI, Interval: raw.Interval, ExpiresIn: raw.ExpiresIn}, nil
}

func (t *TwitchAuth) PollForToken(ctx context.Context, deviceCode string, interval int) (TokenData, error) {
	if interval <= 0 {
		interval = 5
	}
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()
	for attempts := 0; attempts < 120; attempts++ {
		select {
		case <-ctx.Done():
			return TokenData{}, ctx.Err()
		case <-ticker.C:
		}
		form := "client_id=" + t.clientID + "&device_code=" + deviceCode + "&grant_type=urn:ietf:params:oauth:grant-type:device_code"
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://id.twitch.tv/oauth2/token", strings.NewReader(form))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			return TokenData{}, err
		}
		body := readAllAndClose(res.Body)
		var payload map[string]any
		_ = json.Unmarshal(body, &payload)
		if res.StatusCode/100 == 2 {
			expiresIn, _ := payload["expires_in"].(float64)
			token := TokenData{
				AccessToken:  fmt.Sprint(payload["access_token"]),
				RefreshToken: fmt.Sprint(payload["refresh_token"]),
				ExpiresAt:    time.Now().Add(time.Duration(expiresIn) * time.Second).UnixMilli(),
			}
			return token, t.store.SaveToken(ctx, "twitch", token)
		}
		if msg := fmt.Sprint(payload["message"]); strings.Contains(msg, "authorization_pending") {
			continue
		}
		return TokenData{}, fmt.Errorf("twitch token polling failed: %s", body)
	}
	return TokenData{}, fmt.Errorf("twitch token polling timed out")
}

func (t *TwitchAuth) AuthenticatedUserName(ctx context.Context, accessToken string) (string, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.twitch.tv/helix/users", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Client-Id", t.clientID)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	var payload struct {
		Data []struct {
			Login string `json:"login"`
		} `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return "", err
	}
	if len(payload.Data) == 0 {
		return "", fmt.Errorf("authenticated twitch user not found")
	}
	return payload.Data[0].Login, nil
}

func (t *TwitchAuth) RefreshToken(ctx context.Context, refreshToken string) (TokenData, error) {
	form := "client_id=" + t.clientID + "&grant_type=refresh_token&refresh_token=" + refreshToken
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://id.twitch.tv/oauth2/token", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return TokenData{}, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode/100 != 2 {
		return TokenData{}, fmt.Errorf("twitch token refresh failed: %s", body)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return TokenData{}, err
	}
	expiresIn, _ := payload["expires_in"].(float64)
	
	refreshTokenOut := fmt.Sprint(payload["refresh_token"])
	if refreshTokenOut == "" || refreshTokenOut == "<nil>" {
		refreshTokenOut = refreshToken
	}
	
	token := TokenData{
		AccessToken:  fmt.Sprint(payload["access_token"]),
		RefreshToken: refreshTokenOut,
		ExpiresAt:    time.Now().Add(time.Duration(expiresIn) * time.Second).UnixMilli(),
	}
	return token, t.store.SaveToken(ctx, "twitch", token)
}

func GetOrRefreshTwitchToken(ctx context.Context, store *Store, clientID string, logger *slog.Logger) (TokenData, error) {
	token, err := store.Token(ctx, "twitch")
	if err != nil {
		return TokenData{}, err
	}
	if token.AccessToken == "" {
		return TokenData{}, fmt.Errorf("no twitch token found")
	}

	// Check if token has expired or is about to expire (within 5 minutes)
	if token.RefreshToken != "" && (token.ExpiresAt == 0 || time.Now().UnixMilli() > (token.ExpiresAt-5*60*1000)) {
		logger.Info("twitch token expired or expiring soon, refreshing")
		auth := NewTwitchAuth(store, clientID, logger)
		newToken, err := auth.RefreshToken(ctx, token.RefreshToken)
		if err != nil {
			logger.Error("failed to refresh twitch token", "error", err)
			return token, err // return old one as fallback
		}
		logger.Info("twitch token refreshed successfully")
		return newToken, nil
	}

	return token, nil
}

type TwitchChatAdapter struct {
	cfg          AppConfig
	store        *Store
	logger       *slog.Logger
	emit         func(ChatMessage)
	client       *twitch.Client
	emoteCatalog *thirdPartyEmoteCache
}

func NewTwitchChatAdapter(cfg AppConfig, store *Store, logger *slog.Logger, emit func(ChatMessage)) *TwitchChatAdapter {
	return &TwitchChatAdapter{cfg: cfg, store: store, logger: logger.With("adapter", "twitch_chat"), emit: emit, emoteCatalog: newThirdPartyEmoteCache()}
}

func (a *TwitchChatAdapter) Start(ctx context.Context) {
	token, _ := GetOrRefreshTwitchToken(ctx, a.store, a.cfg.TwitchClientID, a.logger)
	if token.AccessToken != "" {
		a.client = twitch.NewClient(a.cfg.TwitchChannel, "oauth:"+token.AccessToken)
	} else {
		a.client = twitch.NewAnonymousClient()
	}
	a.client.Join(a.cfg.TwitchChannel)
	a.client.OnPrivateMessage(func(message twitch.PrivateMessage) {
		badges := twitchMessageBadges(message)
		thirdPartyEmotes := a.emoteCatalog.Emotes(context.Background(), message.RoomID, message.Channel, a.logger)
		html := renderTwitchMessage(message.Message, message.Emotes, thirdPartyEmotes)
		a.emit(ChatMessage{
			ID: message.ID, Platform: PlatformTwitch, User: message.User.Name, DisplayName: message.User.DisplayName,
			Content: sanitizeText(message.Message), HTMLContent: html, Color: message.User.Color, Badges: badges, Timestamp: message.Time,
			CustomRewardID: message.CustomRewardID,
		})
	})
	go func() {
		<-ctx.Done()
		a.Stop()
	}()
	if err := a.client.Connect(); err != nil {
		a.logger.Warn("twitch chat stopped", "error", err)
	}
}

func (a *TwitchChatAdapter) Stop() {
	if a.client != nil {
		a.client.Disconnect()
	}
}

func twitchMessageBadges(message twitch.PrivateMessage) []Badge {
	seen := map[string]bool{}
	var badges []Badge
	add := func(kind string) {
		if seen[kind] {
			return
		}
		seen[kind] = true
		badges = append(badges, Badge{Type: kind})
	}

	for name := range message.User.Badges {
		addTwitchRoleBadge(name, add)
	}
	for _, rawBadge := range strings.Split(message.Tags["badges"], ",") {
		name, _, _ := strings.Cut(rawBadge, "/")
		addTwitchRoleBadge(name, add)
	}

	if message.Tags["user-type"] == "mod" || message.Tags["mod"] == "1" {
		add("moderator")
	}
	if message.Tags["subscriber"] == "1" {
		add("subscriber")
	}
	if strings.EqualFold(message.User.Name, message.Channel) && message.User.Name != "" {
		add("owner")
	}

	return badges
}

func addTwitchRoleBadge(name string, add func(string)) {
	switch kind := strings.ToLower(strings.TrimSpace(name)); kind {
	case "broadcaster":
		add("owner")
	case "moderator", "vip":
		add(kind)
	case "subscriber", "founder":
		add("subscriber")
	}
}

func renderTwitchMessage(message string, emotes []*twitch.Emote, thirdPartyEmotes map[string]thirdPartyEmote) string {
	type repl struct {
		start, end int
		url, alt   string
	}
	var replacements []repl
	for _, emote := range emotes {
		for _, pos := range emote.Positions {
			replacements = append(replacements, repl{
				start: pos.Start,
				end:   pos.End,
				url:   "https://static-cdn.jtvnw.net/emoticons/v2/" + emote.ID + "/default/dark/3.0",
				alt:   "emote",
			})
		}
	}
	sort.Slice(replacements, func(i, j int) bool {
		return replacements[i].start < replacements[j].start
	})
	var out bytes.Buffer
	last := 0
	for _, r := range replacements {
		if r.start < last || r.start < 0 || r.end >= len(message) || r.end < r.start {
			continue
		}
		if r.start > last {
			out.WriteString(renderThirdPartyEmotes(message[last:r.start], thirdPartyEmotes))
		}
		out.WriteString(renderEmoteImage(r.url, r.alt))
		last = r.end + 1
	}
	if last < len(message) {
		out.WriteString(renderThirdPartyEmotes(message[last:], thirdPartyEmotes))
	}
	return out.String()
}

func renderThirdPartyEmotes(text string, emotes map[string]thirdPartyEmote) string {
	if len(emotes) == 0 || text == "" {
		return sanitizeText(text)
	}
	var out bytes.Buffer
	for len(text) > 0 {
		r, size := firstRune(text)
		if unicode.IsSpace(r) {
			out.WriteString(sanitizeText(text[:size]))
			text = text[size:]
			continue
		}
		tokenEnd := size
		for tokenEnd < len(text) {
			r, size = firstRune(text[tokenEnd:])
			if unicode.IsSpace(r) {
				break
			}
			tokenEnd += size
		}
		token := text[:tokenEnd]
		if emote, ok := emotes[token]; ok {
			out.WriteString(renderEmoteImage(emote.URL, emote.Code))
		} else {
			out.WriteString(sanitizeText(token))
		}
		text = text[tokenEnd:]
	}
	return out.String()
}

func firstRune(text string) (rune, int) {
	return utf8.DecodeRuneInString(text)
}

func renderEmoteImage(url, alt string) string {
	return `<img src="` + html.EscapeString(url) + `" class="emote" alt="` + html.EscapeString(alt) + `">`
}

type TwitchEventSubAdapter struct {
	cfg    AppConfig
	store  *Store
	logger *slog.Logger
	emit   func(Event)
	cancel context.CancelFunc
}

func NewTwitchEventSubAdapter(cfg AppConfig, store *Store, logger *slog.Logger, emit func(Event)) *TwitchEventSubAdapter {
	return &TwitchEventSubAdapter{cfg: cfg, store: store, logger: logger.With("adapter", "twitch_eventsub"), emit: emit}
}

func (a *TwitchEventSubAdapter) Start(ctx context.Context) {
	runCtx, cancel := context.WithCancel(ctx)
	a.cancel = cancel
	for {
		if err := a.run(runCtx); err != nil {
			a.logger.Warn("eventsub disconnected", "error", err)
		}
		select {
		case <-runCtx.Done():
			return
		case <-time.After(10 * time.Second):
		}
	}
}

func (a *TwitchEventSubAdapter) Stop() {
	if a.cancel != nil {
		a.cancel()
	}
}

func (a *TwitchEventSubAdapter) run(ctx context.Context) error {
	conn, _, err := websocket.Dial(ctx, "wss://eventsub.wss.twitch.tv/ws", nil)
	if err != nil {
		return err
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return err
		}
		var msg struct {
			Metadata struct {
				MessageType string `json:"message_type"`
			} `json:"metadata"`
			Payload json.RawMessage `json:"payload"`
		}
		if json.Unmarshal(data, &msg) != nil {
			continue
		}
		if msg.Metadata.MessageType == "session_welcome" {
			var welcome struct {
				Session struct {
					ID string `json:"id"`
				} `json:"session"`
			}
			_ = json.Unmarshal(msg.Payload, &welcome)
			go a.subscribeAll(ctx, welcome.Session.ID)
		}
		if msg.Metadata.MessageType == "notification" {
			a.handleNotification(msg.Payload)
		}
	}
}

func (a *TwitchEventSubAdapter) subscribeAll(ctx context.Context, sessionID string) {
	token, err := GetOrRefreshTwitchToken(ctx, a.store, a.cfg.TwitchClientID, a.logger)
	if err != nil || token.AccessToken == "" {
		return
	}
	userID, err := a.twitchUserID(ctx, token.AccessToken)
	if err != nil {
		a.logger.Warn("failed to resolve twitch user id", "error", err)
		return
	}
	types := []struct {
		typ, version string
		condition    map[string]string
	}{
		{"channel.follow", "2", map[string]string{"broadcaster_user_id": userID, "moderator_user_id": userID}},
		{"channel.subscribe", "1", map[string]string{"broadcaster_user_id": userID}},
		{"channel.subscription.message", "1", map[string]string{"broadcaster_user_id": userID}},
		{"channel.cheer", "1", map[string]string{"broadcaster_user_id": userID}},
		{"channel.raid", "1", map[string]string{"to_broadcaster_user_id": userID}},
		{"channel.channel_points_custom_reward_redemption.add", "1", map[string]string{"broadcaster_user_id": userID}},
	}
	for _, sub := range types {
		body, _ := json.Marshal(map[string]any{
			"type": sub.typ, "version": sub.version, "condition": sub.condition,
			"transport": map[string]string{"method": "websocket", "session_id": sessionID},
		})
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.twitch.tv/helix/eventsub/subscriptions", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token.AccessToken)
		req.Header.Set("Client-Id", a.cfg.TwitchClientID)
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err == nil {
			_ = res.Body.Close()
		}
	}
}

func (a *TwitchEventSubAdapter) twitchUserID(ctx context.Context, token string) (string, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.twitch.tv/helix/users?login="+a.cfg.TwitchChannel, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Client-Id", a.cfg.TwitchClientID)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return "", err
	}
	if len(payload.Data) == 0 {
		return "", fmt.Errorf("twitch user not found")
	}
	return payload.Data[0].ID, nil
}

func (a *TwitchEventSubAdapter) handleNotification(payload json.RawMessage) {
	var raw struct {
		Subscription struct {
			Type string `json:"type"`
		} `json:"subscription"`
		Event map[string]any `json:"event"`
	}
	if json.Unmarshal(payload, &raw) != nil {
		return
	}
	user := firstString(raw.Event, "user_name", "user_login", "from_broadcaster_user_name")
	switch raw.Subscription.Type {
	case "channel.follow":
		a.emit(Event{Type: "follow", Platform: PlatformTwitch, User: user})
	case "channel.subscribe", "channel.subscription.message":
		a.emit(Event{Type: "subscription", Platform: PlatformTwitch, User: user, Tier: raw.Event["tier"], Message: firstString(raw.Event, "message")})
	case "channel.cheer":
		a.emit(Event{Type: "bits", Platform: PlatformTwitch, User: user, Amount: raw.Event["bits"], Message: firstString(raw.Event, "message")})
	case "channel.raid":
		a.emit(Event{Type: "raid", Platform: PlatformTwitch, User: user, Viewers: raw.Event["viewers"]})
	case "channel.channel_points_custom_reward_redemption.add":
		a.emit(Event{
			Type: PlatformEventChannelPoints, Platform: PlatformTwitch, User: user,
			Message: firstString(raw.Event, "user_input"), RewardName: nestedString(raw.Event, "reward", "title"),
		})
	}
}

const PlatformEventChannelPoints = "channel_points"

func firstString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := m[key].(string); ok {
			return v
		}
	}
	return ""
}

func nestedString(m map[string]any, key, nestedKey string) string {
	child, ok := m[key].(map[string]any)
	if !ok {
		return ""
	}
	return firstString(child, nestedKey)
}
