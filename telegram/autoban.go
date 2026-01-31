package telegram

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"
)

func replacePlaceholders(escarbot *EscarBot, text string, user tgbotapi.User) string {
	replaced := strings.ReplaceAll(text, "{GROUP_ID}", strconv.FormatInt(escarbot.GroupID, 10))
	replaced = strings.ReplaceAll(replaced, "{USER_NAME}", user.FirstName)
	replaced = strings.ReplaceAll(replaced, "{USER_ID}", strconv.FormatInt(user.ID, 10))
	return replaced
}

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
		if escarbot.AutoBan && hasBannedContent(escarbot, user.ID) {
			log.Printf("User %d (%s) has banned content in personal channel, proceeding with ban", user.ID, user.UserName)

			// Ban user and revoke all their messages
			banUser(escarbot, message.Chat, user)

			// Delete Telegram's join message
			deleteMsg := tgbotapi.NewDeleteMessage(message.Chat.ID, message.MessageID)
			_, err := escarbot.Bot.Request(deleteMsg)
			if err != nil {
				log.Printf("Error deleting join message: %v", err)
			} else {
				log.Printf("Join message deleted (chat %d)", message.Chat.ID)
			}

			continue
		}

		if !escarbot.WelcomeMessage {
			continue
		}

		// Send welcome message
		links := strings.Split(escarbot.WelcomeLinks, "\n")
		var buttons [][]tgbotapi.InlineKeyboardButton
		for _, line := range links {
			parts := strings.SplitN(line, "|", 2)
			if len(parts) != 2 {
				continue
			}
			text := parts[0]
			url := replacePlaceholders(escarbot, parts[1], user)
			button := tgbotapi.NewInlineKeyboardButtonURL(text, url)
			buttons = append(buttons, tgbotapi.NewInlineKeyboardRow(button))
		}

		var welcomeMsg tgbotapi.Chattable
		if escarbot.WelcomePhoto == "" {
			msg := tgbotapi.NewMessage(message.Chat.ID, replacePlaceholders(escarbot, escarbot.WelcomeText, user))
			msg.ParseMode = "Markdown"
			if len(buttons) > 0 {
				msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(buttons...)
			}
			welcomeMsg = msg
		} else if escarbot.WelcomeText != "" {
			photo := tgbotapi.NewPhoto(message.Chat.ID, tgbotapi.FileURL(escarbot.WelcomePhoto))
			photo.Caption = replacePlaceholders(escarbot, escarbot.WelcomeText, user)
			photo.ParseMode = "Markdown"
			if len(buttons) > 0 {
				photo.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(buttons...)
			}
			welcomeMsg = photo
		} else {
			continue
		}

		_, err := escarbot.Bot.Send(welcomeMsg)
		if err != nil {
			log.Printf("Error sending welcome message to user %d: %v", user.ID, err)
		} else {
			log.Printf("Welcome message sent to user %d", user.ID)
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

		// Check for banned words
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
func banUser(escarbot *EscarBot, chat tgbotapi.Chat, user tgbotapi.User) {
	banConfig := tgbotapi.BanChatMemberConfig{
		ChatMemberConfig: tgbotapi.ChatMemberConfig{
			ChatConfig: tgbotapi.ChatConfig{
				ChatID: chat.ID,
			},
			UserID: user.ID,
		},
		RevokeMessages: true,
	}

	_, err := escarbot.Bot.Request(banConfig)
	if err != nil {
		log.Printf("Error banning user %d: %v", user.ID, err)
	} else {
		log.Printf("User %d banned successfully", user.ID)
	}

	msgText := strings.Builder{}
	msgText.WriteString("🚷 #BAN\n")
	msgText.WriteString(fmt.Sprintf("<b>User</b>: <a href=\"tg://user?id=%d\">%s</a> [<code>%d</code>]\n", user.ID, user.FirstName, user.ID))
	msgText.WriteString("#id" + strconv.FormatInt(user.ID, 10))

	logMsg := tgbotapi.NewMessage(escarbot.LogChannelID, msgText.String())
	logMsg.ParseMode = "HTML"
	logMsg.LinkPreviewOptions.IsDisabled = true
	_, err = escarbot.Bot.Send(logMsg)
	if err != nil {
		log.Printf("Error sending ban log message: %v", err)
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
