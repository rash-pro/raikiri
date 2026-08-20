package app

import (
	"context"
	"log/slog"
	"sync"

	"raikiri/internal/tiktoklive"
)

type tikTokLiveRunner interface {
	Run(context.Context, string, func(tiktoklive.Event)) error
}

type TikTokAdapter struct {
	username string
	logger   *slog.Logger
	chat     func(ChatMessage)
	event    func(Event)
	client   tikTokLiveRunner

	mu     sync.Mutex
	cancel context.CancelFunc
}

func NewTikTokAdapter(username string, logger *slog.Logger, chat func(ChatMessage), event func(Event)) *TikTokAdapter {
	if logger == nil {
		logger = slog.Default()
	}
	logger = logger.With("adapter", "tiktok_live")
	return &TikTokAdapter{
		username: username,
		logger:   logger,
		chat:     chat,
		event:    event,
		client:   tiktoklive.New(logger),
	}
}

func (a *TikTokAdapter) Start(ctx context.Context) {
	runCtx, cancel := context.WithCancel(ctx)
	a.mu.Lock()
	a.cancel = cancel
	a.mu.Unlock()
	defer func() {
		cancel()
		a.mu.Lock()
		a.cancel = nil
		a.mu.Unlock()
	}()

	if err := a.client.Run(runCtx, a.username, a.handle); err != nil && runCtx.Err() == nil {
		a.logger.Warn("tiktok live adapter stopped", "username", tiktoklive.NormalizeUsername(a.username), "error", err)
	}
}

func (a *TikTokAdapter) Stop() {
	a.mu.Lock()
	cancel := a.cancel
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (a *TikTokAdapter) handle(incoming tiktoklive.Event) {
	switch incoming.Kind {
	case tiktoklive.EventConnected:
		a.logger.Info("tiktok live connected", "username", tiktoklive.NormalizeUsername(a.username), "roomId", incoming.RoomID)
	case tiktoklive.EventReconnecting:
		a.logger.Warn("tiktok live reconnecting", "details", incoming.Detail)
	case tiktoklive.EventDisconnected:
		a.logger.Info("tiktok live disconnected", "username", tiktoklive.NormalizeUsername(a.username))
	case tiktoklive.EventLiveEnded:
		a.logger.Info("tiktok live ended", "username", tiktoklive.NormalizeUsername(a.username))
	case tiktoklive.EventChat:
		if a.chat != nil {
			a.chat(tikTokChatMessage(incoming))
		}
	case tiktoklive.EventGift:
		if a.event != nil {
			a.event(tikTokGiftEvent(incoming))
		}
	case tiktoklive.EventFollow, tiktoklive.EventLike, tiktoklive.EventShare:
		if a.event != nil {
			a.event(tikTokSocialEvent(incoming))
		}
	}
}

func tikTokChatMessage(incoming tiktoklive.Event) ChatMessage {
	displayName := incoming.User.DisplayName
	if displayName == "" {
		displayName = incoming.User.Username
	}
	content := sanitizeText(incoming.Text)
	badges := make([]Badge, 0, len(incoming.User.Badges))
	for _, badge := range incoming.User.Badges {
		badges = append(badges, Badge{Type: badge.Type})
	}
	return ChatMessage{
		ID: incoming.ID, Platform: PlatformTikTok, User: incoming.User.Username, DisplayName: displayName,
		Content: content, HTMLContent: content, Color: "#ff0050", Badges: badges, Timestamp: incoming.At,
	}
}

func tikTokGiftEvent(incoming tiktoklive.Event) Event {
	var amount any
	if incoming.Value > 0 {
		amount = incoming.Value
	}
	return Event{
		Type: "gift", Platform: PlatformTikTok, User: incoming.User.Username,
		Amount: amount, Count: incoming.Count, Currency: "diamonds", GiftName: incoming.GiftName,
	}
}

func tikTokSocialEvent(incoming tiktoklive.Event) Event {
	event := Event{Type: string(incoming.Kind), Platform: PlatformTikTok, User: incoming.User.Username}
	if incoming.Kind == tiktoklive.EventLike {
		event.Amount = incoming.Count
		event.Count = incoming.Count
	}
	return event
}
