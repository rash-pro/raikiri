package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newStreamInfoTestApp(t *testing.T) *App {
	t.Helper()
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	ctx := context.Background()
	future := time.Now().Add(time.Hour).UnixMilli()
	for _, platform := range []string{"twitch", "youtube"} {
		if err := store.SaveToken(ctx, platform, TokenData{AccessToken: platform + "-token", RefreshToken: "r", ExpiresAt: future}); err != nil {
			t.Fatal(err)
		}
	}
	return &App{store: store, logger: slog.New(slog.NewTextHandler(io.Discard, nil)), cfg: AppConfig{TwitchClientID: "cid"}}
}

func withURL(t *testing.T, target *string, value string) {
	prev := *target
	*target = value
	t.Cleanup(func() { *target = prev })
}

// videos.update replaces the whole snippet, so description and tags must survive a title change.
func TestApplyStreamInfoUpdatesBothPlatforms(t *testing.T) {
	app := newStreamInfoTestApp(t)
	var twitchPatch map[string]string
	var ytUpdate struct {
		ID      string         `json:"id"`
		Snippet map[string]any `json:"snippet"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/validate":
			_, _ = w.Write([]byte(`{"user_id":"42","scopes":["chat:read","channel:manage:broadcast"]}`))
		case r.URL.Path == "/helix/channels" && r.Method == http.MethodPatch:
			if r.URL.Query().Get("broadcaster_id") != "42" {
				t.Errorf("broadcaster_id = %q", r.URL.Query().Get("broadcaster_id"))
			}
			_ = json.NewDecoder(r.Body).Decode(&twitchPatch)
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == "/yt/liveBroadcasts":
			if r.URL.Query().Get("broadcastStatus") == "active" {
				_, _ = w.Write([]byte(`{"items":[{"id":"vid1","snippet":{"title":"old"}}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"items":[]}`))
		case r.URL.Path == "/yt/videos" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"items":[{"snippet":{"title":"old","description":"desc","tags":["a"],"categoryId":"22","channelTitle":"ro"}}]}`))
		case r.URL.Path == "/yt/videos" && r.Method == http.MethodPut:
			_ = json.NewDecoder(r.Body).Decode(&ytUpdate)
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	withURL(t, &twitchValidateURL, srv.URL+"/validate")
	withURL(t, &twitchHelixURL, srv.URL+"/helix")
	withURL(t, &youtubeAPIURL, srv.URL+"/yt")

	body := `{"title":"  Jugando   FF6 ","game":{"id":"858","name":"Final Fantasy VI"},"twitch":true,"youtube":true}`
	rec := httptest.NewRecorder()
	app.handleStreamInfo(rec, httptest.NewRequest(http.MethodPost, "/api/stream-info", strings.NewReader(body)))

	var results map[string]platformResult
	if err := json.Unmarshal(rec.Body.Bytes(), &results); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	if !results["twitch"].OK || !results["youtube"].OK || results["youtube"].State != "live" {
		t.Fatalf("results = %+v", results)
	}
	if twitchPatch["title"] != "Jugando FF6" || twitchPatch["game_id"] != "858" {
		t.Errorf("twitch patch = %v", twitchPatch)
	}
	s := ytUpdate.Snippet
	if ytUpdate.ID != "vid1" || s["title"] != "Jugando FF6" || s["description"] != "desc" || s["categoryId"] != "20" {
		t.Errorf("youtube update = %+v", ytUpdate)
	}
	if _, ok := s["channelTitle"]; ok {
		t.Errorf("read-only snippet field sent back: %v", s)
	}

	var recent []streamInfoPreset
	_ = app.store.WidgetJSON(context.Background(), streamInfoRecentKey, &recent)
	if len(recent) != 1 || recent[0].Title != "Jugando FF6" || recent[0].Game.ID != "858" {
		t.Errorf("recent = %+v", recent)
	}
}

