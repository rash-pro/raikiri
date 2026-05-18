package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type YouTubeWebAdapter struct {
	id     string
	logger *slog.Logger
	chat   func(ChatMessage)
	event  func(Event)
	cancel context.CancelFunc
	seen   map[string]struct{}
}

type ytOptions struct {
	LiveID        string
	APIKey        string
	ClientVersion string
	Continuation  string
}

func NewYouTubeWebAdapter(id string, logger *slog.Logger, chat func(ChatMessage), event func(Event)) *YouTubeWebAdapter {
	return &YouTubeWebAdapter{id: id, logger: logger.With("adapter", "youtube_web"), chat: chat, event: event, seen: map[string]struct{}{}}
}

func (a *YouTubeWebAdapter) Start(ctx context.Context) {
	runCtx, cancel := context.WithCancel(ctx)
	a.cancel = cancel
	for {
		if err := a.run(runCtx); err != nil {
			a.logger.Warn("youtube poller stopped", "error", err)
		}
		select {
		case <-runCtx.Done():
			return
		case <-time.After(10 * time.Second):
		}
	}
}

func (a *YouTubeWebAdapter) Stop() {
	if a.cancel != nil {
		a.cancel()
	}
}

func (a *YouTubeWebAdapter) run(ctx context.Context) error {
	opts, err := a.fetchLiveOptions(ctx)
	if err != nil {
		return err
	}
	a.logger.Info("youtube live chat connected", "liveId", opts.LiveID)
	delay := time.Second
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		items, next, timeoutMs, err := a.fetchChat(ctx, opts)
		if err != nil {
			delay = minDuration(delay*2, 60*time.Second)
			a.logger.Warn("youtube chat fetch failed", "error", err, "retry", delay)
			continue
		}
		delay = time.Duration(timeoutMs) * time.Millisecond
		if delay <= 0 {
			delay = time.Second
		}
		opts.Continuation = next
		for _, item := range items {
			if _, ok := a.seen[item.ID]; ok {
				continue
			}
			a.seen[item.ID] = struct{}{}
			msg := ChatMessage{
				ID: item.ID, Platform: PlatformYouTube, User: item.Author, DisplayName: item.Author,
				Content: item.Text, HTMLContent: item.HTML, Badges: item.Badges, Timestamp: item.Timestamp,
			}
			a.chat(msg)
			if item.SuperchatAmount != "" {
				a.event(Event{Type: "superchat", Platform: PlatformYouTube, User: item.Author, Amount: item.SuperchatAmount, Message: item.Text})
			}
		}
	}
}

func (a *YouTubeWebAdapter) fetchLiveOptions(ctx context.Context) (ytOptions, error) {
	url := youtubeLiveURL(a.id)
	opt, err := a.fetchLiveOptionsFromURL(ctx, url, true)
	if err == nil {
		return opt, nil
	}
	if fallback := youtubeLiveChatURL(a.id); fallback != "" {
		fallbackOpt, fallbackErr := a.fetchLiveOptionsFromURL(ctx, fallback, false)
		if fallbackErr == nil {
			if fallbackOpt.LiveID == "" {
				fallbackOpt.LiveID = a.id
			}
			return fallbackOpt, nil
		}
		return ytOptions{}, fmt.Errorf("%w; live chat fallback failed: %v", err, fallbackErr)
	}
	return ytOptions{}, err
}

func (a *YouTubeWebAdapter) fetchLiveOptionsFromURL(ctx context.Context, url string, requireLiveID bool) (ytOptions, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("User-Agent", browserUA)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return ytOptions{}, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	page := string(body)
	if strings.Contains(page, "Our systems have detected unusual traffic") || strings.Contains(page, "g-recaptcha") {
		return ytOptions{}, fmt.Errorf("youtube returned captcha/unusual traffic page")
	}
	if strings.Contains(page, `"isReplay":true`) {
		return ytOptions{}, fmt.Errorf("youtube live is already finished")
	}
	opt := ytOptions{
		LiveID:        firstMatch(page, `<link rel="canonical" href="https://www.youtube.com/watch\?v=(.+?)">`),
		APIKey:        firstMatch(page, `["']INNERTUBE_API_KEY["']:\s*["'](.+?)["']`),
		ClientVersion: firstMatch(page, `["']clientVersion["']:\s*["']([\d.]+?)["']`),
		Continuation:  firstMatch(page, `["']continuation["']:\s*["'](.+?)["']`),
	}
	if (requireLiveID && opt.LiveID == "") || opt.APIKey == "" || opt.ClientVersion == "" || opt.Continuation == "" {
		return opt, fmt.Errorf("youtube live bootstrap missing required fields")
	}
	return opt, nil
}

func (a *YouTubeWebAdapter) fetchChat(ctx context.Context, opt ytOptions) ([]ytItem, string, int, error) {
	payload := map[string]any{
		"context":      map[string]any{"client": map[string]any{"clientName": "WEB", "clientVersion": opt.ClientVersion}},
		"continuation": opt.Continuation,
	}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://www.youtube.com/youtubei/v1/live_chat/get_live_chat?key="+opt.APIKey, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", browserUA)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", 0, err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode/100 != 2 {
		return nil, "", 0, fmt.Errorf("youtube chat status %d: %s", res.StatusCode, raw)
	}
	return parseYouTubeChat(raw)
}

type ytItem struct {
	ID              string
	Author          string
	Text            string
	HTML            string
	Badges          []Badge
	Timestamp       time.Time
	SuperchatAmount string
}

