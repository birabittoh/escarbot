package telegram

import (
	"testing"
)

func TestIsUserPendingCaptcha(t *testing.T) {
	bot := &EscarBot{
		Cache: NewCache(""), // in-memory mode
	}

	userID := int64(12345)

	if isUserPendingCaptcha(bot, userID) {
		t.Errorf("isUserPendingCaptcha() = true, want false for empty cache")
	}

	bot.Cache.SetCaptcha(userID, &PendingCaptcha{UserID: userID}, 0)

	if !isUserPendingCaptcha(bot, userID) {
		t.Errorf("isUserPendingCaptcha() = false, want true for user in cache")
	}

	bot.Cache.DeleteCaptcha(userID)

	if isUserPendingCaptcha(bot, userID) {
		t.Errorf("isUserPendingCaptcha() = true, want false after deletion")
	}
}
