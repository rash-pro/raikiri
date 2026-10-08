package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

// Overridable in tests.
var (
	twitchHelixURL    = "https://api.twitch.tv/helix"
	twitchValidateURL = "https://id.twitch.tv/oauth2/validate"
)

const (
	twitchBroadcastScope   = "channel:manage:broadcast"
	streamInfoRecentKey    = "streamInfoRecent"
	streamInfoRecentLimit  = 6
	twitchTitleLimit       = 140
	twitchTagLimit         = 10
	twitchTagLength        = 25
	youtubeTitleLimit      = 100
	youtubeGamingCategory  = "20"
	youtubePendingLifetime = 4 * time.Hour
	youtubePendingInterval = 30 * time.Second
)

var (
	errTwitchNotConnected = errors.New("twitch not connected")
	errTwitchMissingScope = errors.New("twitch token lacks " + twitchBroadcastScope)
)

type StreamGame struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	BoxArtURL string `json:"boxArtUrl,omitempty"`
}

type streamInfoPreset struct {
	Title string      `json:"title"`
	Game  *StreamGame `json:"game,omitempty"`
	Tags  []string    `json:"tags,omitempty"`
}

// cleanTags applies Twitch's tag rules (letters and digits only, 25 chars, 10 tags),
// which YouTube also accepts, so both platforms get the same list.
func cleanTags(raw []string) []string {
	tags := []string{}
	for _, tag := range raw {
		tag = strings.Map(func(r rune) rune {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				return r
			}
			return -1
		}, tag)
		if tag == "" || utf8.RuneCountInString(tag) > twitchTagLength {
			continue
		}
		if slices.ContainsFunc(tags, func(t string) bool { return strings.EqualFold(t, tag) }) {
			continue
		}
		if tags = append(tags, tag); len(tags) == twitchTagLimit {
			break
		}
	}
	return tags
}

type platformResult struct {
	OK    bool   `json:"ok"`
	State string `json:"state,omitempty"`
	Code  string `json:"code,omitempty"`
	Error string `json:"error,omitempty"`
}

func resultFromError(err error) platformResult {
	switch {
	case errors.Is(err, errTwitchNotConnected), errors.Is(err, errYouTubeNotConnected):
		return platformResult{Code: "not_connected", Error: err.Error()}
	case errors.Is(err, errTwitchMissingScope):
		return platformResult{Code: "missing_scope", Error: err.Error()}
	}
	return platformResult{Code: "error", Error: err.Error()}
}

// ---- Twitch ----

type twitchIdentity struct {
	token  string
	userID string
	scopes []string
}

