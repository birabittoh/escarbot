package telegram

import (
	"log"
	"strconv"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var emptyKeyboard tgbotapi.InlineKeyboardMarkup
var inlineKeyboardFeedback = tgbotapi.NewInlineKeyboardMarkup(
	tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("✅", "1"),
		tgbotapi.NewInlineKeyboardButtonData("❌", "0"),
	),
)

func callbackQueryHandler(bot *tgbotapi.BotAPI, query *tgbotapi.CallbackQuery) {
	res, err := strconv.ParseInt(query.Data, 10, 64)
	if err != nil {
		log.Println("Could not parse int:", err)
		return
	}

	var callbackResponse tgbotapi.CallbackConfig
	var action tgbotapi.Chattable

	if res == 0 {
		callbackResponse = tgbotapi.NewCallback(query.ID, "Ci ho provato...")
		action = tgbotapi.NewDeleteMessage(query.Message.Chat.ID, query.Message.MessageID)
	} else {
		callbackResponse = tgbotapi.NewCallback(query.ID, "Bene!")
		action = tgbotapi.NewEditMessageReplyMarkup(query.Message.Chat.ID, query.Message.MessageID, emptyKeyboard)
	}

	if _, err := bot.Request(callbackResponse); err != nil {
		panic(err)
	}
	_, err = bot.Request(action)
	if err != nil {
		log.Fatal(err)
	}

}
