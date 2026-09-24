package app

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Confidential Twitch apps reject refresh requests without client_secret, which left
// Raikiri with a dead token after every restart once the ~4h access token expired.
func TestGetOrRefreshTwitchTokenSendsClientSecret(t *testing.T) {
	var gotSecret string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotSecret = r.PostForm.Get("client_secret")
		if gotSecret == "" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"status":400,"message":"missing client secret"}`))
			return
		}
		_, _ = w.Write([]byte(`{"access_token":"new-access","refresh_token":"new-refresh","expires_in":14400}`))
	}))
	defer srv.Close()
	prev := twitchTokenURL
	twitchTokenURL = srv.URL
	defer func() { twitchTokenURL = prev }()

	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	expired := TokenData{AccessToken: "old-access", RefreshToken: "old-refresh", ExpiresAt: time.Now().Add(-time.Hour).UnixMilli()}
	if err := store.SaveToken(ctx, "twitch", expired); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSecret(ctx, twitchClientSecretKey, "shh"); err != nil {
		t.Fatal(err)
	}

	tok, err := GetOrRefreshTwitchToken(ctx, store, "client", slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("refresh failed: %v", err)
	}
	if gotSecret != "shh" || tok.AccessToken != "new-access" {
		t.Fatalf("expected refreshed token using the stored secret, got secret=%q token=%q", gotSecret, tok.AccessToken)
	}
	saved, _ := store.Token(ctx, "twitch")
	if saved.AccessToken != "new-access" || saved.RefreshToken != "new-refresh" {
		t.Fatalf("refreshed token not persisted: %#v", saved)
	}
}
