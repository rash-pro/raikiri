package tiktoklive

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"raikiri/internal/tiktoklive/internal/events"
	pb "raikiri/internal/tiktoklive/internal/protocol"
)

type rawSourceFunc func(context.Context) (<-chan events.Event, error)

func (f rawSourceFunc) Connect(ctx context.Context) (<-chan events.Event, error) {
	return f(ctx)
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestRunEmitsNormalizedEventsThroughPublicInterface(t *testing.T) {
	fixedTime := time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC)
	common := func(id int64) *pb.Common { return &pb.Common{MsgId: id, CreateTime: fixedTime.UnixMilli()} }
	user := &pb.User{
		Id: 42, UniqueId: "creator", Nickname: "Creator Name", IsSubscribe: true,
		BadgeList: []*pb.BadgeStruct{{Text: &pb.BadgeStruct_TextBadge{DefaultPattern: "Moderator"}}},
	}
	rawEvents := make(chan events.Event, 10)
	rawEvents <- events.Event{Type: events.EventConnected, RoomID: "123"}
	rawEvents <- events.Event{Type: events.EventChat, Data: &pb.WebcastChatMessage{Common: common(1), User: user, Content: "hola"}}
	rawEvents <- events.Event{Type: events.EventGift, Data: &pb.WebcastGiftMessage{
		Common: common(2), GiftId: 5655, GroupId: 88, RepeatCount: 2,
		User: user, Gift: &pb.GiftStruct{Name: "Rose", Type: 1, DiamondCount: 5},
	}}
	rawEvents <- events.Event{Type: events.EventGift, Data: &pb.WebcastGiftMessage{
		Common: common(3), GiftId: 5655, GroupId: 88, RepeatCount: 3, RepeatEnd: 1,
		User: user, Gift: &pb.GiftStruct{Name: "Rose", Type: 1, DiamondCount: 5},
	}}
	rawEvents <- events.Event{Type: events.EventFollow, Data: &pb.WebcastSocialMessage{Common: common(4), User: user}}
	rawEvents <- events.Event{Type: events.EventLike, Data: &pb.WebcastLikeMessage{Common: common(5), User: user, Count: 7, Total: 99}}
	rawEvents <- events.Event{Type: events.EventShare, Data: &pb.WebcastSocialMessage{Common: common(6), User: user}}
	rawEvents <- events.Event{Type: events.EventLiveEnded, RoomID: "123"}
	close(rawEvents)

	ctx, cancel := context.WithCancel(context.Background())
	client := New(testLogger())
	client.now = func() time.Time { return fixedTime }
	client.retryWait = time.Hour
	client.newSource = func(username string) rawSource {
		if username != "creator" {
			t.Fatalf("source username = %q, want creator", username)
		}
		return rawSourceFunc(func(context.Context) (<-chan events.Event, error) { return rawEvents, nil })
	}

	var got []Event
	err := client.Run(ctx, "https://www.tiktok.com/@creator/live", func(event Event) {
		got = append(got, event)
		if len(got) == 7 {
			cancel()
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 7 {
		t.Fatalf("got %d events, want 7: %#v", len(got), got)
	}
	wantKinds := []EventKind{EventConnected, EventChat, EventGift, EventFollow, EventLike, EventShare, EventLiveEnded}
	for i, want := range wantKinds {
		if got[i].Kind != want {
			t.Fatalf("event %d kind = %q, want %q", i, got[i].Kind, want)
		}
	}
	chat := got[1]
	if chat.ID != "1" || chat.Text != "hola" || chat.User.Username != "creator" || chat.User.DisplayName != "Creator Name" || !chat.At.Equal(fixedTime) {
		t.Fatalf("unexpected chat event: %#v", chat)
	}
	wantBadges := []string{"owner", "subscriber", "moderator"}
	if len(chat.User.Badges) != len(wantBadges) {
		t.Fatalf("chat badges = %#v, want %v", chat.User.Badges, wantBadges)
	}
	for i, want := range wantBadges {
		if chat.User.Badges[i].Type != want {
			t.Fatalf("badge %d = %q, want %q", i, chat.User.Badges[i].Type, want)
		}
	}
	gift := got[2]
	if gift.GiftName != "Rose" || gift.Count != 3 || gift.Value != 15 {
		t.Fatalf("unexpected final gift streak: %#v", gift)
	}
	like := got[4]
	if like.Count != 7 || like.Total != 99 {
		t.Fatalf("unexpected like event: %#v", like)
	}
}

func TestRunRetriesRemoteFailuresUntilCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client := New(testLogger())
	client.retryWait = time.Millisecond
	attempts := 0
	client.newSource = func(string) rawSource {
		attempts++
		if attempts == 3 {
			cancel()
		}
		return rawSourceFunc(func(context.Context) (<-chan events.Event, error) {
			return nil, errors.New("creator is offline")
		})
	}
	if err := client.Run(ctx, "@creator", func(Event) {}); err != nil {
		t.Fatal(err)
	}
	if attempts != 3 {
		t.Fatalf("connect attempts = %d, want 3", attempts)
	}
}

func TestRunValidatesItsInterface(t *testing.T) {
	client := New(testLogger())
	if err := client.Run(context.Background(), "", func(Event) {}); err == nil {
		t.Fatal("empty username should fail")
	}
	if err := client.Run(context.Background(), "creator", nil); err == nil {
		t.Fatal("nil callback should fail")
	}
}

func TestNormalizeUsername(t *testing.T) {
	tests := map[string]string{
		"creator":                              "creator",
		" @Creator ":                           "Creator",
		"https://www.tiktok.com/@creator/live": "creator",
	}
	for input, want := range tests {
		if got := NormalizeUsername(input); got != want {
			t.Errorf("NormalizeUsername(%q) = %q, want %q", input, got, want)
		}
	}
}
