package telegram

import (
	"log"
	"strconv"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"
)

type EscarBot struct {
	Bot            *tgbotapi.BotAPI
	Power          bool
	LinkDetection  bool
	ChannelForward bool
	AdminForward   bool
	ChannelID      int64
	GroupID        int64
	AdminID        int64
}

func NewBot(botToken string, channelId string, groupId string, adminId string) *EscarBot {
	bot, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		log.Panic(err)
	}

	//bot.Debug = true

	log.Printf("Authorized on account %s", bot.Self.UserName)

	channelIdInt, err := strconv.ParseInt(channelId, 10, 64)
	if err != nil {
		log.Fatal("Error while converting CHANNEL_ID to int64:", err)
	}

	groupIdInt, err := strconv.ParseInt(groupId, 10, 64)
	if err != nil {
		log.Fatal("Error while converting GROUP_ID to int64:", err)
	}

	adminIdInt, err := strconv.ParseInt(adminId, 10, 64)
	if err != nil {
		log.Fatal("Error while converting ADMIN_ID to int64:", err)
	}

	return &EscarBot{
		Bot:            bot,
		Power:          true,
		LinkDetection:  true,
		ChannelForward: true,
		AdminForward:   true,
		ChannelID:      channelIdInt,
		GroupID:        groupIdInt,
		AdminID:        adminIdInt,
	}
}

func BotPoll(escarbot *EscarBot) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	bot := escarbot.Bot
	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		msg := update.Message
		if msg != nil { // If we got a message
			if escarbot.LinkDetection {
				handleLinks(bot, msg)
			}
			if escarbot.AdminForward {
				forwardToAdmin(escarbot, msg)
			}
		}
		if update.ChannelPost != nil { // If we got a channel post
			if escarbot.ChannelForward {
				channelPostHandler(escarbot, update.ChannelPost)
			}
		}
	}
}
