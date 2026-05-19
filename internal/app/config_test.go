package app

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestApplyConfigPatchPreservesMissingValues(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TwitchChannel = "oldchannel"
	cfg.TTSEnabled = true
	cfg.TTSBlockedWords = "old\nwords"
	cfg.ChatFontSize = 15

	var patch map[string]json.RawMessage
	if err := json.Unmarshal([]byte(`{"youtubeChannelId":"abc123","chatFontSize":22,"ttsBlockedWords":"nuevo\nbloqueo"}`), &patch); err != nil {
		t.Fatal(err)
	}
	if err := applyConfigPatch(&cfg, patch); err != nil {
		t.Fatal(err)
	}
	if cfg.TwitchChannel != "oldchannel" {
		t.Fatalf("missing string field was overwritten: %q", cfg.TwitchChannel)
	}
	if !cfg.TTSEnabled {
		t.Fatal("missing bool field was overwritten")
	}
	if cfg.YouTubeID != "abc123" || cfg.ChatFontSize != 22 {
		t.Fatalf("patch did not apply: %#v", cfg)
	}
	if cfg.TTSBlockedWords != "nuevo\nbloqueo" {
		t.Fatalf("blocked words patch did not apply: %q", cfg.TTSBlockedWords)
	}
}

func TestConfigFormEndpointPersistsPatch(t *testing.T) {
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	cfg := DefaultConfig()
	app := &App{store: store, cfg: cfg, hub: NewHub(slog.Default()), logger: slog.Default()}

	form := url.Values{}
	form.Set("twitchChannel", "rashpro0")
	form.Set("youtubeChannelId", "abc123")
	form.Set("ttsEnabled", "false")
	form.Set("ttsBlockedWords", "uno\ndos")
	form.Set("chatFontSize", "24")
	form.Set("alertsConfig", `{"follow":{"enabled":true,"theme":"cyberpurple","voice":"","gifUrl":"","audioUrl":"","messageTemplate":"hi {user}"}}`)

	req := httptest.NewRequest(http.MethodPost, "/api/config/form", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res := httptest.NewRecorder()
	app.handleConfigForm(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", res.Code, res.Body.String())
	}

	got, err := store.LoadConfig(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.TwitchChannel != "rashpro0" || got.YouTubeID != "abc123" || got.TTSEnabled || got.ChatFontSize != 24 || got.TTSBlockedWords != "uno\ndos" {
		t.Fatalf("form config did not persist: %#v", got)
	}
}
