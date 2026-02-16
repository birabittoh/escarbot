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
	ExpirationTimer *time.Timer
}

func SendCaptcha(escarbot *EscarBot, chatID int64, user tgbotapi.User, joinMsgID int) {
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

	photo := tgbotapi.NewPhoto(chatID, file)
	photo.Caption = fmt.Sprintf("Welcome %s! Please solve the captcha within 120 seconds to join the group.", user.FirstName)

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
	defer escarbot.CaptchaMutex.Unlock()

	timer := time.AfterFunc(120*time.Second, func() {
		handleCaptchaTimeout(escarbot, user.ID)
	})

	escarbot.PendingCaptchas[user.ID] = &PendingCaptcha{
		UserID:          user.ID,
		ChatID:          chatID,
		CorrectAnswer:   answerStr,
		CaptchaMsgID:    msg.MessageID,
		JoinMsgID:       joinMsgID,
		ExpirationTimer: timer,
	}
	log.Printf("Captcha sent to user %d in chat %d, answer: %s", user.ID, chatID, answerStr)
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
	chat := tgbotapi.Chat{ID: pending.ChatID}
	user := tgbotapi.User{ID: pending.UserID, FirstName: "User"} // FirstName is used in logs

	banUser(escarbot, chat, user)

	// Delete join message
	deleteJoin := tgbotapi.NewDeleteMessage(pending.ChatID, pending.JoinMsgID)
	escarbot.Bot.Request(deleteJoin)

	// Delete captcha message
	deleteCaptcha := tgbotapi.NewDeleteMessage(pending.ChatID, pending.CaptchaMsgID)
	escarbot.Bot.Request(deleteCaptcha)
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
		callbackConfig := tgbotapi.NewCallbackWithAlert(callback.ID, "This captcha is not for you!")
		escarbot.Bot.Request(callbackConfig)
		return
	}

	escarbot.CaptchaMutex.Lock()
	pending, exists := escarbot.PendingCaptchas[targetUserID]
	if !exists {
		escarbot.CaptchaMutex.Unlock()
		callbackConfig := tgbotapi.NewCallbackWithAlert(callback.ID, "Captcha expired or not found.")
		escarbot.Bot.Request(callbackConfig)
		return
	}

	if givenAnswer == pending.CorrectAnswer {
		pending.ExpirationTimer.Stop()
		delete(escarbot.PendingCaptchas, targetUserID)
		escarbot.CaptchaMutex.Unlock()

		// Success!
		callbackConfig := tgbotapi.NewCallback(callback.ID, "Correct!")
		escarbot.Bot.Request(callbackConfig)

		// Delete captcha message
		deleteCaptcha := tgbotapi.NewDeleteMessage(pending.ChatID, pending.CaptchaMsgID)
		escarbot.Bot.Request(deleteCaptcha)

		// Send welcome message
		sendWelcomeMessage(escarbot, pending.ChatID, *callback.From)
	} else {
		// Wrong answer - act like timeout (ban)
		pending.ExpirationTimer.Stop()
		delete(escarbot.PendingCaptchas, targetUserID)
		escarbot.CaptchaMutex.Unlock()

		callbackConfig := tgbotapi.NewCallbackWithAlert(callback.ID, "Wrong answer! You are banned.")
		escarbot.Bot.Request(callbackConfig)

		log.Printf("User %d gave wrong captcha answer: %s (expected %s)", targetUserID, givenAnswer, pending.CorrectAnswer)

		chat := tgbotapi.Chat{ID: pending.ChatID}
		banUser(escarbot, chat, *callback.From)

		// Delete join message
		deleteJoin := tgbotapi.NewDeleteMessage(pending.ChatID, pending.JoinMsgID)
		escarbot.Bot.Request(deleteJoin)

		// Delete captcha message
		deleteCaptcha := tgbotapi.NewDeleteMessage(pending.ChatID, pending.CaptchaMsgID)
		escarbot.Bot.Request(deleteCaptcha)
	}
}
