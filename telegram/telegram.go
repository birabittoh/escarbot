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
	Bot             *tgbotapi.BotAPI
	Power           bool
	LinkDetection   bool
	ChannelForward  bool
	AdminForward    bool
	AutoBan         bool
	ChannelID       int64
	GroupID         int64
	AdminID         int64
	BannedWords     []string
	MessageCache    map[int64][]CachedMessage
	CacheMutex      sync.Mutex
	MaxCacheSize    int
	OnMessageCached func(CachedMessage) // Callback for when a message is cached
}

// CachedMessage represents a message stored in cache
type CachedMessage struct {
	MessageID      int                      `json:"message_id"`
	ChatID         int64                    `json:"chat_id"`
	FromUsername   string                   `json:"from_username"`
	FromFirstName  string                   `json:"from_first_name"`
	Text           string                   `json:"text"`
	Entities       []tgbotapi.MessageEntity `json:"entities,omitempty"`
	ThreadID       int                      `json:"thread_id,omitempty"`
	IsTopicMessage bool                     `json:"is_topic_message"`
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

	// Read boolean settings from environment (default to true if not set)
	linkDetection := getBoolEnv("LINK_DETECTION", true)
	channelForward := getBoolEnv("CHANNEL_FORWARD", true)
	adminForward := getBoolEnv("ADMIN_FORWARD", true)
	autoBan := getBoolEnv("AUTO_BAN", true)

	return &EscarBot{
		Bot:            bot,
		Power:          true,
		LinkDetection:  linkDetection,
		ChannelForward: channelForward,
		AdminForward:   adminForward,
		AutoBan:        autoBan,
		ChannelID:      channelIdInt,
		GroupID:        groupIdInt,
		AdminID:        adminIdInt,
		BannedWords:    bannedWords,
		MessageCache:   make(map[int64][]CachedMessage),
		MaxCacheSize:   100,
	}
}

// getBoolEnv reads a boolean environment variable with a default value
func getBoolEnv(key string, defaultValue bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value == "true" || value == "1"
}

func BotPoll(escarbot *EscarBot) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	bot := escarbot.Bot
	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		msg := update.Message
		if msg != nil { // If we got a message
			addMessageToCache(escarbot, msg)

			if escarbot.AutoBan {
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
