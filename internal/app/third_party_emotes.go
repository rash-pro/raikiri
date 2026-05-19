package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

const thirdPartyEmoteCacheTTL = 15 * time.Minute

type thirdPartyEmote struct {
	Code     string
	URL      string
	Provider string
}

type thirdPartyEmoteCache struct {
	mu       sync.Mutex
	roomID   string
	channel  string
	loadedAt time.Time
	emotes   map[string]thirdPartyEmote
}

func newThirdPartyEmoteCache() *thirdPartyEmoteCache {
	return &thirdPartyEmoteCache{}
}

func (c *thirdPartyEmoteCache) Emotes(ctx context.Context, roomID, channel string, logger *slog.Logger) map[string]thirdPartyEmote {
	roomID = strings.TrimSpace(roomID)
	channel = strings.ToLower(strings.TrimSpace(channel))
	now := time.Now()

	c.mu.Lock()
	if c.emotes != nil && c.roomID == roomID && c.channel == channel && now.Sub(c.loadedAt) < thirdPartyEmoteCacheTTL {
		emotes := cloneThirdPartyEmotes(c.emotes)
		c.mu.Unlock()
		return emotes
	}
	c.mu.Unlock()

	emotes, err := fetchThirdPartyEmotes(ctx, roomID, channel)
	if err != nil && logger != nil {
		logger.Warn("third-party emote fetch failed", "error", err)
	}

	c.mu.Lock()
	c.roomID = roomID
	c.channel = channel
	c.loadedAt = now
	c.emotes = emotes
	cached := cloneThirdPartyEmotes(c.emotes)
	c.mu.Unlock()

	return cached
}

func cloneThirdPartyEmotes(emotes map[string]thirdPartyEmote) map[string]thirdPartyEmote {
	if len(emotes) == 0 {
		return nil
	}
	clone := make(map[string]thirdPartyEmote, len(emotes))
	for code, emote := range emotes {
		clone[code] = emote
	}
	return clone
}

func fetchThirdPartyEmotes(ctx context.Context, roomID, channel string) (map[string]thirdPartyEmote, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	client := &http.Client{Timeout: 4 * time.Second}
	emotes := map[string]thirdPartyEmote{}
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []string
	var results []struct {
		priority int
		emotes   map[string]thirdPartyEmote
	}

	addProvider := func(name string, priority int, fetch func(context.Context, *http.Client) (map[string]thirdPartyEmote, error)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			providerEmotes, err := fetch(ctx, client)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, name+": "+err.Error())
				return
			}
			results = append(results, struct {
				priority int
				emotes   map[string]thirdPartyEmote
			}{priority: priority, emotes: providerEmotes})
		}()
	}

	addProvider("ffz_global", 10, fetchFFZGlobalEmotes)
	addProvider("bttv_global", 20, fetchBTTVGlobalEmotes)
	addProvider("7tv_global", 30, fetchSevenTVGlobalEmotes)
	if roomID != "" {
		addProvider("bttv_channel", 120, func(ctx context.Context, client *http.Client) (map[string]thirdPartyEmote, error) {
			return fetchBTTVChannelEmotes(ctx, client, roomID)
		})
		addProvider("7tv_channel", 130, func(ctx context.Context, client *http.Client) (map[string]thirdPartyEmote, error) {
			return fetchSevenTVChannelEmotes(ctx, client, roomID)
		})
	}
	if channel != "" {
		addProvider("ffz_channel", 110, func(ctx context.Context, client *http.Client) (map[string]thirdPartyEmote, error) {
			return fetchFFZChannelEmotes(ctx, client, channel)
		})
	}

	wg.Wait()
	sort.Slice(results, func(i, j int) bool {
		return results[i].priority < results[j].priority
	})
	for _, result := range results {
		for code, emote := range result.emotes {
			emotes[code] = emote
		}
	}
	if len(emotes) == 0 && len(errs) > 0 {
		return emotes, fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return emotes, nil
}

func fetchJSON(ctx context.Context, client *http.Client, url string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Raikiri/1.0")
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return nil
	}
	if res.StatusCode/100 != 2 {
		return fmt.Errorf("http %d", res.StatusCode)
	}
	return json.NewDecoder(res.Body).Decode(dest)
}

func fetchBTTVGlobalEmotes(ctx context.Context, client *http.Client) (map[string]thirdPartyEmote, error) {
	var payload []bttvEmote
	if err := fetchJSON(ctx, client, "https://api.betterttv.net/3/cached/emotes/global", &payload); err != nil {
		return nil, err
	}
	return parseBTTVEmotes(payload), nil
}

