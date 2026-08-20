//go:build integration

package tiktoklive

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestLiveConnectionAndTraffic(t *testing.T) {
	username := os.Getenv("RAIKIRI_TIKTOK_LIVE_USER")
	if username == "" {
		t.Skip("set RAIKIRI_TIKTOK_LIVE_USER to a currently LIVE TikTok creator")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	connected := false
	traffic := false
	done := make(chan error, 1)
	go func() {
		done <- New(nil).Run(ctx, username, func(event Event) {
			switch event.Kind {
			case EventConnected:
				connected = true
			case EventChat, EventGift, EventFollow, EventLike, EventShare:
				traffic = true
				cancel()
			}
		})
	}()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !connected {
		t.Fatal("TikTok WebSocket did not connect")
	}
	if !traffic {
		t.Fatal("TikTok WebSocket connected but delivered no traffic before timeout")
	}
}
