package tiktoklive

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"raikiri/internal/tiktoklive/internal/events"
	"raikiri/internal/tiktoklive/internal/gifts"
	pb "raikiri/internal/tiktoklive/internal/protocol"
)

func NormalizeUsername(value string) string {
	value = strings.TrimSpace(value)
	if parsed, err := url.Parse(value); err == nil && parsed.Host != "" {
		for _, part := range strings.Split(strings.Trim(parsed.Path, "/"), "/") {
			if strings.HasPrefix(part, "@") {
				return strings.TrimPrefix(part, "@")
			}
		}
	}
	return strings.TrimPrefix(value, "@")
}

func normalizeEvent(incoming events.Event, channel string, streaks *gifts.GiftStreakTracker, now func() time.Time) (Event, bool) {
	switch incoming.Type {
	case events.EventConnected:
		return Event{Kind: EventConnected, RoomID: incoming.RoomID}, true
	case events.EventReconnecting:
		return Event{Kind: EventReconnecting, RoomID: incoming.RoomID, Detail: fmt.Sprint(incoming.Data)}, true
	case events.EventDisconnected:
		return Event{Kind: EventDisconnected, RoomID: incoming.RoomID}, true
	case events.EventLiveEnded:
		return Event{Kind: EventLiveEnded, RoomID: incoming.RoomID}, true
	case events.EventChat:
		message, ok := incoming.Data.(*pb.WebcastChatMessage)
		if !ok || message == nil {
			return Event{}, false
		}
		user := normalizeUser(message.GetUser(), channel)
		at := eventTimestamp(message.GetCommon(), now)
		id := strconv.FormatInt(message.GetCommon().GetMsgId(), 10)
		if id == "0" {
			id = fmt.Sprintf("tiktok-%s-%d", user.ID, at.UnixNano())
		}
		return Event{Kind: EventChat, ID: id, At: at, User: user, Text: message.GetContent()}, true
	case events.EventGift:
		message, ok := incoming.Data.(*pb.WebcastGiftMessage)
		if !ok || message == nil {
			return Event{}, false
		}
		streak := streaks.Process(message)
		if !streak.IsFinal {
			return Event{}, false
		}
		count := int64(streak.TotalGiftCount)
		if count < 1 {
			count = 1
		}
		giftName := fmt.Sprintf("gift#%d", message.GetGiftId())
		if gift := message.GetGift(); gift != nil {
			if strings.TrimSpace(gift.GetName()) != "" {
				giftName = gift.GetName()
			} else if strings.TrimSpace(gift.GetDescribe()) != "" {
				giftName = gift.GetDescribe()
			}
		}
		return Event{
			Kind: EventGift, ID: strconv.FormatInt(message.GetCommon().GetMsgId(), 10),
			At: eventTimestamp(message.GetCommon(), now), User: normalizeUser(message.GetUser(), channel),
			GiftName: giftName, Count: count, Value: streak.TotalDiamondCount,
		}, true
	case events.EventFollow, events.EventShare:
		message, ok := incoming.Data.(*pb.WebcastSocialMessage)
		if !ok || message == nil {
			return Event{}, false
		}
		kind := EventFollow
		if incoming.Type == events.EventShare {
			kind = EventShare
		}
		return Event{Kind: kind, ID: strconv.FormatInt(message.GetCommon().GetMsgId(), 10), At: eventTimestamp(message.GetCommon(), now), User: normalizeUser(message.GetUser(), channel)}, true
	case events.EventLike:
		message, ok := incoming.Data.(*pb.WebcastLikeMessage)
		if !ok || message == nil {
			return Event{}, false
		}
		return Event{
			Kind: EventLike, ID: strconv.FormatInt(message.GetCommon().GetMsgId(), 10),
			At: eventTimestamp(message.GetCommon(), now), User: normalizeUser(message.GetUser(), channel),
			Count: int64(message.GetCount()), Total: int64(message.GetTotal()),
		}, true
	default:
		return Event{}, false
	}
}

func normalizeUser(user *pb.User, channel string) User {
	if user == nil {
		return User{Badges: []Badge{}}
	}
	username := firstNonEmpty(user.GetUniqueId(), user.GetDisplayId(), user.GetNickname(), user.GetIdStr())
	if username == "" && user.GetId() != 0 {
		username = strconv.FormatInt(user.GetId(), 10)
	}
	displayName := strings.TrimSpace(user.GetNickname())
	if displayName == "" {
		displayName = username
	}
	return User{
		ID: strconv.FormatInt(user.GetId(), 10), Username: username, DisplayName: displayName,
		Badges: normalizeBadges(user, channel),
	}
}

func normalizeBadges(user *pb.User, channel string) []Badge {
	seen := map[string]bool{}
	badges := make([]Badge, 0, 3)
	add := func(kind string) {
		if kind == "" || seen[kind] {
			return
		}
		seen[kind] = true
		badges = append(badges, Badge{Type: kind})
	}
	if strings.EqualFold(normalizeUserName(user), NormalizeUsername(channel)) {
		add("owner")
	}
	if user.GetIsSubscribe() {
		add("subscriber")
	}
	for _, badge := range user.GetBadgeList() {
		label := strings.ToLower(strings.Join([]string{badge.GetText().GetDefaultPattern(), badge.GetStr().GetStr()}, " "))
		switch {
		case strings.Contains(label, "moderator") || strings.Contains(label, "moderador"):
			add("moderator")
		case strings.Contains(label, "subscriber") || strings.Contains(label, "suscriptor"):
			add("subscriber")
		}
	}
	return badges
}

func normalizeUserName(user *pb.User) string {
	if user == nil {
		return ""
	}
	return firstNonEmpty(user.GetUniqueId(), user.GetDisplayId(), user.GetNickname(), user.GetIdStr())
}

func eventTimestamp(common *pb.Common, now func() time.Time) time.Time {
	if common == nil || common.GetCreateTime() <= 0 {
		return now()
	}
	value := common.GetCreateTime()
	if value >= 1_000_000_000_000 {
		return time.UnixMilli(value)
	}
	return time.Unix(value, 0)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
