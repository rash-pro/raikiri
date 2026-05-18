package app

import "time"

type Platform string

const (
	PlatformTwitch  Platform = "twitch"
	PlatformYouTube Platform = "youtube"
)

type Badge struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

type ChatMessage struct {
	ID          string    `json:"id"`
	Platform    Platform  `json:"platform"`
	User        string    `json:"user"`
	DisplayName string    `json:"displayName"`
	Content     string    `json:"content"`
	HTMLContent string    `json:"htmlContent"`
	Color       string    `json:"color,omitempty"`
	Badges      []Badge   `json:"badges"`
	Timestamp   time.Time `json:"timestamp"`
	AnimationID string    `json:"animationId,omitempty"`
}

type Event struct {
	Type     string   `json:"type"`
	Platform Platform `json:"platform"`
	User     string   `json:"user,omitempty"`
	Amount   any      `json:"amount,omitempty"`
	Count    any      `json:"count,omitempty"`
	Tier     any      `json:"tier,omitempty"`
	Currency string   `json:"currency,omitempty"`
	Message  string   `json:"message,omitempty"`
	GiftName string   `json:"giftName,omitempty"`
	Viewers  any      `json:"viewers,omitempty"`
}

type AlertConfig struct {
	Enabled         bool   `json:"enabled"`
	Theme           string `json:"theme"`
	Voice           string `json:"voice"`
	GifURL          string `json:"gifUrl"`
	AudioURL        string `json:"audioUrl"`
	MessageTemplate string `json:"messageTemplate"`
}

type AppConfig struct {
	TwitchClientID string `json:"twitchClientId"`
	TwitchChannel  string `json:"twitchChannel"`
	YouTubeID      string `json:"youtubeChannelId"`
	KickUsername   string `json:"kickUsername"`
	TikTokUsername string `json:"tiktokUsername"`

	TTSEnabled bool   `json:"ttsEnabled"`
	TTSVoice   string `json:"ttsVoice"`
	TTSMinBits int    `json:"ttsMinBits"`
	TTSSubTier int    `json:"ttsSubTier"`
	AudioMode  string `json:"audioMode"`
	AudioVol   int    `json:"audioVolume"`

	TTSRewardEnabled bool   `json:"ttsRewardEnabled"`
	TTSRewardName    string `json:"ttsRewardName"`
	TTSCmdEnabled    bool   `json:"ttsCmdEnabled"`
	TTSCmdPrefix     string `json:"ttsCmdPrefix"`
	TTSCmdMod        bool   `json:"ttsCmdMod"`
	TTSCmdSub        bool   `json:"ttsCmdSub"`
	TTSCmdVip        bool   `json:"ttsCmdVip"`
	TTSCmdHost       bool   `json:"ttsCmdHost"`

	ChatTheme      string                 `json:"chatTheme"`
	ChatFontSize   int                    `json:"chatFontSize"`
	ChatHideAfter  int                    `json:"chatHideAfter"`
	ChatAnimations bool                   `json:"chatAnimations"`
	AlertsConfig   map[string]AlertConfig `json:"alertsConfig"`
}

func DefaultConfig() AppConfig {
	return AppConfig{
		TTSEnabled: true, TTSVoice: "es-MX-DaliaNeural", TTSMinBits: 100, TTSSubTier: 3,
		AudioMode: "websocket", AudioVol: 50,
		TTSCmdPrefix: "!voz", TTSCmdMod: true, TTSCmdHost: true,
		ChatTheme: "glassmorphism", ChatFontSize: 15, ChatHideAfter: 30, ChatAnimations: true,
		AlertsConfig: map[string]AlertConfig{
			"follow":         {Enabled: true, Theme: "cyberpurple", MessageTemplate: "¡{user} ha comenzado a seguirte!"},
			"subscription":   {Enabled: true, Theme: "cyberpurple", MessageTemplate: "¡{user} se ha suscrito (Tier {tier})! {message}"},
			"bits":           {Enabled: true, Theme: "cyberpurple", MessageTemplate: "{user} ha donado {amount} bits: {message}"},
			"raid":           {Enabled: true, Theme: "cyberpurple", MessageTemplate: "¡Alerta de Raid! {user} trae {amount} espectadores."},
			"superchat":      {Enabled: true, Theme: "cyberpurple", MessageTemplate: "¡{user} donó {amount} súper chat! {message}"},
			"gift":           {Enabled: true, Theme: "cyberpurple", MessageTemplate: "¡{user} ha regalado {amount} suscripciones!"},
			"channel_points": {Enabled: true, Theme: "cyberpurple", MessageTemplate: "{user} dice: {message}"},
		},
	}
}
