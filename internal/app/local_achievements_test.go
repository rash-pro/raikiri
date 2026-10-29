package app

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"raikiri/internal/steamach"
)

const localNotice = `{"id":"ps1-test-shiva","game":"Final Fantasy VIII · PS1","title":"Shiva","description":"Obtuviste a Shiva.","icon":"http://127.0.0.1:30012/icons/gf-shiva.png","unlocked":2,"total":17}`

func TestLocalAchievementUsesExistingWidgetEventAndDeduplicates(t *testing.T) {
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	a := &App{store: store, hub: NewHub(slog.Default()), logger: slog.Default()}
	mux := http.NewServeMux()
	a.routes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/ws/widgets", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	for a.hub.Count("widgets") == 0 {
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		default:
			time.Sleep(time.Millisecond)
		}
	}
	post := func() *http.Response {
		res, err := http.Post(server.URL+"/api/widgets/achievements/local", "application/json", strings.NewReader(localNotice))
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	res := post()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	res.Body.Close()
	_, body, err := conn.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var event struct {
		Event string         `json:"event"`
		Data  steamach.Event `json:"data"`
	}
	if err := json.Unmarshal(body, &event); err != nil {
		t.Fatal(err)
	}
	if event.Event != "achievement_unlocked" || event.Data.Names["spanish"] != "Shiva" || event.Data.Source != "local-video" || event.Data.AppID != 0 || event.Data.Total != 17 {
		t.Fatalf("wrong event: %+v", event)
	}
	res = post()
	defer res.Body.Close()
	var receipt struct {
		Success   bool `json:"success"`
		Duplicate bool `json:"duplicate"`
	}
	if err := json.NewDecoder(res.Body).Decode(&receipt); err != nil {
		t.Fatal(err)
	}
	if !receipt.Success || !receipt.Duplicate {
		t.Fatalf("wrong receipt: %+v", receipt)
	}
	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM local_achievement_notices`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("got %d receipts", count)
	}
}

func TestLocalAchievementBoundaryAndUnavailableOverlay(t *testing.T) {
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	a := &App{store: store, hub: NewHub(slog.Default()), logger: slog.Default()}
	for _, tc := range []struct {
		name, remote, origin, content, body string
		want                                int
	}{
		{"no overlay", "127.0.0.1:1234", "", "application/json", localNotice, 503},
		{"remote host", "192.0.2.1:1234", "", "application/json", localNotice, 403},
		{"cross origin", "127.0.0.1:1234", "https://example.org", "application/json", localNotice, 403},
		{"wrong content", "127.0.0.1:1234", "", "text/plain", localNotice, 415},
		{"invalid count", "127.0.0.1:1234", "", "application/json", strings.Replace(localNotice, `"total":17`, `"total":1`, 1), 400},
		{"external image", "127.0.0.1:1234", "", "application/json", strings.Replace(localNotice, "127.0.0.1:30012", "example.org", 1), 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/api/widgets/achievements/local", strings.NewReader(tc.body))
			r.RemoteAddr = tc.remote
			r.Header.Set("Origin", tc.origin)
			r.Header.Set("Content-Type", tc.content)
			w := httptest.NewRecorder()
			a.handleLocalAchievement(w, r)
			if w.Code != tc.want {
				t.Fatalf("status %d, expected %d: %s", w.Code, tc.want, w.Body.String())
			}
		})
	}
	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM local_achievement_notices`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("rejected notices must remain retryable")
	}
}
