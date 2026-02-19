package telegram

import (
	"fmt"
	"log"
	"math/rand"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/birabittoh/captcha"
)

type PendingCaptcha struct {
	UserID          int64
	ChatID          int64
	CorrectAnswer   string
	CaptchaMsgID    int
	JoinMsgID       int
	Attempts        int
	ExpirationTimer *time.Timer
}

func SendCaptcha(escarbot *EscarBot, chatID int64, user tgbotapi.User, joinMsgID int, attempts int) {
	// 1. Generate random 4-digit answer
	answerInt := 1000 + rand.Intn(9000)
	answerStr := strconv.Itoa(answerInt)

	// 2. Generate image
	digits := make([]byte, len(answerStr))
	for i, char := range answerStr {
		digits[i] = byte(char - '0')
	}
	captchaImage := captcha.NewImage(strconv.Itoa(int(user.ID)), digits, 240, 80)

	// 3. Prepare message
	file := tgbotapi.FileBytes{
		Name:  "captcha.png",
		Bytes: captchaImage.EncodedPNG(),
	}

	escarbot.StateMutex.RLock()
	timeout := escarbot.CaptchaTimeout
	captchaText := escarbot.CaptchaText
	escarbot.StateMutex.RUnlock()

	photo := tgbotapi.NewPhoto(chatID, file)
	if captchaText == "" {
		photo.Caption = fmt.Sprintf("Welcome %s! Please solve the captcha within %d seconds to join the group.", user.FirstName, timeout)
	} else {
		photo.Caption = replacePlaceholders(escarbot, captchaText, user)
		photo.Caption = strings.ReplaceAll(photo.Caption, "{TIMEOUT}", strconv.Itoa(timeout))
	}
	photo.ParseMode = "Markdown"

	// Add buttons
	var buttons []tgbotapi.InlineKeyboardButton
	correctIdx := rand.Intn(4)
	usedAnswers := make(map[int]bool)
	usedAnswers[answerInt] = true

	for i := 0; i < 4; i++ {
		var btnAnswer int
		if i == correctIdx {
			btnAnswer = answerInt
		} else {
			for {
				btnAnswer = 1000 + rand.Intn(9000)
				if !usedAnswers[btnAnswer] {
					break
				}
			}
			usedAnswers[btnAnswer] = true
		}
		buttons = append(buttons, tgbotapi.NewInlineKeyboardButtonData(strconv.Itoa(btnAnswer), fmt.Sprintf("captcha:%d:%d", user.ID, btnAnswer)))
	}

	photo.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(tgbotapi.NewInlineKeyboardRow(buttons...))

	msg, err := escarbot.Bot.Send(photo)
	if err != nil {
		log.Printf("Error sending captcha: %v", err)
		return
	}

	// 4. Store state
	escarbot.CaptchaMutex.Lock()

	// Stop existing timer if any (e.g. user rejoined quickly)
	if existing, ok := escarbot.PendingCaptchas[user.ID]; ok && existing.ExpirationTimer != nil {
		existing.ExpirationTimer.Stop()
	}

	timer := time.AfterFunc(time.Duration(timeout)*time.Second, func() {
		handleCaptchaTimeout(escarbot, user.ID)
	})

	escarbot.PendingCaptchas[user.ID] = &PendingCaptcha{
		UserID:          user.ID,
		ChatID:          chatID,
		CorrectAnswer:   answerStr,
		CaptchaMsgID:    msg.MessageID,
		JoinMsgID:       joinMsgID,
		Attempts:        attempts,
		ExpirationTimer: timer,
	}
	escarbot.CaptchaMutex.Unlock()

	log.Printf("Captcha sent to user %d in chat %d, answer: %s", user.ID, chatID, answerStr)

	// 5. Restrict user permissions while they solve the captcha
	restrictUser(escarbot, chatID, user.ID)
}

func isUserPendingCaptcha(escarbot *EscarBot, userID int64) bool {
	escarbot.CaptchaMutex.RLock()
	defer escarbot.CaptchaMutex.RUnlock()
	_, exists := escarbot.PendingCaptchas[userID]
	return exists
}

