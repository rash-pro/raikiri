package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchTTWIDRegistersAnonymousWebDevice(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("content type = %q, want application/json", got)
		}
		var payload struct {
			Region        string `json:"region"`
			Aid           int    `json:"aid"`
			Service       string `json:"service"`
			Union         bool   `json:"union"`
			CBURLProtocol string `json:"cbUrlProtocol"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.Region != "va" || payload.Aid != 1768 || payload.Service != "www.tiktok.com" || !payload.Union || payload.CBURLProtocol != "https" {
			t.Fatalf("unexpected registration payload: %#v", payload)
		}
		http.SetCookie(w, &http.Cookie{Name: "ttwid", Value: "device-cookie", Path: "/"})
		_, _ = w.Write([]byte(`{"status_code":0}`))
	}))
	defer server.Close()

	got, err := fetchTTWID(context.Background(), server.Client(), server.URL, "test-agent")
	if err != nil {
		t.Fatal(err)
	}
	if got != "device-cookie" {
		t.Fatalf("ttwid = %q, want device-cookie", got)
	}
}

func TestFetchTTWIDRejectsResponseWithoutCookie(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status_code":0}`))
	}))
	defer server.Close()

	if _, err := fetchTTWID(context.Background(), server.Client(), server.URL, "test-agent"); err == nil {
		t.Fatal("response without ttwid cookie should fail")
	}
}
