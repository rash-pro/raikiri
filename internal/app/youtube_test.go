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
	              "timestampUsec": "1710000001000000"
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
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Author != "Viewer" || items[0].Text != "Hola :smile:" || len(items[0].Badges) != 1 {
		t.Fatalf("unexpected text message: %#v", items[0])
	}
	if items[1].SuperchatAmount != "$5.00" {
		t.Fatalf("unexpected superchat: %#v", items[1])
	}
}

func TestYouTubeLiveChatURLOnlyForVideoIDs(t *testing.T) {
	if got := youtubeLiveChatURL("jfKfPfyJRdk"); got != "https://www.youtube.com/live_chat?is_popout=1&v=jfKfPfyJRdk" {
		t.Fatalf("unexpected video fallback url: %q", got)
	}
	for _, id := range []string{"@lofigirl", "UCSJ4gkVC6NrvII8umztf0Ow", "https://www.youtube.com/watch?v=jfKfPfyJRdk"} {
		if got := youtubeLiveChatURL(id); got != "" {
			t.Fatalf("expected no fallback for %q, got %q", id, got)
		}
	}
}
