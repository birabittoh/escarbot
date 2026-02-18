package telegram

import (
	"testing"
	tgbotapi "github.com/OvyFlash/telegram-bot-api"
)

func TestAddMessageToCache(t *testing.T) {
	bot := &EscarBot{
		MessageCache:       make(map[int64][]CachedMessage),
		ChatCache:          make(map[int64]ChatInfo),
		AvailableReactions: make(map[int64][]string),
		MaxCacheSize:       10,
		Bot:                &tgbotapi.BotAPI{}, // Mock
	}

	msg := &tgbotapi.Message{
		MessageID: 1,
		Chat: tgbotapi.Chat{
			ID:    123,
			Title: "Test Chat",
		},
		From: &tgbotapi.User{
			FirstName: "Alice",
		},
		Text: "Hello",
	}

	// We need to avoid the GetChat call or mock it.
	// Since GetChat is called if chat doesn't exist in ChatCache.
	// Let's pre-populate ChatCache.
	bot.ChatCache[123] = ChatInfo{ID: 123, Title: "Test Chat"}

	AddMessageToCache(bot, msg)

	if len(bot.MessageCache) != 1 {
		t.Errorf("Expected 1 chat in cache, got %d", len(bot.MessageCache))
	}

	msgs, ok := bot.MessageCache[123]
	if !ok {
		t.Fatal("Chat 123 not found in cache")
	}

	if len(msgs) != 1 {
		t.Errorf("Expected 1 message in chat 123, got %d", len(msgs))
	}

	if msgs[0].Text != "Hello" {
		t.Errorf("Expected message text 'Hello', got '%s'", msgs[0].Text)
	}
}
