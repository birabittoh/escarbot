package telegram

import (
	"log"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"
)

func channelPostHandler(escarbot *EscarBot, message *tgbotapi.Message) {
	chatId := message.Chat.ID
	escarbot.StateMutex.RLock()
	targetChannelID := escarbot.ChannelID
	targetGroupID := escarbot.GroupID
	escarbot.StateMutex.RUnlock()

	if chatId != targetChannelID {
		log.Println("Ignoring message since it did not come from the correct chat_id.")
		return
	}

	msg := tgbotapi.NewForward(targetGroupID, chatId, message.MessageID)
	_, err := escarbot.Bot.Send(msg)
	if err != nil {
		log.Println("Error forwarding message to group:", err)
	}
}

func forwardToAdmin(escarbot *EscarBot, message *tgbotapi.Message) {
	if !message.Chat.IsPrivate() {
		return
	}

	escarbot.StateMutex.RLock()
	adminID := escarbot.AdminID
	escarbot.StateMutex.RUnlock()

	msg := tgbotapi.NewForward(adminID, message.Chat.ID, message.MessageID)
	_, err := escarbot.Bot.Send(msg)
	if err != nil {
		log.Println("Error forwarding message to admin:", err)
	}
}
