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

func TestAlertEventEnabledRequiresEnabledConfiguration(t *testing.T) {
	cfg := DefaultConfig()
	if !alertEventEnabled(Event{Type: "gift"}, cfg) {
		t.Fatal("configured gift should publish an alert")
	}
	disabled := cfg.AlertsConfig["gift"]
	disabled.Enabled = false
	cfg.AlertsConfig["gift"] = disabled
	if alertEventEnabled(Event{Type: "gift"}, cfg) {
		t.Fatal("disabled gift should not publish an alert")
	}
	if alertEventEnabled(Event{Type: "like"}, cfg) {
		t.Fatal("unconfigured likes should not flood the alert overlay")
	}
}

func TestEventShouldPersistSkipsHighFrequencyLikes(t *testing.T) {
	if eventShouldPersist(Event{Type: "like"}) {
		t.Fatal("likes should remain transient")
	}
	if !eventShouldPersist(Event{Type: "gift"}) || !eventShouldPersist(Event{Type: "share"}) {
		t.Fatal("support and social events should persist")
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
