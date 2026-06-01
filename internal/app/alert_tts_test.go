package app

import "testing"

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
