package telegram

import (
	"testing"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"
)

func TestHandleChatMemberUpdateLeaveWrongChat(t *testing.T) {
	bot := &EscarBot{
		Cache: NewCache(""), // in-memory mode
		Bot:   &tgbotapi.BotAPI{}, // Mock
	}

	userID := int64(123)
	chatID := int64(100)
	captchaMsgID := 456

	bot.Cache.SetCaptcha(userID, &PendingCaptcha{
		UserID:       userID,
		ChatID:       chatID,
		CaptchaMsgID: captchaMsgID,
	}, 0)

	// Leave from different chat
	update := &tgbotapi.ChatMemberUpdated{
		Chat: tgbotapi.Chat{ID: 200},
		OldChatMember: tgbotapi.ChatMember{Status: "member"},
		NewChatMember: tgbotapi.ChatMember{
			Status: "left",
			User:   &tgbotapi.User{ID: userID},
		},
	}

	handleChatMemberUpdate(bot, update)

	if !bot.Cache.HasCaptcha(userID) {
		t.Errorf("handleChatMemberUpdate() should NOT have deleted captcha for wrong chatID")
	}

	// Leave from correct chat
	update.Chat.ID = chatID
	handleChatMemberUpdate(bot, update)

	if bot.Cache.HasCaptcha(userID) {
		t.Errorf("handleChatMemberUpdate() SHOULD have deleted captcha for correct chatID")
	}
}

func TestProcessJoinIgnoresOtherChats(t *testing.T) {
	bot := &EscarBot{
		Cache:   NewCache(""), // in-memory mode
		GroupID: 100,
	}

	user := tgbotapi.User{ID: 123, FirstName: "TestUser"}

	// Test join in wrong chat
	processJoin(bot, 200, user, 1)

	// Since we can't easily check internal flow without mocking a lot,
	// let's check if a JoinEntry was created.
	// processJoin would create a JoinEntry early for the correct chat.
	if _, exists := bot.Cache.GetJoinEntry(user.ID); exists {
		t.Errorf("processJoin() should NOT have created join entry for wrong chatID")
	}

	// Now try correct chatID
	// It should proceed and eventually reach SetJoinEntry before possibly crashing/failing on Bot calls.
	// But let's just use the early return check.
}
