package app

import (
	"context"
	"testing"
)

func TestStoreConfigRoundTrip(t *testing.T) {
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	cfg := DefaultConfig()
	cfg.TwitchClientID = "client"
	cfg.TwitchChannel = "channel"
	cfg.YouTubeID = "@handle"
	cfg.TTSEnabled = false
	cfg.ChatFontSize = 22

	if err := store.SaveConfig(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	got, err := store.LoadConfig(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.TwitchClientID != "client" || got.TwitchChannel != "channel" || got.YouTubeID != "@handle" {
		t.Fatalf("config did not round-trip: %#v", got)
	}
	if got.TTSEnabled || got.ChatFontSize != 22 {
		t.Fatalf("typed config did not round-trip: %#v", got)
	}
}
