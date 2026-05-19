package app

import (
	"strings"
	"time"
)

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
	ID             string    `json:"id"`
	Platform       Platform  `json:"platform"`
	User           string    `json:"user"`
	DisplayName    string    `json:"displayName"`
	Content        string    `json:"content"`
	HTMLContent    string    `json:"htmlContent"`
	Kind           string    `json:"kind,omitempty"`
	AmountText     string    `json:"amountText,omitempty"`
	StickerURL     string    `json:"stickerUrl,omitempty"`
	HeaderColor    string    `json:"headerColor,omitempty"`
	BodyColor      string    `json:"bodyColor,omitempty"`
	Color          string    `json:"color,omitempty"`
	Badges         []Badge   `json:"badges"`
	Timestamp      time.Time `json:"timestamp"`
	AnimationID    string    `json:"animationId,omitempty"`
	CustomRewardID string    `json:"customRewardId,omitempty"`
}

type Event struct {
	Type       string   `json:"type"`
	Platform   Platform `json:"platform"`
	User       string   `json:"user,omitempty"`
	Amount     any      `json:"amount,omitempty"`
	Count      any      `json:"count,omitempty"`
	Tier       any      `json:"tier,omitempty"`
	Currency   string   `json:"currency,omitempty"`
	Message    string   `json:"message,omitempty"`
	GiftName   string   `json:"giftName,omitempty"`
	RewardName string   `json:"rewardName,omitempty"`
	Viewers    any      `json:"viewers,omitempty"`
}

type AlertConfig struct {
	Enabled         bool   `json:"enabled"`
	Theme           string `json:"theme"`
	Voice           string `json:"voice"`
	GifURL          string `json:"gifUrl"`
	AudioURL        string `json:"audioUrl"`
	MessageTemplate string `json:"messageTemplate"`
}

type WidgetsConfig struct {
	SupportGoal  SupportGoalConfig    `json:"supportGoal"`
	RecentEvents RecentEventsConfig   `json:"recentEvents"`
	Custom       []CustomWidgetConfig `json:"custom"`
}

type WidgetAppearance struct {
	Theme             string `json:"theme"`
	AccentColor       string `json:"accentColor"`
	FontFamily        string `json:"fontFamily"`
	BackgroundOpacity int    `json:"backgroundOpacity"`
	BorderRadius      int    `json:"borderRadius"`
	Width             int    `json:"width"`
	ShowIcons         bool   `json:"showIcons"`
}

type SupportGoalConfig struct {
	Enabled      bool             `json:"enabled"`
	Title        string           `json:"title"`
	TargetAmount float64          `json:"targetAmount"`
	Currency     string           `json:"currency"`
	Appearance   WidgetAppearance `json:"appearance"`
}

type RecentEventsConfig struct {
	Enabled    bool             `json:"enabled"`
	Limit      int              `json:"limit"`
	Types      []string         `json:"types"`
	Appearance WidgetAppearance `json:"appearance"`
}

type CustomWidgetConfig struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Enabled    bool                   `json:"enabled"`
	Activation CustomWidgetActivation `json:"activation"`
	HTML       string                 `json:"html"`
	CSS        string                 `json:"css"`
	JS         string                 `json:"js"`
	Appearance WidgetAppearance       `json:"appearance"`
}

type CustomWidgetActivation struct {
	EventType  string  `json:"eventType"`
	MinAmount  float64 `json:"minAmount"`
	RewardName string  `json:"rewardName"`
}

type WidgetState struct {
	Config       WidgetsConfig    `json:"config"`
	SupportGoal  SupportGoalState `json:"supportGoal"`
	RecentEvents []RecentEvent    `json:"recentEvents"`
}

type SupportGoalState struct {
	SupportGoalConfig
	CurrentAmount float64   `json:"currentAmount"`
	ResetAt       time.Time `json:"resetAt"`
}

