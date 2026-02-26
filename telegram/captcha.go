package telegram

import (
	"fmt"
	"html"
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
	UserFirstName   string
	ChatID          int64
	CorrectAnswer   string
	CaptchaMsgID    int
	JoinMsgID       int
	Attempts        int
	ExpirationTimer *time.Timer
}

func restrictUser(escarbot *EscarBot, chatID int64, userID int64) {
	permissions := tgbotapi.ChatPermissions{}
	restrictConfig := tgbotapi.RestrictChatMemberConfig{
		ChatMemberConfig: tgbotapi.ChatMemberConfig{
			ChatConfig: tgbotapi.ChatConfig{ChatID: chatID},
			UserID:     userID,
		},
		UntilDate:   0,
		Permissions: &permissions,
	}
	_, err := escarbot.Bot.Request(restrictConfig)
	if err != nil {
		if strings.Contains(err.Error(), "method is available only for supergroups") {
			log.Printf("Cannot restrict user %d: chat %d is not a supergroup (restriction skipped)", userID, chatID)
		} else {
			log.Printf("Error restricting user %d: %v", userID, err)
		}
	} else {
		log.Printf("User %d restricted in chat %d", userID, chatID)
	}
}

func unrestrictUser(escarbot *EscarBot, chatID int64, userID int64) {
	permissions := tgbotapi.ChatPermissions{
		CanSendMessages:       true,
		CanSendPolls:          true,
		CanSendOtherMessages:  true,
		CanAddWebPagePreviews: true,
		CanInviteUsers:        true,
	}
	permissions.SetCanSendMediaMessages(true)
	restrictConfig := tgbotapi.RestrictChatMemberConfig{
		ChatMemberConfig: tgbotapi.ChatMemberConfig{
			ChatConfig: tgbotapi.ChatConfig{ChatID: chatID},
			UserID:     userID,
		},
		UntilDate:   0,
		Permissions: &permissions,
	}
	_, err := escarbot.Bot.Request(restrictConfig)
	if err != nil {
		if strings.Contains(err.Error(), "method is available only for supergroups") {
			log.Printf("Cannot unrestrict user %d: chat %d is not a supergroup (unrestriction skipped)", userID, chatID)
		} else {
			log.Printf("Error unrestricting user %d: %v", userID, err)
		}
	} else {
		log.Printf("User %d unrestricted in chat %d", userID, chatID)
	}
}

func SendCaptcha(escarbot *EscarBot, chatID int64, user tgbotapi.User, joinMsgID int, attempts int) {
	answerInt := 1000 + rand.Intn(9000)
	answerStr := strconv.Itoa(answerInt)

	digits := make([]byte, len(answerStr))
	for i, char := range answerStr {
		digits[i] = byte(char - '0')
	}
	captchaImage := captcha.NewImage(strconv.Itoa(int(user.ID)), digits, 240, 80)

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
		photo.Caption = fmt.Sprintf("Welcome %s! Please solve the captcha within %d seconds to join the group.", html.EscapeString(user.FirstName), timeout)
	} else {
		photo.Caption = replacePlaceholders(escarbot, captchaText, user)
		photo.Caption = strings.ReplaceAll(photo.Caption, "{TIMEOUT}", strconv.Itoa(timeout))
	}
	photo.ParseMode = "HTML"

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

	// Stop existing timer if user rejoined quickly.
	if existing, ok := escarbot.Cache.GetCaptcha(user.ID); ok && existing.ExpirationTimer != nil {
		existing.ExpirationTimer.Stop()
	}

	timer := time.AfterFunc(time.Duration(timeout)*time.Second, func() {
		handleCaptchaTimeout(escarbot, user.ID)
	})

	pending := &PendingCaptcha{
		UserID:          user.ID,
		UserFirstName:   user.FirstName,
		ChatID:          chatID,
		CorrectAnswer:   answerStr,
		CaptchaMsgID:    msg.MessageID,
		JoinMsgID:       joinMsgID,
		Attempts:        attempts,
		ExpirationTimer: timer,
	}
	// Give the cache record a slightly longer TTL than the timer to avoid race conditions.
	escarbot.Cache.SetCaptcha(user.ID, pending, time.Duration(timeout+30)*time.Second)
	log.Printf("Captcha sent to user %d in chat %d, answer: %s", user.ID, chatID, answerStr)
}

func isUserPendingCaptcha(escarbot *EscarBot, userID int64) bool {
	return escarbot.Cache.HasCaptcha(userID)
}

func handleCaptchaTimeout(escarbot *EscarBot, userID int64) {
	pending, exists := escarbot.Cache.GetCaptcha(userID)
	if !exists {
		return
	}
	escarbot.Cache.DeleteCaptcha(userID)

	log.Printf("User %d timed out on captcha", userID)

	user := tgbotapi.User{ID: pending.UserID, FirstName: pending.UserFirstName}
	banAndCleanup(escarbot, pending.ChatID, user, pending.JoinMsgID, pending.CaptchaMsgID)
}

func HandleCaptchaCallback(escarbot *EscarBot, callback *tgbotapi.CallbackQuery) {
	if callback.Data == "" || !strings.HasPrefix(callback.Data, "captcha:") {
		return
	}

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

	pending, exists := escarbot.Cache.GetCaptcha(targetUserID)
	if !exists {
		callbackConfig := tgbotapi.NewCallback(callback.ID, "")
		escarbot.Bot.Request(callbackConfig)
		return
	}

	escarbot.StateMutex.RLock()
	maxRetries := escarbot.CaptchaMaxRetries
	escarbot.StateMutex.RUnlock()

	if givenAnswer == pending.CorrectAnswer {
		if pending.ExpirationTimer != nil {
			pending.ExpirationTimer.Stop()
		}
		escarbot.Cache.DeleteCaptcha(targetUserID)

		callbackConfig := tgbotapi.NewCallback(callback.ID, "Correct!")
		escarbot.Bot.Request(callbackConfig)

		unrestrictUser(escarbot, pending.ChatID, targetUserID)
		deleteMessages(escarbot, pending.ChatID, pending.CaptchaMsgID)

		escarbot.StateMutex.RLock()
		welcomeEnabled := escarbot.WelcomeMessage
		escarbot.StateMutex.RUnlock()

		if welcomeEnabled {
			sendWelcomeMessage(escarbot, pending.ChatID, *callback.From)
		}
	} else {
		if pending.ExpirationTimer != nil {
			pending.ExpirationTimer.Stop()
		}
		escarbot.Cache.DeleteCaptcha(targetUserID)

		log.Printf("User %d gave wrong captcha answer: %s (expected %s). Attempt: %d/%d",
			targetUserID, givenAnswer, pending.CorrectAnswer, pending.Attempts+1, maxRetries+1)

		callbackConfig := tgbotapi.NewCallback(callback.ID, "")
		escarbot.Bot.Request(callbackConfig)

		if pending.Attempts < maxRetries {
			deleteMessages(escarbot, pending.ChatID, pending.CaptchaMsgID)
			SendCaptcha(escarbot, pending.ChatID, *callback.From, pending.JoinMsgID, pending.Attempts+1)
		} else {
			banAndCleanup(escarbot, pending.ChatID, *callback.From, pending.JoinMsgID, pending.CaptchaMsgID)
		}
	}
}
