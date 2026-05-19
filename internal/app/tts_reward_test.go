package app

import "testing"

func TestTTSRewardTextUsesCommandStyleMessage(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TTSRewardEnabled = true
	cfg.TTSRewardName = "Leer mensaje"
	evt := Event{
		Type:       PlatformEventChannelPoints,
		User:       "Viewer",
		Message:    "hola desde puntos",
		RewardName: "leer mensaje",
	}

	text, ok := ttsRewardText(evt, cfg)
	if !ok {
		t.Fatal("expected channel points reward to trigger TTS")
	}
	if text != "Viewer dice: hola desde puntos" {
		t.Fatalf("unexpected reward TTS text %q", text)
	}
}

func TestTTSRewardTextRejectsDifferentReward(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TTSRewardEnabled = true
	cfg.TTSRewardName = "Leer mensaje"
	evt := Event{
		Type:       PlatformEventChannelPoints,
		User:       "Viewer",
		Message:    "hola",
		RewardName: "Otro reward",
	}

	if text, ok := ttsRewardText(evt, cfg); ok {
		t.Fatalf("unexpected reward TTS text %q", text)
	}
}

func TestTTSRewardTextRequiresUserInput(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TTSRewardEnabled = true
	evt := Event{
		Type:       PlatformEventChannelPoints,
		User:       "Viewer",
		RewardName: "Leer mensaje",
	}

	if text, ok := ttsRewardText(evt, cfg); ok {
		t.Fatalf("unexpected reward TTS text %q", text)
	}
}

func TestTTSRewardTextStripsCommandPrefixFromRewardInput(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TTSRewardEnabled = true
	cfg.TTSRewardName = "Leer mensaje"
	evt := Event{
		Type:       PlatformEventChannelPoints,
		User:       "Viewer",
		Message:    "!voz hola desde puntos",
		RewardName: "Leer mensaje",
	}

	text, ok := ttsRewardText(evt, cfg)
	if !ok {
		t.Fatal("expected channel points reward to trigger TTS")
	}
	if text != "Viewer dice: hola desde puntos" {
		t.Fatalf("unexpected reward TTS text %q", text)
	}
}
