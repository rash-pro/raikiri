package app

import "testing"

func TestTTSCommandTextStripsConfiguredCommand(t *testing.T) {
	text, ok := ttsCommandText("!voz hola como estas", "!voz")
	if !ok {
		t.Fatal("command was not detected")
	}
	if text != "hola como estas" {
		t.Fatalf("unexpected command text %q", text)
	}
}

func TestTTSCommandTextIsCaseInsensitive(t *testing.T) {
	text, ok := ttsCommandText(" !VOZ Hola ", "!voz")
	if !ok {
		t.Fatal("command was not detected")
	}
	if text != "Hola" {
		t.Fatalf("unexpected command text %q", text)
	}
}

func TestTTSCommandTextRejectsDifferentCommand(t *testing.T) {
	if text, ok := ttsCommandText("!tts hola", "!voz"); ok {
		t.Fatalf("unexpected command match with text %q", text)
	}
}

func TestTTSCommandTextRejectsPrefixOnlyMatch(t *testing.T) {
	if text, ok := ttsCommandText("!vozhola", "!voz"); ok {
		t.Fatalf("unexpected command match with text %q", text)
	}
}

func TestTTSCommandAllowedMatchesConfiguredBadges(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TTSCmdHost = false
	cfg.TTSCmdMod = false
	cfg.TTSCmdSub = true
	cfg.TTSCmdVip = false

	msg := ChatMessage{Badges: []Badge{{Type: "SUBSCRIBER"}}}
	if !ttsCommandAllowed(msg, cfg) {
		t.Fatal("subscriber should be allowed when subscriber TTS command is enabled")
	}
}

func TestTTSCommandAllowedRejectsUnconfiguredBadges(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TTSCmdHost = false
	cfg.TTSCmdMod = true
	cfg.TTSCmdSub = false
	cfg.TTSCmdVip = false

	msg := ChatMessage{Badges: []Badge{{Type: "subscriber"}, {Type: "vip"}}}
	if ttsCommandAllowed(msg, cfg) {
		t.Fatal("subscriber/vip should not be allowed when only moderators are enabled")
	}
}

func TestTTSCommandAllowedRejectsUsersWithoutBadges(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TTSCmdHost = true
	cfg.TTSCmdMod = true
	cfg.TTSCmdSub = true
	cfg.TTSCmdVip = true

	if ttsCommandAllowed(ChatMessage{}, cfg) {
		t.Fatal("users without role badges should not be allowed")
	}
}
