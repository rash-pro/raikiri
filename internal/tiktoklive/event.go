// Package tiktoklive owns Raikiri's TikTok LIVE protocol implementation.
// Callers only see normalized events; Webcast transport and protobuf details
// remain private to this module.
package tiktoklive

import "time"

type EventKind string

const (
	EventConnected    EventKind = "connected"
	EventReconnecting EventKind = "reconnecting"
	EventDisconnected EventKind = "disconnected"
	EventLiveEnded    EventKind = "live_ended"
	EventChat         EventKind = "chat"
	EventGift         EventKind = "gift"
	EventFollow       EventKind = "follow"
	EventLike         EventKind = "like"
	EventShare        EventKind = "share"
)

type Badge struct {
	Type string
}

type User struct {
	ID          string
	Username    string
	DisplayName string
	Badges      []Badge
}

type Event struct {
	Kind EventKind
	ID   string
	At   time.Time

	RoomID string
	Detail string
	User   User
	Text   string

	GiftName string
	Count    int64
	Value    int64
	Total    int64
}