func fetchBTTVChannelEmotes(ctx context.Context, client *http.Client, roomID string) (map[string]thirdPartyEmote, error) {
	var payload struct {
		ChannelEmotes []bttvEmote `json:"channelEmotes"`
		SharedEmotes  []bttvEmote `json:"sharedEmotes"`
	}
	if err := fetchJSON(ctx, client, "https://api.betterttv.net/3/cached/users/twitch/"+roomID, &payload); err != nil {
		return nil, err
	}
	return parseBTTVEmotes(append(payload.ChannelEmotes, payload.SharedEmotes...)), nil
}

type bttvEmote struct {
	ID   string `json:"id"`
	Code string `json:"code"`
}

func parseBTTVEmotes(items []bttvEmote) map[string]thirdPartyEmote {
	emotes := map[string]thirdPartyEmote{}
	for _, item := range items {
		if item.ID == "" || item.Code == "" {
			continue
		}
		emotes[item.Code] = thirdPartyEmote{
			Code:     item.Code,
			URL:      "https://cdn.betterttv.net/emote/" + item.ID + "/3x",
			Provider: "bttv",
		}
	}
	return emotes
}

func fetchFFZGlobalEmotes(ctx context.Context, client *http.Client) (map[string]thirdPartyEmote, error) {
	var payload ffzEmotePayload
	if err := fetchJSON(ctx, client, "https://api.frankerfacez.com/v1/set/global", &payload); err != nil {
		return nil, err
	}
	return parseFFZEmotes(payload), nil
}

func fetchFFZChannelEmotes(ctx context.Context, client *http.Client, channel string) (map[string]thirdPartyEmote, error) {
	var payload ffzEmotePayload
	if err := fetchJSON(ctx, client, "https://api.frankerfacez.com/v1/room/"+channel, &payload); err != nil {
		return nil, err
	}
	return parseFFZEmotes(payload), nil
}

type ffzEmotePayload struct {
	Sets map[string]struct {
		Emoticons []struct {
			Name string            `json:"name"`
			URLs map[string]string `json:"urls"`
		} `json:"emoticons"`
	} `json:"sets"`
}

func parseFFZEmotes(payload ffzEmotePayload) map[string]thirdPartyEmote {
	emotes := map[string]thirdPartyEmote{}
	for _, set := range payload.Sets {
		for _, item := range set.Emoticons {
			url := item.URLs["4"]
			if url == "" {
				url = item.URLs["2"]
			}
			if url == "" {
				url = item.URLs["1"]
			}
			if item.Name == "" || url == "" {
				continue
			}
			if strings.HasPrefix(url, "//") {
				url = "https:" + url
			}
			emotes[item.Name] = thirdPartyEmote{Code: item.Name, URL: url, Provider: "ffz"}
		}
	}
	return emotes
}

func fetchSevenTVGlobalEmotes(ctx context.Context, client *http.Client) (map[string]thirdPartyEmote, error) {
	var payload sevenTVEmoteSet
	if err := fetchJSON(ctx, client, "https://7tv.io/v3/emote-sets/global", &payload); err != nil {
		return nil, err
	}
	return parseSevenTVEmoteSet(payload), nil
}

func fetchSevenTVChannelEmotes(ctx context.Context, client *http.Client, roomID string) (map[string]thirdPartyEmote, error) {
	var payload struct {
		EmoteSet sevenTVEmoteSet `json:"emote_set"`
	}
	if err := fetchJSON(ctx, client, "https://7tv.io/v3/users/twitch/"+roomID, &payload); err != nil {
		return nil, err
	}
	return parseSevenTVEmoteSet(payload.EmoteSet), nil
}

type sevenTVEmoteSet struct {
	Emotes []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	} `json:"emotes"`
}

func parseSevenTVEmoteSet(payload sevenTVEmoteSet) map[string]thirdPartyEmote {
	emotes := map[string]thirdPartyEmote{}
	for _, item := range payload.Emotes {
		id := item.Data.ID
		if id == "" {
			id = item.ID
		}
		if id == "" || item.Name == "" {
			continue
		}
		emotes[item.Name] = thirdPartyEmote{
			Code:     item.Name,
			URL:      "https://cdn.7tv.app/emote/" + id + "/3x.webp",
			Provider: "7tv",
		}
	}
	return emotes
}
