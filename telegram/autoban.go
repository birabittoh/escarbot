package telegram

import (
	"log"
	"strings"
	"time"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"
)

// handleNewChatMembers handles new members joining the group
func handleNewChatMembers(escarbot *EscarBot, message *tgbotapi.Message) {
	if message.NewChatMembers == nil {
		return
	}

	for _, user := range message.NewChatMembers {
		// Ignore bots
		if user.IsBot {
			continue
		}

		// Check user's personal channel
		if hasBannedContent(escarbot, user.ID) {
			log.Printf("User %d (%s) has banned content in personal channel, proceeding with ban", user.ID, user.UserName)

			// Ban user and revoke all their messages
			banUser(escarbot.Bot, message.Chat.ID, user.ID)

			// Delete Telegram's join message
			deleteMsg := tgbotapi.NewDeleteMessage(message.Chat.ID, message.MessageID)
			_, err := escarbot.Bot.Request(deleteMsg)
			if err != nil {
				log.Printf("Error deleting join message: %v", err)
			} else {
				log.Printf("Join message deleted (chat %d)", message.Chat.ID)
			}

			// Delete welcome message from other bot
			go deleteNextMessage(escarbot, message.Chat.ID, message.MessageID)
		}
	}
}

// hasBannedContent checks if user has a personal channel with banned words
func hasBannedContent(escarbot *EscarBot, userID int64) bool {
	chatConfig := tgbotapi.ChatInfoConfig{
		ChatConfig: tgbotapi.ChatConfig{
			ChatID: userID,
		},
	}

	chat, err := escarbot.Bot.GetChat(chatConfig)
	if err != nil {
		log.Printf("Unable to get user info %d: %v", userID, err)
		return false
	}

	// Check personal channel (Personal Channel)
	if chat.PersonalChat != nil {
		channelName := strings.ToLower(chat.PersonalChat.Title)

		// Check for banned words (case insensitive)
		for _, word := range escarbot.BannedWords {
			if strings.Contains(channelName, strings.ToLower(word)) {
				log.Printf("Found banned word '%s' in personal channel: %s", word, chat.PersonalChat.Title)
				return true
			}
		}
	}

	return false
}

// banUser bans a user from the group
func banUser(bot *tgbotapi.BotAPI, chatID int64, userID int64) {
	banConfig := tgbotapi.BanChatMemberConfig{
		ChatMemberConfig: tgbotapi.ChatMemberConfig{
			ChatConfig: tgbotapi.ChatConfig{
				ChatID: chatID,
			},
			UserID: userID,
		},
		RevokeMessages: true,
	}

	_, err := bot.Request(banConfig)
	if err != nil {
		log.Printf("Error banning user %d: %v", userID, err)
	} else {
		log.Printf("User %d banned successfully", userID)
	}
}

// addMessageToCache adds a message to cache for future searches
func addMessageToCache(escarbot *EscarBot, message *tgbotapi.Message) {
	if message == nil || message.From == nil {
		return
	}

	escarbot.CacheMutex.Lock()
	defer escarbot.CacheMutex.Unlock()

	// Get available reactions for this chat
	reactions := getAvailableReactions(escarbot, message.Chat.ID)

	cached := CachedMessage{
		MessageID:          message.MessageID,
		ChatID:             message.Chat.ID,
		FromUsername:       message.From.UserName,
		FromFirstName:      message.From.FirstName,
		Text:               message.Text,
		Entities:           message.Entities,
		ThreadID:           message.MessageThreadID,
		IsTopicMessage:     message.IsTopicMessage,
		AvailableReactions: reactions,
		// Reactions field left empty - not available in current Message struct
	}

	// Prepend to keep newest messages first
	escarbot.MessageCache = append([]CachedMessage{cached}, escarbot.MessageCache...)

	// Keep only last N messages (remove from end)
	if len(escarbot.MessageCache) > escarbot.MaxCacheSize {
		escarbot.MessageCache = escarbot.MessageCache[:escarbot.MaxCacheSize]
	}

	// Broadcast message if callback is set
	if escarbot.OnMessageCached != nil {
		escarbot.OnMessageCached(cached)
	}
}

// deleteNextMessage deletes the welcome message from another bot
func deleteNextMessage(escarbot *EscarBot, chatID int64, messageID int) {
	// Wait 1 second to give the other bot time to send the welcome message
	time.Sleep(time.Second)

	deleteMsg := tgbotapi.NewDeleteMessage(chatID, messageID+1)
	_, err := escarbot.Bot.Request(deleteMsg)
	if err != nil {
		log.Printf("Error deleting welcome message: %v", err)
	} else {
		log.Printf("Welcome message deleted (chat %d, msg %d)", chatID, messageID+1)
	}

}
