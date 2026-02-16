package telegram

import (
	"testing"
)

func TestIsUserPendingCaptcha(t *testing.T) {
	bot := &EscarBot{
		PendingCaptchas: make(map[int64]*PendingCaptcha),
	}

	userID := int64(12345)

	if isUserPendingCaptcha(bot, userID) {
		t.Errorf("isUserPendingCaptcha() = true, want false for empty map")
	}

	bot.PendingCaptchas[userID] = &PendingCaptcha{UserID: userID}

	if !isUserPendingCaptcha(bot, userID) {
		t.Errorf("isUserPendingCaptcha() = false, want true for user in map")
	}

	delete(bot.PendingCaptchas, userID)

	if isUserPendingCaptcha(bot, userID) {
		t.Errorf("isUserPendingCaptcha() = true, want false after deletion")
	}
}