func (a *App) twitchIdentity(ctx context.Context) (twitchIdentity, error) {
	token, err := GetOrRefreshTwitchToken(ctx, a.store, a.config().TwitchClientID, a.logger)
	if err != nil || token.AccessToken == "" {
		return twitchIdentity{}, errTwitchNotConnected
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, twitchValidateURL, nil)
	req.Header.Set("Authorization", "OAuth "+token.AccessToken)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return twitchIdentity{}, err
	}
	body := readAllAndClose(res.Body)
	if res.StatusCode == http.StatusUnauthorized {
		return twitchIdentity{}, errTwitchNotConnected
	}
	var payload struct {
		UserID string   `json:"user_id"`
		Scopes []string `json:"scopes"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || payload.UserID == "" {
		return twitchIdentity{}, fmt.Errorf("twitch token validation failed: %s", body)
	}
	return twitchIdentity{token: token.AccessToken, userID: payload.UserID, scopes: payload.Scopes}, nil
}

func (id twitchIdentity) canEditBroadcast() bool {
	return slices.Contains(id.scopes, twitchBroadcastScope)
}

func (a *App) twitchHelix(ctx context.Context, id twitchIdentity, method, path string, query url.Values, body, out any) error {
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return err
		}
	}
	req, _ := http.NewRequestWithContext(ctx, method, twitchHelixURL+path+"?"+query.Encode(), bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+id.token)
	req.Header.Set("Client-Id", a.config().TwitchClientID)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	raw := readAllAndClose(res.Body)
	if res.StatusCode/100 != 2 {
		var apiErr struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(raw, &apiErr)
		if strings.Contains(strings.ToLower(apiErr.Message), "scope") {
			return errTwitchMissingScope
		}
		if apiErr.Message != "" {
			return fmt.Errorf("twitch %s %s: %s", method, path, apiErr.Message)
		}
		return fmt.Errorf("twitch %s %s: HTTP %d", method, path, res.StatusCode)
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func twitchBoxArt(raw string) string {
	raw = strings.ReplaceAll(raw, "{width}x{height}", "104x144")
	return strings.Replace(raw, "-52x72.", "-104x144.", 1)
}

type twitchChannelInfo struct {
	Title string      `json:"title"`
	Game  *StreamGame `json:"game,omitempty"`
	Tags  []string    `json:"tags"`
	Live  bool        `json:"live"`
}

func (a *App) twitchChannelInfo(ctx context.Context, id twitchIdentity) (twitchChannelInfo, error) {
	var channels struct {
		Data []struct {
			Title    string   `json:"title"`
			GameID   string   `json:"game_id"`
			GameName string   `json:"game_name"`
			Tags     []string `json:"tags"`
		} `json:"data"`
	}
	if err := a.twitchHelix(ctx, id, http.MethodGet, "/channels", url.Values{"broadcaster_id": {id.userID}}, nil, &channels); err != nil {
		return twitchChannelInfo{}, err
	}
	if len(channels.Data) == 0 {
		return twitchChannelInfo{}, fmt.Errorf("twitch channel not found")
	}
	ch := channels.Data[0]
	info := twitchChannelInfo{Title: ch.Title, Tags: ch.Tags}
	if info.Tags == nil {
		info.Tags = []string{}
	}
	var streams struct {
		Data []struct{} `json:"data"`
	}
	if err := a.twitchHelix(ctx, id, http.MethodGet, "/streams", url.Values{"user_id": {id.userID}}, nil, &streams); err == nil {
		info.Live = len(streams.Data) > 0
	}
	if ch.GameID != "" {
		info.Game = &StreamGame{ID: ch.GameID, Name: ch.GameName}
		var games struct {
			Data []struct {
				BoxArtURL string `json:"box_art_url"`
			} `json:"data"`
		}
		if err := a.twitchHelix(ctx, id, http.MethodGet, "/games", url.Values{"id": {ch.GameID}}, nil, &games); err == nil && len(games.Data) > 0 {
			info.Game.BoxArtURL = twitchBoxArt(games.Data[0].BoxArtURL)
		}
	}
	return info, nil
}

// updateTwitchChannel leaves tags untouched when nil; an empty slice clears them.
func (a *App) updateTwitchChannel(ctx context.Context, title string, game *StreamGame, tags []string) platformResult {
	if utf8.RuneCountInString(title) > twitchTitleLimit {
		return platformResult{Code: "title_too_long", Error: fmt.Sprintf("Twitch admite hasta %d caracteres", twitchTitleLimit)}
	}
	id, err := a.twitchIdentity(ctx)
	if err != nil {
		return resultFromError(err)
	}
	if !id.canEditBroadcast() {
		return resultFromError(errTwitchMissingScope)
	}
	patch := map[string]any{}
	if title != "" {
		patch["title"] = title
	}
	if game != nil {
		patch["game_id"] = game.ID
	}
	if tags != nil {
		patch["tags"] = tags
	}
	if err := a.twitchHelix(ctx, id, http.MethodPatch, "/channels", url.Values{"broadcaster_id": {id.userID}}, patch, nil); err != nil {
		return resultFromError(err)
	}
	return platformResult{OK: true}
}

func (a *App) handleStreamInfoGames(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	games := []StreamGame{}
	if query == "" {
		writeJSON(w, games)
		return
	}
	id, err := a.twitchIdentity(r.Context())
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		writeJSON(w, resultFromError(err))
		return
	}
	var payload struct {
		Data []struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			BoxArtURL string `json:"box_art_url"`
		} `json:"data"`
	}
	if err := a.twitchHelix(r.Context(), id, http.MethodGet, "/search/categories", url.Values{"query": {query}, "first": {"8"}}, nil, &payload); err != nil {
		w.WriteHeader(http.StatusBadGateway)
		writeJSON(w, resultFromError(err))
		return
	}
	for _, g := range payload.Data {
		games = append(games, StreamGame{ID: g.ID, Name: g.Name, BoxArtURL: twitchBoxArt(g.BoxArtURL)})
	}
	writeJSON(w, games)
}

// ---- YouTube ----

type youtubeBroadcast struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"` // "live" or "upcoming"
}

type youtubePending struct {
	Title  string `json:"title"`
	tags   []string
	gaming bool
	cancel context.CancelFunc
}

