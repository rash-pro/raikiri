package app

import "testing"

func TestCustomWidgetChatCommandMatchesPlatform(t *testing.T) {
	widget := CustomWidgetConfig{
		ID: "quack", Enabled: true,
		Activation: CustomWidgetActivation{EventType: PlatformEventChannelPoints, RewardName: "Quack", ChatCommand: "!quack", ChatPlatform: PlatformYouTube},
	}

	evt, ok := customWidgetChatCommandEvent(widget, ChatMessage{Platform: PlatformYouTube, DisplayName: "Viewer", Content: "!QUACK fuerte"})
	if !ok {
		t.Fatal("expected YouTube !quack to activate the widget")
	}
	if evt.Type != "chat_command" || evt.User != "Viewer" || evt.Message != "fuerte" || evt.Platform != PlatformYouTube {
		t.Fatalf("unexpected event %+v", evt)
	}

	for _, msg := range []ChatMessage{
		{Platform: PlatformTwitch, Content: "!quack"},
		{Platform: PlatformYouTube, Content: "!quackquack"},
		{Platform: PlatformYouTube, Content: "hola !quack"},
	} {
		if _, ok := customWidgetChatCommandEvent(widget, msg); ok {
			t.Fatalf("unexpected activation for %+v", msg)
		}
	}

	widget.Enabled = false
	if _, ok := customWidgetChatCommandEvent(widget, ChatMessage{Platform: PlatformYouTube, Content: "!quack"}); ok {
		t.Fatal("disabled widget must not activate")
	}
}
