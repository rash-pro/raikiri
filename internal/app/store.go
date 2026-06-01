package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type TokenData struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresAt    int64  `json:"expiresAt"`
	Scope        string `json:"scope"`
}

func OpenStore(dataDir string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dataDir, "media"), 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "audio"), 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "logs"), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", filepath.Join(dataDir, "raikiri.db"))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	store := &Store{db: db}
	if err := store.migrate(context.Background()); err != nil {
		_ = store.Close()
		return nil, err
	}
	return store, nil
}

func Migrate(dataDir string) error {
	store, err := OpenStore(dataDir)
	if err != nil {
		return err
	}
	defer store.Close()
	return nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate(ctx context.Context) error {
	queries := []string{
		`PRAGMA busy_timeout = 5000`,
		`PRAGMA journal_mode = WAL`,
		`PRAGMA synchronous = NORMAL`,
		`CREATE TABLE IF NOT EXISTS config (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS tokens (platform TEXT PRIMARY KEY, access_token TEXT NOT NULL, refresh_token TEXT, expires_at INTEGER, scope TEXT)`,
		`CREATE TABLE IF NOT EXISTS events (id INTEGER PRIMARY KEY AUTOINCREMENT, type TEXT NOT NULL, platform TEXT NOT NULL, data TEXT NOT NULL, created_at DATETIME DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE TABLE IF NOT EXISTS widget_state (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
	}
	for _, q := range queries {
		if _, err := s.db.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) LoadConfig(ctx context.Context) (AppConfig, error) {
	cfg := DefaultConfig()
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM config`)
	if err != nil {
		return cfg, err
	}
	defer rows.Close()

	values := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return cfg, err
		}
		values[key] = value
	}
	applyConfigValues(&cfg, values)
	return cfg, rows.Err()
}

func (s *Store) SaveConfig(ctx context.Context, cfg AppConfig) error {
	values := map[string]any{
		"twitchClientId": cfg.TwitchClientID, "twitchChannel": cfg.TwitchChannel, "youtubeChannelId": cfg.YouTubeID,
		"kickUsername": cfg.KickUsername, "tiktokUsername": cfg.TikTokUsername, "ttsEnabled": cfg.TTSEnabled,
		"ttsVoice": cfg.TTSVoice, "ttsMinBits": cfg.TTSMinBits, "ttsSubTier": cfg.TTSSubTier, "audioMode": cfg.AudioMode,
		"audioVolume": cfg.AudioVol, "ttsRewardEnabled": cfg.TTSRewardEnabled, "ttsRewardName": cfg.TTSRewardName,
		"ttsCmdEnabled": cfg.TTSCmdEnabled, "ttsCmdPrefix": cfg.TTSCmdPrefix, "ttsBlockedWords": cfg.TTSBlockedWords, "ttsCmdMod": cfg.TTSCmdMod,
		"ttsCmdSub": cfg.TTSCmdSub, "ttsCmdVip": cfg.TTSCmdVip, "ttsCmdHost": cfg.TTSCmdHost, "chatTheme": cfg.ChatTheme,
		"chatFontSize": cfg.ChatFontSize, "chatHideAfter": cfg.ChatHideAfter, "chatAnimations": cfg.ChatAnimations,
		"alertsConfig": cfg.AlertsConfig, "widgetsConfig": cfg.WidgetsConfig,
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for key, value := range values {
		b, err := json.Marshal(value)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO config (key, value) VALUES (?, ?)`, key, string(b)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) Token(ctx context.Context, platform string) (TokenData, error) {
	var t TokenData
	err := s.db.QueryRowContext(ctx, `SELECT access_token, COALESCE(refresh_token, ''), COALESCE(expires_at, 0), COALESCE(scope, '') FROM tokens WHERE platform = ?`, platform).
		Scan(&t.AccessToken, &t.RefreshToken, &t.ExpiresAt, &t.Scope)
	if errors.Is(err, sql.ErrNoRows) {
		return t, nil
	}
	return t, err
}

func (s *Store) SaveToken(ctx context.Context, platform string, t TokenData) error {
	_, err := s.db.ExecContext(ctx, `INSERT OR REPLACE INTO tokens (platform, access_token, refresh_token, expires_at, scope) VALUES (?, ?, ?, ?, ?)`,
		platform, t.AccessToken, t.RefreshToken, t.ExpiresAt, t.Scope)
	return err
}

func (s *Store) SaveEvent(ctx context.Context, evt Event) {
	data, _ := json.Marshal(evt)
	_, _ = s.db.ExecContext(ctx, `INSERT INTO events (type, platform, data) VALUES (?, ?, ?)`, evt.Type, string(evt.Platform), string(data))
}

func (s *Store) RecentEvents(ctx context.Context, types []string, limit int) ([]RecentEvent, error) {
	if limit <= 0 {
		limit = 8
	}
	rows, err := s.db.QueryContext(ctx, `SELECT data, created_at FROM events ORDER BY id DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	allowed := map[string]bool{}
	for _, t := range types {
		allowed[t] = true
	}
	var events []RecentEvent
	for rows.Next() {
		var raw string
		var createdRaw any
		if err := rows.Scan(&raw, &createdRaw); err != nil {
			return nil, err
		}
		created := parseSQLiteTime(createdRaw)
		var evt Event
		if err := json.Unmarshal([]byte(raw), &evt); err != nil {
			continue
		}
		if len(allowed) > 0 && !allowed[evt.Type] {
			continue
		}
		events = append(events, RecentEvent{
			Type: evt.Type, Platform: evt.Platform, User: evt.User, Amount: evt.Amount, Message: evt.Message, CreatedAt: created,
		})
		if len(events) >= limit {
			break
		}
	}
	return events, rows.Err()
}

func parseSQLiteTime(raw any) time.Time {
	switch value := raw.(type) {
	case time.Time:
		return value
	case string:
		for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02T15:04:05Z07:00"} {
			if parsed, err := time.Parse(layout, value); err == nil {
				return parsed
			}
		}
	case []byte:
		return parseSQLiteTime(string(value))
	}
	return time.Now()
}

func (s *Store) SumSupportSince(ctx context.Context, since time.Time) (float64, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT data FROM events WHERE created_at >= ?`, since.UTC().Format("2006-01-02 15:04:05"))
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var total float64
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return 0, err
		}
		var evt Event
		if err := json.Unmarshal([]byte(raw), &evt); err != nil {
			continue
		}
		switch evt.Type {
		case "superchat", "supersticker", "bits", "subscription", "gift":
			total += amountFloat(evt.Amount)
		}
	}
	return total, rows.Err()
}

func (s *Store) WidgetTime(ctx context.Context, key string) (time.Time, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM widget_state WHERE key = ?`, key).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	var value time.Time
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return time.Time{}, err
	}
	return value, nil
}