// currentYouTubeBroadcast prefers the live broadcast; otherwise the next scheduled one.
func (a *App) currentYouTubeBroadcast(ctx context.Context) (*youtubeBroadcast, error) {
	for _, status := range []string{"active", "upcoming"} {
		var payload struct {
			Items []struct {
				ID      string `json:"id"`
				Snippet struct {
					Title              string `json:"title"`
					ScheduledStartTime string `json:"scheduledStartTime"`
				} `json:"snippet"`
			} `json:"items"`
		}
		q := url.Values{"part": {"snippet"}, "broadcastStatus": {status}, "broadcastType": {"all"}, "maxResults": {"10"}}
		if err := a.youtubeAPI(ctx, http.MethodGet, "/liveBroadcasts", q, nil, &payload); err != nil {
			return nil, err
		}
		if len(payload.Items) == 0 {
			continue
		}
		best := payload.Items[0]
		for _, item := range payload.Items[1:] {
			// RFC 3339 UTC timestamps sort lexically.
			if item.Snippet.ScheduledStartTime < best.Snippet.ScheduledStartTime {
				best = item
			}
		}
		label := "live"
		if status == "upcoming" {
			label = "upcoming"
		}
		return &youtubeBroadcast{ID: best.ID, Title: best.Snippet.Title, Status: label}, nil
	}
	return nil, nil
}

// updateYouTubeVideo rewrites the broadcast's video snippet. videos.update replaces the whole
// snippet, so description and tags are carried over to avoid wiping them; nil tags keep the
// current ones.
func (a *App) updateYouTubeVideo(ctx context.Context, videoID, title string, tags []string, gaming bool) error {
	var payload struct {
		Items []struct {
			Snippet map[string]any `json:"snippet"`
		} `json:"items"`
	}
	if err := a.youtubeAPI(ctx, http.MethodGet, "/videos", url.Values{"part": {"snippet"}, "id": {videoID}}, nil, &payload); err != nil {
		return err
	}
	if len(payload.Items) == 0 {
		return fmt.Errorf("youtube video %s not found", videoID)
	}
	current := payload.Items[0].Snippet
	snippet := map[string]any{"title": title}
	if title == "" {
		snippet["title"] = current["title"]
	}
	for _, key := range []string{"description", "tags", "categoryId", "defaultLanguage", "defaultAudioLanguage"} {
		if v, ok := current[key]; ok {
			snippet[key] = v
		}
	}
	if tags != nil {
		snippet["tags"] = tags
	}
	if gaming || snippet["categoryId"] == nil {
		snippet["categoryId"] = youtubeGamingCategory
	}
	return a.youtubeAPI(ctx, http.MethodPut, "/videos", url.Values{"part": {"snippet"}}, map[string]any{"id": videoID, "snippet": snippet}, nil)
}

func validateYouTubeTitle(title string) error {
	if utf8.RuneCountInString(title) > youtubeTitleLimit {
		return fmt.Errorf("YouTube admite hasta %d caracteres", youtubeTitleLimit)
	}
	if strings.ContainsAny(title, "<>") {
		return fmt.Errorf("YouTube no acepta < ni > en el título")
	}
	return nil
}

func (a *App) updateYouTube(ctx context.Context, title string, tags []string, gaming bool) platformResult {
	if title == "" && tags == nil {
		return platformResult{OK: true, State: "skipped"}
	}
	if err := validateYouTubeTitle(title); err != nil {
		return platformResult{Code: "title_too_long", Error: err.Error()}
	}
	broadcast, err := a.currentYouTubeBroadcast(ctx)
	if err != nil {
		return resultFromError(err)
	}
	if broadcast == nil {
		a.scheduleYouTubePending(title, tags, gaming)
		return platformResult{OK: true, State: "pending"}
	}
	if err := a.updateYouTubeVideo(ctx, broadcast.ID, title, tags, gaming); err != nil {
		return resultFromError(err)
	}
	a.cancelYouTubePending()
	return platformResult{OK: true, State: broadcast.Status}
}

func (a *App) cancelYouTubePending() {
	a.ytPendingMu.Lock()
	defer a.ytPendingMu.Unlock()
	if a.ytPending != nil {
		a.ytPending.cancel()
		a.ytPending = nil
	}
}

func (a *App) youtubePendingTitle() string {
	a.ytPendingMu.Lock()
	defer a.ytPendingMu.Unlock()
	if a.ytPending == nil {
		return ""
	}
	return a.ytPending.Title
}

