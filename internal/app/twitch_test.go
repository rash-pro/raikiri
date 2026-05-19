package app

import (
	"testing"

	twitch "github.com/gempir/go-twitch-irc/v4"
)

func TestTwitchMessageBadgesDetectsModeratorFromTags(t *testing.T) {
	badges := twitchMessageBadges(twitch.PrivateMessage{
		User: twitch.User{
			Name:   "moduser",
			Badges: map[string]int{},
		},
		Tags: map[string]string{"mod": "1", "user-type": "mod"},
	})

	if !hasBadgeType(badges, "moderator") {
		t.Fatalf("expected moderator badge from Twitch mod tags, got %#v", badges)
	}
}

func TestTwitchMessageBadgesDetectsSubscriberFromTags(t *testing.T) {
	badges := twitchMessageBadges(twitch.PrivateMessage{
		User: twitch.User{
			Name:   "subuser",
			Badges: map[string]int{},
		},
		Tags: map[string]string{"subscriber": "1"},
	})

	if !hasBadgeType(badges, "subscriber") {
		t.Fatalf("expected subscriber badge from Twitch subscriber tag, got %#v", badges)
	}
}

func TestTwitchMessageBadgesDetectsVIPFromRawBadgesTag(t *testing.T) {
	badges := twitchMessageBadges(twitch.PrivateMessage{
		User: twitch.User{
			Name:   "vipuser",
			Badges: map[string]int{},
		},
		Tags: map[string]string{"badges": "vip/1"},
	})

	if !hasBadgeType(badges, "vip") {
		t.Fatalf("expected vip badge from Twitch raw badges tag, got %#v", badges)
	}
}

func TestTwitchMessageBadgesDetectsFounderAsSubscriber(t *testing.T) {
	badges := twitchMessageBadges(twitch.PrivateMessage{
		User: twitch.User{
			Name: "founderuser",
			Badges: map[string]int{
				"founder": 0,
			},
		},
		Tags: map[string]string{},
	})

	if !hasBadgeType(badges, "subscriber") {
		t.Fatalf("expected founder badge to count as subscriber, got %#v", badges)
	}
}

func TestTwitchMessageBadgesDeduplicatesRoles(t *testing.T) {
	badges := twitchMessageBadges(twitch.PrivateMessage{
		User: twitch.User{
			Name: "moduser",
			Badges: map[string]int{
				"moderator": 1,
			},
		},
		Tags: map[string]string{"mod": "1"},
	})

	count := 0
	for _, badge := range badges {
		if badge.Type == "moderator" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected one moderator badge, got %d in %#v", count, badges)
	}
}

func hasBadgeType(badges []Badge, badgeType string) bool {
	for _, badge := range badges {
		if badge.Type == badgeType {
			return true
		}
	}
	return false
}
