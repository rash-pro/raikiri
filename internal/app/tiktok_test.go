package app

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"raikiri/internal/tiktoklive"
)

type tikTokLiveRunnerFunc func(context.Context, string, func(tiktoklive.Event)) error

func (f tikTokLiveRunnerFunc) Run(ctx context.Context, username string, emit func(tiktoklive.Event)) error {
	return f(ctx, username, emit)
}

func TestTikTokAdapterMapsPublicModuleEvents(t *testing.T) {
	createdAt := time.Date(2026, time.August, 20, 12, 30, 0, 0, time.UTC)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	var chats []ChatMessage
	var appEvents []Event
	adapter := NewTikTokAdapter("https://www.tiktok.com/@creator/live", logger, func(message ChatMessage) {
		chats = append(chats, message)
	}, func(event Event) {
		appEvents = append(appEvents, event)
	})
	adapter.client = tikTokLiveRunnerFunc(func(_ context.Context, username string, emit func(tiktoklive.Event)) error {
		if username != adapter.username {
			t.Fatalf("runner username = %q, want configured value %q", username, adapter.username)
		}
		emit(tiktoklive.Event{
			Kind: tiktoklive.EventChat, ID: "987", At: createdAt,
			User: tiktoklive.User{Username: "creator", DisplayName: "Creator Name", Badges: []tiktoklive.Badge{{Type: "owner"}, {Type: "subscriber"}}},
			Text: "hola <script>alert(1)</script>",
		})
		emit(tiktoklive.Event{
			Kind: tiktoklive.EventGift, User: tiktoklive.User{Username: "viewer"},
			GiftName: "Rose", Count: 3, Value: 15,
		})
		emit(tiktoklive.Event{Kind: tiktoklive.EventLike, User: tiktoklive.User{Username: "fan"}, Count: 7, Total: 99})
		return nil
	})

	adapter.Start(context.Background())
	if len(chats) != 1 {
		t.Fatalf("got %d chat messages, want 1", len(chats))
	}
	chat := chats[0]
	if chat.ID != "987" || chat.Platform != PlatformTikTok || chat.User != "creator" || chat.DisplayName != "Creator Name" {
		t.Fatalf("unexpected chat identity: %#v", chat)
	}
	if chat.Content != "hola alert(1)" || chat.HTMLContent != "hola alert(1)" || !chat.Timestamp.Equal(createdAt) {
		t.Fatalf("unexpected chat content: %#v", chat)
	}
	if len(chat.Badges) != 2 || chat.Badges[0].Type != "owner" || chat.Badges[1].Type != "subscriber" {
		t.Fatalf("unexpected chat badges: %#v", chat.Badges)
	}
	if len(appEvents) != 2 {
		t.Fatalf("got %d app events, want 2", len(appEvents))
	}
	gift := appEvents[0]
	if gift.Type != "gift" || gift.Platform != PlatformTikTok || gift.User != "viewer" || gift.GiftName != "Rose" || gift.Count != int64(3) || gift.Amount != int64(15) || gift.Currency != "diamonds" {
		t.Fatalf("unexpected gift event: %#v", gift)
	}
	like := appEvents[1]
	if like.Type != "like" || like.User != "fan" || like.Count != int64(7) || like.Amount != int64(7) {
		t.Fatalf("unexpected like event: %#v", like)
	}
}

func TestTikTokAdapterStopCancelsModuleRun(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	adapter := NewTikTokAdapter("creator", logger, func(ChatMessage) {}, func(Event) {})
	started := make(chan struct{})
	stopped := make(chan struct{})
	adapter.client = tikTokLiveRunnerFunc(func(ctx context.Context, _ string, _ func(tiktoklive.Event)) error {
		close(started)
		<-ctx.Done()
		close(stopped)
		return nil
	})

	go adapter.Start(context.Background())
	<-started
	adapter.Stop()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("adapter did not cancel module run")
	}
}
