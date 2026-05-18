package app

import (
	"context"
	"testing"
	"time"
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
	cfg.WidgetsConfig.SupportGoal.Title = "Pizza Fund"
	cfg.WidgetsConfig.SupportGoal.TargetAmount = 250

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
	if got.WidgetsConfig.SupportGoal.Title != "Pizza Fund" || got.WidgetsConfig.SupportGoal.TargetAmount != 250 {
		t.Fatalf("widget config did not round-trip: %#v", got.WidgetsConfig)
	}
}

func TestStoreWidgetStateFromEvents(t *testing.T) {
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	store.SaveEvent(ctx, Event{Type: "superchat", Platform: PlatformYouTube, User: "Donor", Amount: "$5.50", Message: "Nice"})
	store.SaveEvent(ctx, Event{Type: "bits", Platform: PlatformTwitch, User: "Cheer", Amount: 100})
	store.SaveEvent(ctx, Event{Type: "raid", Platform: PlatformTwitch, User: "Raider", Viewers: 10})

	total, err := store.SumSupportSince(ctx, time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if total != 105.5 {
		t.Fatalf("unexpected support total: %v", total)
	}

	recent, err := store.RecentEvents(ctx, []string{"superchat", "raid"}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 2 || recent[0].Type != "raid" || recent[1].Type != "superchat" {
		t.Fatalf("unexpected recent events: %#v", recent)
	}
}

func TestAmountFloatParsesCurrencyText(t *testing.T) {
	cases := map[string]float64{
		"$5.50":      5.5,
		"MX$1,234.5": 1234.5,
		"€7,25":      7.25,
	}
	for input, want := range cases {
		if got := amountFloat(input); got != want {
			t.Fatalf("amountFloat(%q) = %v, want %v", input, got, want)
		}
	}
}
