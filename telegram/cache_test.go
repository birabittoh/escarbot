package telegram

import (
	"testing"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"
)

func TestAddMessageToCache(t *testing.T) {
	bot := &EscarBot{
		Cache:        NewCache(""), // in-memory mode
		MaxCacheSize: 10,
		Bot:          &tgbotapi.BotAPI{}, // Mock
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

	// Pre-populate chat cache so AddMessageToCache skips the GetChat API call.
	bot.Cache.SetChatInfo(123, ChatInfo{ID: 123, Title: "Test Chat"})

	AddMessageToCache(bot, msg)

	allMsgs := bot.Cache.GetAllMessages()
	if len(allMsgs) != 1 {
		t.Errorf("Expected 1 chat in cache, got %d", len(allMsgs))
	}

	msgs := bot.Cache.GetMessages(123)
	if len(msgs) != 1 {
		t.Errorf("Expected 1 message in chat 123, got %d", len(msgs))
	}

	if msgs[0].Text != "Hello" {
		t.Errorf("Expected message text 'Hello', got '%s'", msgs[0].Text)
	}
}