func parseYouTubeChat(raw []byte) ([]ytItem, string, int, error) {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, "", 0, err
	}
	cont := dig(payload, "continuationContents", "liveChatContinuation").(map[string]any)
	next, timeout := parseYTContinuation(cont["continuations"])
	var items []ytItem
	for _, action := range toSlice(cont["actions"]) {
		actionMap, _ := action.(map[string]any)
		add, _ := actionMap["addChatItemAction"].(map[string]any)
		item, _ := add["item"].(map[string]any)
		renderer, kind := firstRenderer(item)
		if renderer == nil {
			continue
		}
		yt := ytItem{
			ID: fmt.Sprint(renderer["id"]), Author: simpleText(renderer["authorName"]),
			HTML: runsHTML(renderer["message"]), Text: runsText(renderer["message"]), Timestamp: time.Now(),
		}
		if ts := fmt.Sprint(renderer["timestampUsec"]); ts != "" && ts != "<nil>" {
			var usec int64
			fmt.Sscanf(ts, "%d", &usec)
			if usec > 0 {
				yt.Timestamp = time.UnixMicro(usec)
			}
		}
		yt.Badges = youtubeBadges(renderer["authorBadges"])
		if kind == "liveChatPaidMessageRenderer" || kind == "liveChatPaidStickerRenderer" {
			yt.SuperchatAmount = simpleText(renderer["purchaseAmountText"])
		}
		items = append(items, yt)
	}
	return items, next, timeout, nil
}

func firstRenderer(item map[string]any) (map[string]any, string) {
	for _, key := range []string{"liveChatTextMessageRenderer", "liveChatPaidMessageRenderer", "liveChatPaidStickerRenderer", "liveChatMembershipItemRenderer"} {
		if r, ok := item[key].(map[string]any); ok {
			return r, key
		}
	}
	return nil, ""
}

func parseYTContinuation(raw any) (string, int) {
	for _, c := range toSlice(raw) {
		m, _ := c.(map[string]any)
		for _, key := range []string{"invalidationContinuationData", "timedContinuationData"} {
			if data, ok := m[key].(map[string]any); ok {
				return fmt.Sprint(data["continuation"]), intNumber(data["timeoutMs"])
			}
		}
	}
	return "", 1000
}

func runsHTML(raw any) string {
	var out strings.Builder
	runs, _ := raw.(map[string]any)
	for _, run := range toSlice(runs["runs"]) {
		m, _ := run.(map[string]any)
		if text, ok := m["text"].(string); ok {
			out.WriteString(sanitizeText(text))
			continue
		}
		emoji, _ := m["emoji"].(map[string]any)
		img := bestThumb(digOK(emoji, "image", "thumbnails"))
		alt := fmt.Sprint(emoji["emojiId"])
		if img != "" {
			out.WriteString(`<img src="` + sanitizeText(img) + `" class="emote" alt="` + sanitizeText(alt) + `">`)
		}
	}
	return out.String()
}

func runsText(raw any) string {
	var out strings.Builder
	runs, _ := raw.(map[string]any)
	for _, run := range toSlice(runs["runs"]) {
		m, _ := run.(map[string]any)
		if text, ok := m["text"].(string); ok {
			out.WriteString(text)
		} else if emoji, ok := m["emoji"].(map[string]any); ok {
			out.WriteString(fmt.Sprint(emoji["emojiId"]))
		}
	}
	return out.String()
}

func youtubeBadges(raw any) []Badge {
	var badges []Badge
	for _, item := range toSlice(raw) {
		m, _ := item.(map[string]any)
		r, _ := m["liveChatAuthorBadgeRenderer"].(map[string]any)
		icon, _ := r["icon"].(map[string]any)
		switch icon["iconType"] {
		case "OWNER":
			badges = append(badges, Badge{Type: "owner"})
		case "MODERATOR":
			badges = append(badges, Badge{Type: "moderator"})
		}
		if _, ok := r["customThumbnail"]; ok {
			badges = append(badges, Badge{Type: "subscriber"})
		}
	}
	return badges
}

func youtubeLiveURL(id string) string {
	if strings.HasPrefix(id, "UC") {
		return "https://www.youtube.com/channel/" + id + "/live"
	}
	if strings.HasPrefix(id, "@") {
		return "https://www.youtube.com/" + id + "/live"
	}
	if strings.Contains(id, "youtube.com") {
		return id
	}
	return "https://www.youtube.com/watch?v=" + id
}

func youtubeLiveChatURL(id string) string {
	if strings.HasPrefix(id, "UC") || strings.HasPrefix(id, "@") || strings.Contains(id, "youtube.com") {
		return ""
	}
	return "https://www.youtube.com/live_chat?is_popout=1&v=" + id
}

func firstMatch(s, pattern string) string {
	m := regexp.MustCompile(pattern).FindStringSubmatch(s)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

func simpleText(raw any) string {
	m, _ := raw.(map[string]any)
	if text, ok := m["simpleText"].(string); ok {
		return text
	}
	return runsText(raw)
}

func bestThumb(raw any) string {
	var best string
	for _, thumb := range toSlice(raw) {
		m, _ := thumb.(map[string]any)
		if url, ok := m["url"].(string); ok {
			best = url
		}
	}
	return best
}

func dig(root map[string]any, keys ...string) any {
	var cur any = root
	for _, key := range keys {
		m, _ := cur.(map[string]any)
		cur = m[key]
	}
	return cur
}

func digOK(root map[string]any, keys ...string) any {
	if root == nil {
		return nil
	}
	return dig(root, keys...)
}

func toSlice(raw any) []any {
	if raw == nil {
		return nil
	}
	s, _ := raw.([]any)
	return s
}

func intNumber(raw any) int {
	switch v := raw.(type) {
	case float64:
		return int(v)
	case int:
		return v
	default:
		return 1000
	}
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

const browserUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36"