func TestApplyStreamInfoReportsMissingTwitchScope(t *testing.T) {
	app := newStreamInfoTestApp(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"user_id":"42","scopes":["chat:read"]}`))
	}))
	defer srv.Close()
	withURL(t, &twitchValidateURL, srv.URL)

	rec := httptest.NewRecorder()
	app.handleStreamInfo(rec, httptest.NewRequest(http.MethodPost, "/api/stream-info", bytes.NewBufferString(`{"title":"x","twitch":true}`)))
	var results map[string]platformResult
	_ = json.Unmarshal(rec.Body.Bytes(), &results)
	if results["twitch"].OK || results["twitch"].Code != "missing_scope" {
		t.Fatalf("results = %+v", results)
	}
}

func TestYouTubeTitleWithoutBroadcastIsPending(t *testing.T) {
	app := newStreamInfoTestApp(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer srv.Close()
	withURL(t, &youtubeAPIURL, srv.URL)

	res := app.updateYouTube(context.Background(), "Nuevo título", nil, true)
	defer app.cancelYouTubePending()
	if !res.OK || res.State != "pending" || app.youtubePendingTitle() != "Nuevo título" {
		t.Fatalf("res = %+v, pending = %q", res, app.youtubePendingTitle())
	}
	if res := app.updateYouTube(context.Background(), strings.Repeat("a", 101), nil, false); res.OK {
		t.Fatalf("101-char title accepted: %+v", res)
	}
}

func TestCleanTagsFollowsTwitchRules(t *testing.T) {
	got := cleanTags([]string{"Final Fantasy", "#Español", "español", "", "100%", strings.Repeat("a", 26), "a", "b", "c", "d", "e", "f", "g", "h", "i"})
	want := []string{"FinalFantasy", "Español", "100", "a", "b", "c", "d", "e", "f", "g"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("cleanTags = %v, want %v", got, want)
	}
}

func TestApplyStreamInfoSendsTagsOnlyWhenGiven(t *testing.T) {
	app := newStreamInfoTestApp(t)
	var patches []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/validate" {
			_, _ = w.Write([]byte(`{"user_id":"42","scopes":["channel:manage:broadcast"]}`))
			return
		}
		var patch map[string]any
		_ = json.NewDecoder(r.Body).Decode(&patch)
		patches = append(patches, patch)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	withURL(t, &twitchValidateURL, srv.URL+"/validate")
	withURL(t, &twitchHelixURL, srv.URL+"/helix")

	for _, body := range []string{
		`{"title":"x","twitch":true}`,
		`{"tags":["RPG","Final Fantasy"],"twitch":true}`,
		`{"tags":[],"twitch":true}`,
	} {
		app.handleStreamInfo(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/stream-info", strings.NewReader(body)))
	}
	if len(patches) != 3 {
		t.Fatalf("patches = %v", patches)
	}
	if _, ok := patches[0]["tags"]; ok {
		t.Errorf("title-only patch touched tags: %v", patches[0])
	}
	if fmt.Sprint(patches[1]["tags"]) != "[RPG FinalFantasy]" {
		t.Errorf("tags patch = %v", patches[1])
	}
	if tags, ok := patches[2]["tags"].([]any); !ok || len(tags) != 0 {
		t.Errorf("clearing patch = %v", patches[2])
	}
}

func TestStreamSuggestUsesConfiguredAgent(t *testing.T) {
	app := newStreamInfoTestApp(t)
	app.cfg.SuggestAgent, app.cfg.SuggestModel = "codex", "gpt-x"
	withURL(t, &twitchValidateURL, "http://127.0.0.1:0") // Twitch unreachable: suggestions still work
	prev := runSuggestAgent
	t.Cleanup(func() { runSuggestAgent = prev })
	var gotAgent, gotModel string
	runSuggestAgent = func(_ context.Context, agent, model, _, _ string) ([]byte, error) {
		gotAgent, gotModel = agent, model
		return []byte(`{"titles":["  Uno   dos ", "` + strings.Repeat("x", 101) + `"],"tags":["Final Fantasy","RPG"]}`), nil
	}
	rec := httptest.NewRecorder()
	app.handleStreamInfoSuggest(rec, httptest.NewRequest(http.MethodPost, "/api/stream-info/suggest", strings.NewReader(`{"title":"x"}`)))
	var got streamSuggestion
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if gotAgent != "codex" || gotModel != "gpt-x" {
		t.Errorf("agent/model = %q/%q", gotAgent, gotModel)
	}
	if fmt.Sprint(got.Titles) != "[Uno dos]" || fmt.Sprint(got.Tags) != "[FinalFantasy RPG]" {
		t.Errorf("suggestion = %+v (%s)", got, rec.Body.String())
	}
}
