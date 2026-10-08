package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	youtubeClientIDKey     = "youtubeClientId"
	youtubeClientSecretKey = "youtubeClientSecret"
	youtubeChannelTitleKey = "youtubeChannelTitle"
	// The plain youtube scope (not youtube.force-ssl) is enough for videos.update and liveBroadcasts.list.
	youtubeScope = "https://www.googleapis.com/auth/youtube"
)

// Overridable in tests.
var (
	googleAuthURL  = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenURL = "https://oauth2.googleapis.com/token"
	youtubeAPIURL  = "https://www.googleapis.com/youtube/v3"
)

var errYouTubeNotConnected = errors.New("youtube not connected")

type youtubeOAuthState struct {
	verifier string
	expires  time.Time
}

func (a *App) youtubeCredentials(ctx context.Context) (string, string) {
	id, _ := a.store.Secret(ctx, youtubeClientIDKey)
	secret, _ := a.store.Secret(ctx, youtubeClientSecretKey)
	return id, secret
}

// Google "Desktop app" OAuth clients accept any loopback port, so the callback lands back on Raikiri.
func (a *App) youtubeRedirectURI() string {
	return fmt.Sprintf("http://127.0.0.1:%d/api/auth/youtube/callback", a.opts.Port)
}

func (a *App) handleYouTubeCredentials(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		ClientID     string `json:"clientId"`
		ClientSecret string `json:"clientSecret"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	oldID, _ := a.youtubeCredentials(ctx)
	newID := strings.TrimSpace(body.ClientID)
	if err := a.store.SaveSecret(ctx, youtubeClientIDKey, newID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// The secret is write-only: an empty field keeps the saved one.
	if secret := strings.TrimSpace(body.ClientSecret); secret != "" {
		if err := a.store.SaveSecret(ctx, youtubeClientSecretKey, secret); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if newID != oldID {
		// Tokens are bound to the OAuth client that issued them.
		_ = a.store.SaveToken(ctx, "youtube", TokenData{})
	}
	writeJSON(w, map[string]bool{"success": true})
}

func (a *App) handleYouTubeStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, secret := a.youtubeCredentials(ctx)
	_, err := a.youtubeToken(ctx)
	var channel string
	_ = a.store.WidgetJSON(ctx, youtubeChannelTitleKey, &channel)
	writeJSON(w, map[string]any{
		"clientId":        id,
		"hasClientSecret": secret != "",
		"authenticated":   err == nil,
		"channel":         channel,
		"redirectUri":     a.youtubeRedirectURI(),
	})
}

func (a *App) handleYouTubeAuthStart(w http.ResponseWriter, r *http.Request) {
	id, secret := a.youtubeCredentials(r.Context())
	if id == "" || secret == "" {
		http.Error(w, "Save the YouTube OAuth Client ID and Client Secret in the Raikiri dashboard first.", http.StatusBadRequest)
		return
	}
	a.ytOAuthStates.Range(func(key, value any) bool {
		if time.Now().After(value.(youtubeOAuthState).expires) {
			a.ytOAuthStates.Delete(key)
		}
		return true
	})
	state := randomHex(16)
	verifier := base64.RawURLEncoding.EncodeToString(randomBytes(32))
	a.ytOAuthStates.Store(state, youtubeOAuthState{verifier: verifier, expires: time.Now().Add(10 * time.Minute)})
	challenge := sha256.Sum256([]byte(verifier))
	q := url.Values{
		"client_id":             {id},
		"redirect_uri":          {a.youtubeRedirectURI()},
		"response_type":         {"code"},
		"scope":                 {youtubeScope},
		"access_type":           {"offline"},
		"prompt":                {"consent"},
		"state":                 {state},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(challenge[:])},
		"code_challenge_method": {"S256"},
	}
	http.Redirect(w, r, googleAuthURL+"?"+q.Encode(), http.StatusFound)
}

func (a *App) handleYouTubeAuthCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if msg := q.Get("error"); msg != "" {
		writeAuthPage(w, http.StatusBadRequest, "YouTube no se conectó: "+msg)
		return
	}
	raw, ok := a.ytOAuthStates.LoadAndDelete(q.Get("state"))
	if !ok || time.Now().After(raw.(youtubeOAuthState).expires) {
		writeAuthPage(w, http.StatusBadRequest, "La sesión de login expiró. Vuelve a intentarlo desde el dashboard.")
		return
	}
	id, secret := a.youtubeCredentials(r.Context())
	form := url.Values{
		"client_id":     {id},
		"client_secret": {secret},
		"code":          {q.Get("code")},
		"code_verifier": {raw.(youtubeOAuthState).verifier},
		"grant_type":    {"authorization_code"},
		"redirect_uri":  {a.youtubeRedirectURI()},
	}
	token, err := googleTokenRequest(r.Context(), form, "")
	if err != nil {
		a.logger.Warn("youtube auth failed", "error", err)
		writeAuthPage(w, http.StatusBadGateway, "Google rechazó el login: "+err.Error())
		return
	}
	a.ytTokenMu.Lock()
	err = a.store.SaveToken(r.Context(), "youtube", token)
	a.ytTokenMu.Unlock()
	if err != nil {
		writeAuthPage(w, http.StatusInternalServerError, err.Error())
		return
	}
	channel, err := a.youtubeChannelTitle(r.Context())
	if err != nil {
		a.logger.Warn("failed to read youtube channel", "error", err)
	}
	_ = a.store.SaveWidgetJSON(r.Context(), youtubeChannelTitleKey, channel)
	msg := "YouTube conectado. Ya puedes cerrar esta pestaña."
	if channel != "" {
		msg = "YouTube conectado como " + channel + ". Ya puedes cerrar esta pestaña."
	}
	writeAuthPage(w, http.StatusOK, msg)
}

func (a *App) youtubeToken(ctx context.Context) (TokenData, error) {
	a.ytTokenMu.Lock()
	defer a.ytTokenMu.Unlock()
	token, err := a.store.Token(ctx, "youtube")
	if err != nil || token.AccessToken == "" {
		return TokenData{}, errYouTubeNotConnected
	}
	if time.Now().UnixMilli() < token.ExpiresAt-5*60*1000 {
		return token, nil
	}
	if token.RefreshToken == "" {
		return TokenData{}, errYouTubeNotConnected
	}
	id, secret := a.youtubeCredentials(ctx)
	form := url.Values{
		"client_id":     {id},
		"client_secret": {secret},
		"refresh_token": {token.RefreshToken},
		"grant_type":    {"refresh_token"},
	}
	refreshed, err := googleTokenRequest(ctx, form, token.RefreshToken)
	if err != nil {
		a.logger.Warn("youtube token refresh failed", "error", err)
		if strings.Contains(err.Error(), "invalid_grant") {
			// Revoked or expired grant: forget it so we stop retrying until the user reconnects.
			_ = a.store.SaveToken(ctx, "youtube", TokenData{})
			return TokenData{}, errYouTubeNotConnected
		}
		return TokenData{}, err
	}
	return refreshed, a.store.SaveToken(ctx, "youtube", refreshed)
}

// googleTokenRequest posts to the token endpoint; refresh responses omit refresh_token, so the old one is kept.
func googleTokenRequest(ctx context.Context, form url.Values, refreshToken string) (TokenData, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, googleTokenURL, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return TokenData{}, err
	}
	body := readAllAndClose(res.Body)
	var payload struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		Scope        string `json:"scope"`
		Error        string `json:"error"`
		Description  string `json:"error_description"`
	}
	_ = json.Unmarshal(body, &payload)
	if res.StatusCode/100 != 2 || payload.AccessToken == "" {
		if payload.Error != "" {
			return TokenData{}, fmt.Errorf("%s: %s", payload.Error, payload.Description)
		}
		return TokenData{}, fmt.Errorf("token request failed: %s", body)
	}
	if payload.RefreshToken == "" {
		payload.RefreshToken = refreshToken
	}
	return TokenData{
		AccessToken:  payload.AccessToken,
		RefreshToken: payload.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(payload.ExpiresIn) * time.Second).UnixMilli(),
		Scope:        payload.Scope,
	}, nil
}

// youtubeAPI calls the YouTube Data API v3 and decodes the response into out (when non-nil).
func (a *App) youtubeAPI(ctx context.Context, method, path string, query url.Values, body, out any) error {
	token, err := a.youtubeToken(ctx)
	if err != nil {
		return err
	}
	var reader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, _ := http.NewRequestWithContext(ctx, method, youtubeAPIURL+path+"?"+query.Encode(), reader)
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	raw := readAllAndClose(res.Body)
	if res.StatusCode/100 != 2 {
		var apiErr struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(raw, &apiErr) == nil && apiErr.Error.Message != "" {
			return fmt.Errorf("youtube %s %s: %s", method, path, apiErr.Error.Message)
		}
		return fmt.Errorf("youtube %s %s: HTTP %d", method, path, res.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func (a *App) youtubeChannelTitle(ctx context.Context) (string, error) {
	var payload struct {
		Items []struct {
			Snippet struct {
				Title string `json:"title"`
			} `json:"snippet"`
		} `json:"items"`
	}
	if err := a.youtubeAPI(ctx, http.MethodGet, "/channels", url.Values{"part": {"snippet"}, "mine": {"true"}}, nil, &payload); err != nil {
		return "", err
	}
	if len(payload.Items) == 0 {
		return "", nil
	}
	return payload.Items[0].Snippet.Title, nil
}

func writeAuthPage(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	fmt.Fprintf(w, `<!doctype html><meta charset="utf-8"><title>Raikiri</title>`+
		`<body style="font:16px system-ui;background:#16161a;color:#ececf1;display:grid;place-items:center;height:100vh;margin:0">`+
		`<p>%s</p></body>`, html.EscapeString(msg))
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return b
}

func randomHex(n int) string { return hex.EncodeToString(randomBytes(n)) }
