package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	tthttp "raikiri/internal/tiktoklive/internal/web"
)

const ttwidRegisterURL = "https://ttwid.bytedance.com/ttwid/union/register/"

// FetchTTWID registers an anonymous web device and extracts the ttwid cookie
// from the Set-Cookie response header.
// The userAgent parameter overrides the default random UA when non-empty.
// The proxy parameter sets an HTTP/HTTPS proxy when non-empty.
func FetchTTWID(ctx context.Context, timeout time.Duration, userAgent string, proxy string) (string, error) {
	transport := &http.Transport{Proxy: http.ProxyFromEnvironment}
	if proxy != "" {
		proxyURL, err := url.Parse(proxy)
		if err != nil {
			return "", fmt.Errorf("ttwid: invalid proxy URL: %w", err)
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	}
	client := &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
	return fetchTTWID(ctx, client, ttwidRegisterURL, userAgent)
}

func fetchTTWID(ctx context.Context, client *http.Client, endpoint string, userAgent string) (string, error) {
	payload, err := json.Marshal(map[string]any{
		"region": "va", "aid": 1768, "needFid": false, "service": "www.tiktok.com",
		"migrate_info":  map[string]string{"ticket": "", "source": "node"},
		"cbUrlProtocol": "https", "union": true,
	})
	if err != nil {
		return "", fmt.Errorf("ttwid: encode registration: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("ttwid: build request: %w", err)
	}

	ua := userAgent
	if ua == "" {
		ua = tthttp.RandomUA()
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/plain, */*")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ttwid: register device: %w", err)
	}
	defer resp.Body.Close()

	for _, cookie := range resp.Cookies() {
		if cookie.Name == "ttwid" {
			value := strings.TrimSpace(cookie.Value)
			if value == "" {
				return "", fmt.Errorf("ttwid: empty ttwid cookie value")
			}
			return value, nil
		}
	}
	return "", fmt.Errorf("ttwid: no ttwid cookie in response (status %d)", resp.StatusCode)
}