func (s *Store) SaveWidgetTime(ctx context.Context, key string, value time.Time) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT OR REPLACE INTO widget_state (key, value) VALUES (?, ?)`, key, string(raw))
	return err
}

func applyConfigValues(cfg *AppConfig, values map[string]string) {
	set := func(key string, dest any) {
		raw, ok := values[key]
		if !ok || raw == "" {
			return
		}
		_ = json.Unmarshal([]byte(raw), dest)
		if reflect.ValueOf(dest).Elem().Kind() == reflect.String && reflect.ValueOf(dest).Elem().String() == "" {
			reflect.ValueOf(dest).Elem().SetString(raw)
		}
	}
	set("twitchClientId", &cfg.TwitchClientID)
	set("twitchChannel", &cfg.TwitchChannel)
	set("youtubeChannelId", &cfg.YouTubeID)
	set("kickUsername", &cfg.KickUsername)
	set("tiktokUsername", &cfg.TikTokUsername)
	set("ttsEnabled", &cfg.TTSEnabled)
	set("ttsVoice", &cfg.TTSVoice)
	set("ttsMinBits", &cfg.TTSMinBits)
	set("ttsSubTier", &cfg.TTSSubTier)
	set("audioMode", &cfg.AudioMode)
	set("audioVolume", &cfg.AudioVol)
	set("ttsRewardEnabled", &cfg.TTSRewardEnabled)
	set("ttsRewardName", &cfg.TTSRewardName)
	set("ttsCmdEnabled", &cfg.TTSCmdEnabled)
	set("ttsCmdPrefix", &cfg.TTSCmdPrefix)
	set("ttsBlockedWords", &cfg.TTSBlockedWords)
	set("ttsCmdMod", &cfg.TTSCmdMod)
	set("ttsCmdSub", &cfg.TTSCmdSub)
	set("ttsCmdVip", &cfg.TTSCmdVip)
	set("ttsCmdHost", &cfg.TTSCmdHost)
	set("chatTheme", &cfg.ChatTheme)
	set("chatFontSize", &cfg.ChatFontSize)
	set("chatHideAfter", &cfg.ChatHideAfter)
	set("chatAnimations", &cfg.ChatAnimations)
	set("alertsConfig", &cfg.AlertsConfig)
	set("widgetsConfig", &cfg.WidgetsConfig)
}
