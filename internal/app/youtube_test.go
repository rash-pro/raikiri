package app

import "testing"

func TestParseYouTubeChat(t *testing.T) {
	raw := []byte(`{
	  "continuationContents": {
	    "liveChatContinuation": {
	      "actions": [{
	        "addChatItemAction": {
	          "item": {
	            "liveChatTextMessageRenderer": {
	              "id": "msg-1",
	              "authorName": {"simpleText": "Viewer"},
	              "authorBadges": [{"liveChatAuthorBadgeRenderer": {"icon": {"iconType": "MODERATOR"}}}],
	              "message": {"runs": [{"text": "Hola "}, {"emoji": {"emojiId": ":smile:", "image": {"thumbnails": [{"url": "https://example.test/e.png"}]}}}]},
	              "timestampUsec": "1710000000000000"
	            }
	          }
	        }
	      }, {
	        "addChatItemAction": {
	          "item": {
	            "liveChatPaidMessageRenderer": {
	              "id": "sc-1",
	              "authorName": {"simpleText": "Donor"},
	              "message": {"runs": [{"text": "Gracias"}]},
	              "purchaseAmountText": {"simpleText": "$5.00"},
	              "headerBackgroundColor": 4278238420,
	              "bodyBackgroundColor": 4278233518,
	              "timestampUsec": "1710000001000000"
	            }
	          }
	        }
	      }, {
	        "addChatItemAction": {
	          "item": {
	            "liveChatPaidStickerRenderer": {
	              "id": "sticker-1",
	              "authorName": {"simpleText": "StickerFan"},
	              "purchaseAmountText": {"simpleText": "$2.00"},
	              "sticker": {"thumbnails": [{"url": "https://example.test/small.png"}, {"url": "https://example.test/big.png"}]},
	              "backgroundColor": 4294947584,
	              "timestampUsec": "1710000002000000"
	            }
	          }
	        }
	      }, {
	        "addChatItemAction": {
	          "item": {
	            "liveChatMembershipItemRenderer": {
	              "id": "member-1",
	              "authorName": {"simpleText": "MemberFan"},
	              "headerSubtext": {"runs": [{"text": "New member"}]},
	              "timestampUsec": "1710000003000000"
	            }
	          }
	        }
	      }],
	      "continuations": [{"timedContinuationData": {"continuation": "next-token", "timeoutMs": 1500}}]
	    }
	  }
	}`)

	items, next, timeout, err := parseYouTubeChat(raw)
	if err != nil {
		t.Fatal(err)
	}
	if next != "next-token" || timeout != 1500 {
		t.Fatalf("unexpected continuation %q timeout %d", next, timeout)
	}
	if len(items) != 4 {
		t.Fatalf("expected 4 items, got %d", len(items))
	}
	if items[0].Author != "Viewer" || items[0].Text != "Hola :smile:" || len(items[0].Badges) != 1 {
		t.Fatalf("unexpected text message: %#v", items[0])
	}
	if items[1].Kind != "superchat" || items[1].AmountText != "$5.00" || items[1].HeaderColor == "" || items[1].BodyColor == "" {
		t.Fatalf("unexpected superchat: %#v", items[1])
	}
	if items[2].Kind != "supersticker" || items[2].AmountText != "$2.00" || items[2].StickerURL != "https://example.test/big.png" {
		t.Fatalf("unexpected supersticker: %#v", items[2])
	}
	if items[3].Kind != "membership" || items[3].Text != "New member" {
		t.Fatalf("unexpected membership: %#v", items[3])
	}
}

func TestYouTubeLiveChatURLOnlyForVideoIDs(t *testing.T) {
	if got := youtubeLiveChatURL("jfKfPfyJRdk"); got != "https://www.youtube.com/live_chat?is_popout=1&v=jfKfPfyJRdk" {
		t.Fatalf("unexpected video fallback url: %q", got)
	}
	for input, want := range map[string]string{
		"https://www.youtube.com/watch?v=jfKfPfyJRdk":        "https://www.youtube.com/live_chat?is_popout=1&v=jfKfPfyJRdk",
		"https://youtu.be/jfKfPfyJRdk":                       "https://www.youtube.com/live_chat?is_popout=1&v=jfKfPfyJRdk",
		"https://www.youtube.com/live/jfKfPfyJRdk?feature=x": "https://www.youtube.com/live_chat?is_popout=1&v=jfKfPfyJRdk",
	} {
		if got := youtubeLiveChatURL(input); got != want {
			t.Fatalf("unexpected popout url for %q: %q", input, got)
		}
	}
	for _, id := range []string{"@lofigirl", "UCSJ4gkVC6NrvII8umztf0Ow"} {
		if got := youtubeLiveChatURL(id); got != "" {
			t.Fatalf("expected no fallback for %q, got %q", id, got)
		}
	}
}