func handleCaptchaTimeout(escarbot *EscarBot, userID int64) {
	escarbot.CaptchaMutex.Lock()
	pending, exists := escarbot.PendingCaptchas[userID]
	if !exists {
		escarbot.CaptchaMutex.Unlock()
		return
	}
	delete(escarbot.PendingCaptchas, userID)
	escarbot.CaptchaMutex.Unlock()

	log.Printf("User %d timed out on captcha", userID)

	// Act like auto-ban
	user := tgbotapi.User{ID: pending.UserID, FirstName: "User"} // FirstName is used in logs

	banAndCleanup(escarbot, pending.ChatID, user, pending.JoinMsgID, pending.CaptchaMsgID)
}

func HandleCaptchaCallback(escarbot *EscarBot, callback *tgbotapi.CallbackQuery) {
	if callback.Data == "" || !strings.HasPrefix(callback.Data, "captcha:") {
		return
	}

	// data format: captcha:userID:answer
	parts := strings.Split(callback.Data, ":")
	if len(parts) != 3 {
		return
	}

	targetUserID, _ := strconv.ParseInt(parts[1], 10, 64)
	givenAnswer := parts[2]

	if callback.From.ID != targetUserID {
		callbackConfig := tgbotapi.NewCallback(callback.ID, "")
		escarbot.Bot.Request(callbackConfig)
		return
	}

	escarbot.CaptchaMutex.Lock()
	pending, exists := escarbot.PendingCaptchas[targetUserID]
	if !exists {
		escarbot.CaptchaMutex.Unlock()
		callbackConfig := tgbotapi.NewCallback(callback.ID, "")
		escarbot.Bot.Request(callbackConfig)
		return
	}

	escarbot.StateMutex.RLock()
	maxRetries := escarbot.CaptchaMaxRetries
	escarbot.StateMutex.RUnlock()

	if givenAnswer == pending.CorrectAnswer {
		pending.ExpirationTimer.Stop()
		delete(escarbot.PendingCaptchas, targetUserID)
		escarbot.CaptchaMutex.Unlock()

		// Success!
		callbackConfig := tgbotapi.NewCallback(callback.ID, "Correct!")
		escarbot.Bot.Request(callbackConfig)

		// Unrestrict user and mark as verified
		unrestrictUser(escarbot, pending.ChatID, targetUserID)
		addVerifiedUser(escarbot, targetUserID)

		// Delete captcha message
		deleteMessages(escarbot, pending.ChatID, pending.CaptchaMsgID)

		// Send welcome message if enabled
		escarbot.StateMutex.RLock()
		welcomeEnabled := escarbot.WelcomeMessage
		escarbot.StateMutex.RUnlock()

		if welcomeEnabled {
			sendWelcomeMessage(escarbot, pending.ChatID, *callback.From)
		}
	} else {
		// Wrong answer
		pending.ExpirationTimer.Stop()
		delete(escarbot.PendingCaptchas, targetUserID)
		escarbot.CaptchaMutex.Unlock()

		log.Printf("User %d gave wrong captcha answer: %s (expected %s). Attempt: %d/%d",
			targetUserID, givenAnswer, pending.CorrectAnswer, pending.Attempts+1, maxRetries+1)

		if pending.Attempts < maxRetries {
			callbackConfig := tgbotapi.NewCallback(callback.ID, "")
			escarbot.Bot.Request(callbackConfig)

			// Delete old captcha message
			deleteMessages(escarbot, pending.ChatID, pending.CaptchaMsgID)

			// Send new captcha
			SendCaptcha(escarbot, pending.ChatID, *callback.From, pending.JoinMsgID, pending.Attempts+1)
		} else {
			callbackConfig := tgbotapi.NewCallback(callback.ID, "")
			escarbot.Bot.Request(callbackConfig)

			banAndCleanup(escarbot, pending.ChatID, *callback.From, pending.JoinMsgID, pending.CaptchaMsgID)
		}
	}
}