// scheduleYouTubePending applies the title once a broadcast shows up (e.g. when OBS starts
// streaming with a stream key, YouTube only creates the broadcast at go-live).
func (a *App) scheduleYouTubePending(title string, tags []string, gaming bool) {
	ctx, cancel := context.WithTimeout(context.Background(), youtubePendingLifetime)
	pending := &youtubePending{Title: title, tags: tags, gaming: gaming, cancel: cancel}
	a.ytPendingMu.Lock()
	if a.ytPending != nil {
		a.ytPending.cancel()
	}
	a.ytPending = pending
	a.ytPendingMu.Unlock()

	go func() {
		defer func() {
			a.ytPendingMu.Lock()
			if a.ytPending == pending {
				a.ytPending = nil
			}
			a.ytPendingMu.Unlock()
			cancel()
		}()
		ticker := time.NewTicker(youtubePendingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			broadcast, err := a.currentYouTubeBroadcast(ctx)
			if errors.Is(err, errYouTubeNotConnected) {
				a.logger.Warn("dropping pending youtube title: youtube disconnected")
				return
			}
			if err != nil || broadcast == nil {
				continue
			}
			if err := a.updateYouTubeVideo(ctx, broadcast.ID, title, tags, gaming); err != nil {
				a.logger.Warn("pending youtube title update failed", "error", err)
				continue
			}
			a.logger.Info("applied pending youtube title", "video", broadcast.ID)
			return
		}
	}()
}

// ---- HTTP ----

func (a *App) handleStreamInfo(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		a.getStreamInfo(w, r)
	case http.MethodPost:
		a.applyStreamInfo(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *App) getStreamInfo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	type twitchStatus struct {
		platformResult
		CanEdit bool `json:"canEdit"`
		twitchChannelInfo
	}
	type youtubeStatus struct {
		platformResult
		Broadcast *youtubeBroadcast `json:"broadcast,omitempty"`
		Pending   string            `json:"pending,omitempty"`
	}
	var tw twitchStatus
	var yt youtubeStatus
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		id, err := a.twitchIdentity(ctx)
		if err != nil {
			tw.platformResult = resultFromError(err)
			return
		}
		tw.CanEdit = id.canEditBroadcast()
		info, err := a.twitchChannelInfo(ctx, id)
		if err != nil {
			tw.platformResult = resultFromError(err)
			return
		}
		tw.OK, tw.twitchChannelInfo = true, info
	}()
	go func() {
		defer wg.Done()
		broadcast, err := a.currentYouTubeBroadcast(ctx)
		if err != nil {
			yt.platformResult = resultFromError(err)
			return
		}
		yt.OK, yt.Broadcast, yt.Pending = true, broadcast, a.youtubePendingTitle()
	}()
	wg.Wait()
	recent := []streamInfoPreset{}
	_ = a.store.WidgetJSON(ctx, streamInfoRecentKey, &recent)
	writeJSON(w, map[string]any{"twitch": tw, "youtube": yt, "recent": recent})
}

func (a *App) applyStreamInfo(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title   string      `json:"title"`
		Game    *StreamGame `json:"game"`
		Tags    *[]string   `json:"tags"` // absent = leave the platforms' tags alone
		Twitch  bool        `json:"twitch"`
		YouTube bool        `json:"youtube"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	title := strings.Join(strings.Fields(body.Title), " ")
	if body.Game != nil && body.Game.ID == "" {
		body.Game = nil
	}
	var tags []string
	if body.Tags != nil {
		tags = cleanTags(*body.Tags)
	}
	if title == "" && body.Game == nil && tags == nil {
		http.Error(w, "title, game or tags required", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	results := map[string]platformResult{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	run := func(name string, enabled bool, fn func() platformResult) {
		if !enabled {
			return
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			res := fn()
			mu.Lock()
			results[name] = res
			mu.Unlock()
		}()
	}
	run("twitch", body.Twitch, func() platformResult { return a.updateTwitchChannel(ctx, title, body.Game, tags) })
	run("youtube", body.YouTube, func() platformResult { return a.updateYouTube(ctx, title, tags, body.Game != nil) })
	wg.Wait()

	for _, res := range results {
		if res.OK && title != "" {
			a.rememberStreamInfo(ctx, streamInfoPreset{Title: title, Game: body.Game, Tags: tags})
			break
		}
	}
	writeJSON(w, results)
}

func (a *App) rememberStreamInfo(ctx context.Context, preset streamInfoPreset) {
	recent := []streamInfoPreset{}
	_ = a.store.WidgetJSON(ctx, streamInfoRecentKey, &recent)
	gameID := func(p streamInfoPreset) string {
		if p.Game == nil {
			return ""
		}
		return p.Game.ID
	}
	recent = slices.DeleteFunc(recent, func(p streamInfoPreset) bool {
		return p.Title == preset.Title && gameID(p) == gameID(preset)
	})
	recent = append([]streamInfoPreset{preset}, recent...)
	if len(recent) > streamInfoRecentLimit {
		recent = recent[:streamInfoRecentLimit]
	}
	if err := a.store.SaveWidgetJSON(ctx, streamInfoRecentKey, recent); err != nil {
		a.logger.Warn("failed to save stream info history", "error", err)
	}
}
