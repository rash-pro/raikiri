package app

import (
	"encoding/json"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"raikiri/internal/steamach"
)

type localAchievement struct {
	ID          string `json:"id"`
	Game        string `json:"game"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	Unlocked    int    `json:"unlocked"`
	Total       int    `json:"total"`
}

var localNoticeID = regexp.MustCompile(`^[a-zA-Z0-9._:-]{1,128}$`)

// Local achievements use the existing toast and sound without writing Steam state.
func (a *App) handleLocalAchievement(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || !net.ParseIP(host).IsLoopback() {
		http.Error(w, "local connections only", http.StatusForbidden)
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+r.Host {
		http.Error(w, "same-origin requests only", http.StatusForbidden)
		return
	}
	mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mediaType != "application/json" {
		http.Error(w, "expected application/json", http.StatusUnsupportedMediaType)
		return
	}
	var notice localAchievement
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&notice); err != nil || decoder.Decode(new(any)) != io.EOF {
		http.Error(w, "invalid notification", http.StatusBadRequest)
		return
	}
	if !localNoticeID.MatchString(notice.ID) || strings.TrimSpace(notice.Title) == "" ||
		strings.TrimSpace(notice.Game) == "" || len(notice.Title) > 200 || len(notice.Game) > 200 ||
		len(notice.Description) > 1000 || notice.Total < 1 || notice.Unlocked < 1 || notice.Unlocked > notice.Total {
		http.Error(w, "invalid achievement fields", http.StatusBadRequest)
		return
	}
	if notice.Icon != "" {
		icon, err := url.Parse(notice.Icon)
		if err != nil || icon.Scheme != "http" || icon.User != nil ||
			(icon.Hostname() != "127.0.0.1" && icon.Hostname() != "localhost") || !strings.HasPrefix(icon.Path, "/icons/") {
			http.Error(w, "expected a local tracker icon", http.StatusBadRequest)
			return
		}
	}
	var seen int
	if err := a.store.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM local_achievement_notices WHERE id=?`, notice.ID).Scan(&seen); err != nil {
		http.Error(w, "cannot read notification receipt", http.StatusInternalServerError)
		return
	}
	if seen != 0 {
		writeJSON(w, map[string]any{"success": true, "duplicate": true})
		return
	}
	clients := a.hub.Count("widgets")
	if clients == 0 {
		http.Error(w, "waiting for the achievements overlay", http.StatusServiceUnavailable)
		return
	}
	result, err := a.store.db.ExecContext(r.Context(), `INSERT OR IGNORE INTO local_achievement_notices(id) VALUES (?)`, notice.ID)
	if err != nil {
		http.Error(w, "cannot save notification receipt", http.StatusInternalServerError)
		return
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		http.Error(w, "cannot read notification receipt", http.StatusInternalServerError)
		return
	}
	if inserted != 0 {
		a.publishAchievement(steamach.Event{
			Game: notice.Game, APIName: notice.ID,
			Names: map[string]string{"english": notice.Title, "spanish": notice.Title},
			Descs: map[string]string{"english": notice.Description, "spanish": notice.Description},
			Icon:  notice.Icon, Unlocked: notice.Unlocked, Total: notice.Total,
			Time: time.Now().Unix(), Source: "local-video",
		})
	}
	writeJSON(w, map[string]any{"success": true, "duplicate": inserted == 0, "widgetClients": clients})
}
