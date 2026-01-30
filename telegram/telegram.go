package telegram

import (
	"log"
	"os"
	"strconv"
	"strings"
	"sync"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"
)

type EscarBot struct {
	Bot            *tgbotapi.BotAPI
	Power          bool
	LinkDetection  bool
	ChannelForward bool
	AdminForward   bool
	AutoBan        bool
	ChannelID      int64
	GroupID        int64
	AdminID        int64
	BannedWords    []string
	MessageCache   map[int64][]CachedMessage
	CacheMutex     sync.Mutex
	MaxCacheSize   int
}

// CachedMessage represents a message stored in cache
type CachedMessage struct {
	MessageID int
	From      *tgbotapi.User
	Text      string
	Entities  []tgbotapi.MessageEntity
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

	// Read banned words from environment variable (comma-separated)
	bannedWordsEnv := os.Getenv("BANNED_WORDS")
	var bannedWords []string
	if bannedWordsEnv != "" {
		bannedWords = strings.Split(bannedWordsEnv, ",")
		// Trim whitespace from each word
		for i, word := range bannedWords {
			bannedWords[i] = strings.TrimSpace(word)
		}
		log.Printf("Loaded %d banned words from BANNED_WORDS env", len(bannedWords))
	} else {
		// Default banned words if env not set
		bannedWords = []string{"18+"}
		log.Printf("Using default banned words: %v", bannedWords)
	}

	return &EscarBot{
		Bot:            bot,
		Power:          true,
		LinkDetection:  true,
		ChannelForward: true,
		AdminForward:   true,
		AutoBan:        true,
		ChannelID:      channelIdInt,
		GroupID:        groupIdInt,
		AdminID:        adminIdInt,
		BannedWords:    bannedWords,
		MessageCache:   make(map[int64][]CachedMessage),
		MaxCacheSize:   15,
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
			if escarbot.AutoBan {
				addMessageToCache(escarbot, msg)
				handleNewChatMembers(escarbot, msg)
			}
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
