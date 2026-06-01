package app

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAlertEventTriggersTTSForConfiguredAlertTypes(t *testing.T) {
	cfg := DefaultConfig()

	for _, eventType := range []string{"follow", "raid", "bits", "subscription"} {
		if !alertEventTriggersTTS(Event{Type: eventType}, cfg) {
			t.Fatalf("%s should trigger alert TTS", eventType)
		}
	}
}

func TestAlertEventTriggersTTSSkipsRewardAndUnknownTypes(t *testing.T) {
	cfg := DefaultConfig()

	if alertEventTriggersTTS(Event{Type: PlatformEventChannelPoints}, cfg) {
		t.Fatal("channel points reward TTS is handled separately")
	}
	if alertEventTriggersTTS(Event{Type: "unknown"}, cfg) {
		t.Fatal("unknown event type should not trigger alert TTS")
	}
}

func TestHandleAlertTestDoesNotPersistEvent(t *testing.T) {
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	cfg := DefaultConfig()
	cfg.TTSEnabled = false
	hub := NewHub(slog.Default())
	app := &App{store: store, cfg: cfg, hub: hub, logger: slog.Default()}
	app.tts = NewTTSEngine(slog.Default(), func(AudioPayload) {})

	req := httptest.NewRequest(http.MethodPost, "/api/alerts/test", strings.NewReader(`{"type":"follow"}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	app.handleAlertTest(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", res.Code, res.Body.String())
	}
	var payload map[string]any
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload["audioClients"].(float64) != 0 {
		t.Fatalf("unexpected audio client count: %#v", payload)
	}
	events, err := store.RecentEvents(context.Background(), nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("alert test persisted events: %#v", events)
	}
}
