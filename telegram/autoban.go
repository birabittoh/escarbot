package telegram

import (
	"fmt"
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

			// Search and delete welcome message from other bot in cache
			go findAndDeleteWelcomeMessage(escarbot, message.Chat.ID, user.ID, user.UserName)
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

	cached := CachedMessage{
		MessageID:      message.MessageID,
		ChatID:         message.Chat.ID,
		FromUsername:   message.From.UserName,
		FromFirstName:  message.From.FirstName,
		Text:           message.Text,
		Entities:       message.Entities,
		ThreadID:       message.MessageThreadID,
		IsTopicMessage: message.IsTopicMessage,
	}

	cache := escarbot.MessageCache[message.Chat.ID]
	cache = append(cache, cached)

	// Keep only last N messages
	if len(cache) > escarbot.MaxCacheSize {
		cache = cache[1:]
	}

	escarbot.MessageCache[message.Chat.ID] = cache

	// Broadcast message if callback is set
	if escarbot.OnMessageCached != nil {
		escarbot.OnMessageCached(cached)
	}
}

// findAndDeleteWelcomeMessage searches and deletes the welcome message from another bot
func findAndDeleteWelcomeMessage(escarbot *EscarBot, chatID int64, userID int64, userName string) {
	// Wait 1 second to give the other bot time to send the welcome message
	time.Sleep(1 * time.Second)

	escarbot.CacheMutex.Lock()
	cache := escarbot.MessageCache[chatID]
	escarbot.CacheMutex.Unlock()

	// Search in recent messages (from newest to oldest)
	for i := len(cache) - 1; i >= 0; i-- {
		msg := cache[i]

		// We can't reliably determine if sender is a bot from cache anymore
		// So we check all messages and look for user mentions

		// Check if message contains the nickname or user ID
		if containsUser(msg, userID, userName) {
			deleteMsg := tgbotapi.NewDeleteMessage(chatID, msg.MessageID)
			_, err := escarbot.Bot.Request(deleteMsg)
			if err != nil {
				log.Printf("Error deleting welcome message: %v", err)
			} else {
				log.Printf("Welcome message found and deleted (chat %d, msg %d)", chatID, msg.MessageID)
			}

			// Remove from cache
			escarbot.CacheMutex.Lock()
			cache = append(cache[:i], cache[i+1:]...)
			escarbot.MessageCache[chatID] = cache
			escarbot.CacheMutex.Unlock()

			return
		}
	}

	log.Printf("No welcome message found for user %d", userID)
}

// containsUser checks if a message contains references to a specific user
func containsUser(msg CachedMessage, userID int64, userName string) bool {
	// Check in message text
	userIDStr := fmt.Sprintf("%d", userID)
	if strings.Contains(msg.Text, userIDStr) || (userName != "" && strings.Contains(msg.Text, userName)) {
		return true
	}

	// Check in entities (text_mention, mention)
	for _, entity := range msg.Entities {
		// Text mention: when user is mentioned without username
		if entity.Type == "text_mention" && entity.User != nil && entity.User.ID == userID {
			return true
		}

		// Username mention: @username
		if entity.Type == "mention" && userName != "" {
			// Extract mention from text
			if entity.Offset+entity.Length <= len(msg.Text) {
				mention := msg.Text[entity.Offset : entity.Offset+entity.Length]
				if strings.TrimPrefix(mention, "@") == userName {
					return true
				}
			}
		}
	}

	return false
}