type RecentEvent struct {
	Type      string    `json:"type"`
	Platform  Platform  `json:"platform"`
	User      string    `json:"user,omitempty"`
	Amount    any       `json:"amount,omitempty"`
	Message   string    `json:"message,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
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
	TTSBlockedWords  string `json:"ttsBlockedWords"`
	TTSCmdMod        bool   `json:"ttsCmdMod"`
	TTSCmdSub        bool   `json:"ttsCmdSub"`
	TTSCmdVip        bool   `json:"ttsCmdVip"`
	TTSCmdHost       bool   `json:"ttsCmdHost"`

	ChatTheme      string                 `json:"chatTheme"`
	ChatFontSize   int                    `json:"chatFontSize"`
	ChatHideAfter  int                    `json:"chatHideAfter"`
	ChatAnimations bool                   `json:"chatAnimations"`
	AlertsConfig   map[string]AlertConfig `json:"alertsConfig"`
	WidgetsConfig  WidgetsConfig          `json:"widgetsConfig"`
}

func DefaultConfig() AppConfig {
	return AppConfig{
		TTSEnabled: true, TTSVoice: "es-MX-DaliaNeural", TTSMinBits: 100, TTSSubTier: 3,
		AudioMode: "websocket", AudioVol: 50,
		TTSCmdPrefix: "!voz", TTSBlockedWords: defaultTTSBlockedWords(), TTSCmdMod: true, TTSCmdHost: true,
		ChatTheme: "glassmorphism", ChatFontSize: 15, ChatHideAfter: 30, ChatAnimations: true,
		WidgetsConfig: WidgetsConfig{
			SupportGoal: SupportGoalConfig{
				Enabled: true, Title: "Support Goal", TargetAmount: 100, Currency: "USD",
				Appearance: DefaultWidgetAppearance(720, true),
			},
			RecentEvents: RecentEventsConfig{
				Enabled: true, Limit: 8,
				Types:      []string{"superchat", "supersticker", "membership", "subscription", "bits", "raid", "gift", "follow"},
				Appearance: DefaultWidgetAppearance(520, true),
			},
		},
		AlertsConfig: map[string]AlertConfig{
			"follow":         {Enabled: true, Theme: "cyberpurple", MessageTemplate: "¡{user} ha comenzado a seguirte!"},
			"subscription":   {Enabled: true, Theme: "cyberpurple", MessageTemplate: "¡{user} se ha suscrito (Tier {tier})! {message}"},
			"bits":           {Enabled: true, Theme: "cyberpurple", MessageTemplate: "{user} ha donado {amount} bits: {message}"},
			"raid":           {Enabled: true, Theme: "cyberpurple", MessageTemplate: "¡Alerta de Raid! {user} trae {amount} espectadores."},
			"superchat":      {Enabled: true, Theme: "cyberpurple", MessageTemplate: "¡{user} donó {amount} súper chat! {message}"},
			"supersticker":   {Enabled: true, Theme: "cyberpurple", MessageTemplate: "¡{user} envió un súper sticker de {amount}!"},
			"membership":     {Enabled: true, Theme: "cyberpurple", MessageTemplate: "¡{user} se hizo miembro! {message}"},
			"gift":           {Enabled: true, Theme: "cyberpurple", MessageTemplate: "¡{user} ha regalado {amount} suscripciones!"},
			"channel_points": {Enabled: true, Theme: "cyberpurple", MessageTemplate: "{user} dice: {message}"},
		},
	}
}

func defaultTTSBlockedWords() string {
	return strings.Join([]string{
		"puta",
		"puto",
		"putas",
		"putos",
		"putazo",
		"putiza",
		"pendejo",
		"pendeja",
		"pendejos",
		"pendejas",
		"pendejada",
		"pendejadas",
		"cabron",
		"cabrón",
		"cabrona",
		"cabrones",
		"cabronazo",
		"chingar",
		"chingada",
		"chingado",
		"chingadera",
		"chingaderas",
		"chingas",
		"chinga",
		"chingon",
		"chingón",
		"chingona",
		"chingue",
		"verga",
		"vergazo",
		"verguiza",
		"verguero",
		"mierda",
		"mierdas",
		"culero",
		"culera",
		"culeros",
		"culeras",
		"culito",
		"culo",
		"coño",
		"cono",
		"joder",
		"jodido",
		"jodida",
		"jodidos",
		"jodidas",
		"jodiendo",
		"idiota",
		"idiotas",
		"imbecil",
		"imbécil",
		"imbeciles",
		"imbéciles",
		"estupido",
		"estúpido",
		"estupida",
		"estúpida",
		"estupidos",
		"estúpidos",
		"estupidas",
		"estúpidas",
		"mamon",
		"mamón",
		"mamona",
		"mamones",
		"mamadas",
		"mamado",
		"gilipollas",
		"carajo",
		"carajazo",
		"pinche",
		"pinches",
		"maldito",
		"maldita",
		"malditos",
		"malditas",
		"zorra",
		"zorras",
		"perra",
		"perras",
		"cojones",
		"cojer",
		"coger",
		"cogiendo",
		"cojiendo",
		"cojido",
		"cogido",
		"pito",
		"pitos",
		"pija",
		"pijas",
		"vagina",
		"vaginas",
		"pene",
		"penes",
		"ano",
		"anos",
		"nalgas",
		"tetona",
		"tetonas",
		"tetas",
		"chichi",
		"chichis",
		"chaqueta",
		"puñeta",
		"puneta",
		"puñetas",
		"punetas",
		"boludo",
		"boluda",
		"pelotudo",
		"pelotuda",
		"weon",
		"weón",
		"huevon",
		"huevón",
		"huevona",
		"guey",
		"güey",
		"wey",
	}, "\n")
}

func DefaultWidgetAppearance(width int, showIcons bool) WidgetAppearance {
	return WidgetAppearance{
		Theme: "glass", AccentColor: "#10b981", FontFamily: "Inter, system-ui, sans-serif",
		BackgroundOpacity: 78, BorderRadius: 8, Width: width, ShowIcons: showIcons,
	}
}
