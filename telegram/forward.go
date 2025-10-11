package telegram

import (
	"log"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"
)

func channelPostHandler(escarbot *EscarBot, message *tgbotapi.Message) {
	chatId := message.Chat.ID
	if chatId != escarbot.ChannelID {
		log.Println("Ignoring message since it did not come from the correct chat_id.")
		return
	}

	msg := tgbotapi.NewForward(escarbot.GroupID, chatId, message.MessageID)
	_, err := escarbot.Bot.Send(msg)
	if err != nil {
		log.Println("Error forwarding message to group:", err)
	}
}

func forwardToAdmin(escarbot *EscarBot, message *tgbotapi.Message) {
	if !message.Chat.IsPrivate() {
		return
	}
	msg := tgbotapi.NewForward(escarbot.AdminID, message.Chat.ID, message.MessageID)
	_, err := escarbot.Bot.Send(msg)
	if err != nil {
		log.Println("Error forwarding message to admin:", err)
	}
}
