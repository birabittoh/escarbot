package telegram

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"
)

func replacePlaceholders(escarbot *EscarBot, text string, user tgbotapi.User) string {
	escarbot.StateMutex.RLock()
	groupID := escarbot.GroupID
	escarbot.StateMutex.RUnlock()

	replaced := strings.ReplaceAll(text, "{GROUP_ID}", strconv.FormatInt(groupID, 10))
	replaced = strings.ReplaceAll(replaced, "{USER_NAME}", user.FirstName)
	replaced = strings.ReplaceAll(replaced, "{USER_ID}", strconv.FormatInt(user.ID, 10))
	return replaced
}

// handleNewChatMembers handles new members joining the group via service message
func handleNewChatMembers(escarbot *EscarBot, message *tgbotapi.Message) {
	if message.NewChatMembers == nil {
		return
	}

	for _, user := range message.NewChatMembers {
		processJoin(escarbot, message.Chat.ID, user, message.MessageID)
	}
}

// handleChatMemberUpdate handles updates to a chat member's status
func handleChatMemberUpdate(escarbot *EscarBot, update *tgbotapi.ChatMemberUpdated) {
	// We only care about users joining the group
	// Status change from (left/kicked) to member/restricted
	oldStatus := update.OldChatMember.Status
	newStatus := update.NewChatMember.Status

	isJoining := (oldStatus == "left" || oldStatus == "kicked") &&
		(newStatus == "member" || newStatus == "restricted")

	if isJoining && update.NewChatMember.User != nil {
		log.Printf("User %d joined chat %d (detected via ChatMemberUpdated)", update.NewChatMember.User.ID, update.Chat.ID)
		processJoin(escarbot, update.Chat.ID, *update.NewChatMember.User, 0)
	}
}

// processJoin handles a user joining the group
func processJoin(escarbot *EscarBot, chatID int64, user tgbotapi.User, joinMsgID int) {
	// Ignore bots
	if user.IsBot {
		return
	}

	// De-duplication logic
	escarbot.JoinCacheMutex.Lock()
	// Cleanup old entries (older than 1 minute) periodically
	// We do it here for simplicity
	for id, t := range escarbot.JoinProcessedCache {
		if time.Since(t) > 1*time.Minute {
			delete(escarbot.JoinProcessedCache, id)
		}
	}

	lastProcessed, exists := escarbot.JoinProcessedCache[user.ID]
	// If processed in the last 10 seconds, it's a duplicate
	if exists && time.Since(lastProcessed) < 10*time.Second {
		escarbot.JoinCacheMutex.Unlock()

		// Even if it's a duplicate, we might want to update the joinMsgID for captcha
		escarbot.CaptchaMutex.Lock()
		if pending, ok := escarbot.PendingCaptchas[user.ID]; ok {
			if joinMsgID != 0 && pending.JoinMsgID == 0 {
				pending.JoinMsgID = joinMsgID
				log.Printf("Updated JoinMsgID for user %d in PendingCaptchas", user.ID)
			}
		}
		escarbot.CaptchaMutex.Unlock()
		return
	}
	escarbot.JoinProcessedCache[user.ID] = time.Now()
	escarbot.JoinCacheMutex.Unlock()

	escarbot.StateMutex.RLock()
	autoBan := escarbot.AutoBan
	captcha := escarbot.Captcha
	welcomeMessage := escarbot.WelcomeMessage
	escarbot.StateMutex.RUnlock()

	// Check user's personal channel
	if autoBan && hasBannedContent(escarbot, user.ID) {
		log.Printf("User %d (%s) has banned content in personal channel, proceeding with ban", user.ID, user.UserName)

		// Ban user and cleanup join message
		banAndCleanup(escarbot, chatID, user, joinMsgID)

		return
	}

	if captcha {
		SendCaptcha(escarbot, chatID, user, joinMsgID, 0)
		return
	}

	if !welcomeMessage {
		return
	}

	sendWelcomeMessage(escarbot, chatID, user)
}

func sendWelcomeMessage(escarbot *EscarBot, chatID int64, user tgbotapi.User) {
	escarbot.StateMutex.RLock()
	welcomeLinks := escarbot.WelcomeLinks
	welcomePhoto := escarbot.WelcomePhoto
	welcomeText := escarbot.WelcomeText
	escarbot.StateMutex.RUnlock()

	// Send welcome message
	links := strings.Split(welcomeLinks, "\n")
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
	if welcomePhoto == "" {
		msg := tgbotapi.NewMessage(chatID, replacePlaceholders(escarbot, welcomeText, user))
		msg.ParseMode = "Markdown"
		if len(buttons) > 0 {
			msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(buttons...)
		}
		welcomeMsg = msg
	} else if welcomeText != "" {
		photo := tgbotapi.NewPhoto(chatID, tgbotapi.FileURL(welcomePhoto))
		photo.Caption = replacePlaceholders(escarbot, welcomeText, user)
		photo.ParseMode = "Markdown"
		if len(buttons) > 0 {
			photo.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(buttons...)
		}
		welcomeMsg = photo
	} else {
		return
	}

	_, err := escarbot.Bot.Send(welcomeMsg)
	if err != nil {
		log.Printf("Error sending welcome message to user %d: %v", user.ID, err)
	} else {
		log.Printf("Welcome message sent to user %d", user.ID)
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

		escarbot.StateMutex.RLock()
		bannedWords := make([]string, len(escarbot.BannedWords))
		copy(bannedWords, escarbot.BannedWords)
		escarbot.StateMutex.RUnlock()

		// Check for banned words
		for _, word := range bannedWords {
			if strings.Contains(channelName, strings.ToLower(word)) {
				log.Printf("Found banned word '%s' in personal channel: %s", word, chat.PersonalChat.Title)
				return true
			}
		}
	}

	return false
}

// banUser bans a user from the group
func banUser(escarbot *EscarBot, chatID int64, user tgbotapi.User) {
	banConfig := tgbotapi.BanChatMemberConfig{
		ChatMemberConfig: tgbotapi.ChatMemberConfig{
			ChatConfig: tgbotapi.ChatConfig{
				ChatID: chatID,
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

	escarbot.StateMutex.RLock()
	logChannelID := escarbot.LogChannelID
	escarbot.StateMutex.RUnlock()

	msgText := strings.Builder{}
	msgText.WriteString("🚷 #BAN\n")
	msgText.WriteString(fmt.Sprintf("<b>User</b>: <a href=\"tg://user?id=%d\">%s</a> [<code>%d</code>]\n", user.ID, user.FirstName, user.ID))
	msgText.WriteString("#id" + strconv.FormatInt(user.ID, 10))

	logMsg := tgbotapi.NewMessage(logChannelID, msgText.String())
	logMsg.ParseMode = "HTML"
	logMsg.LinkPreviewOptions.IsDisabled = true
	_, err = escarbot.Bot.Send(logMsg)
	if err != nil {
		log.Printf("Error sending ban log message: %v", err)
	}
}

// deleteMessages deletes multiple messages from a chat
func deleteMessages(escarbot *EscarBot, chatID int64, messageIDs ...int) {
	for _, id := range messageIDs {
		if id == 0 {
			continue
		}
		deleteMsg := tgbotapi.NewDeleteMessage(chatID, id)
		_, err := escarbot.Bot.Request(deleteMsg)
		if err != nil {
			log.Printf("Error deleting message %d in chat %d: %v", id, chatID, err)
		}
	}
}

// updateIndividualReactionInCache updates individual reactions for a cached message
func updateIndividualReactionInCache(escarbot *EscarBot, update *tgbotapi.MessageReactionUpdated) {
	if update == nil {
		return
	}

	escarbot.CacheMutex.Lock()
	defer escarbot.CacheMutex.Unlock()

	chatMessages, ok := escarbot.MessageCache[update.Chat.ID]
	if !ok {
		return
	}

	for i, msg := range chatMessages {
		if msg.MessageID == update.MessageID {
			// Find user name
			userName := "Anonymous"
			if update.User != nil {
				userName = update.User.FirstName
			} else if update.ActorChat != nil {
				userName = update.ActorChat.Title
			}

			// Deduplicate and update reaction in the list
			updated := false

			// Remove existing entries for this user first
			newReactions := []ReactionDetail{}
			for _, r := range msg.RecentReactions {
				if r.User != userName {
					newReactions = append(newReactions, r)
				} else {
					updated = true // Something changed
				}
			}

			// Add the new reaction if present
			if len(update.NewReaction) > 0 {
				// Only take the first emoji reaction for simplicity
				for _, reaction := range update.NewReaction {
					if reaction.Type == "emoji" {
						detail := ReactionDetail{
							User:  userName,
							Emoji: reaction.Emoji,
						}
						// Prepend
						newReactions = append([]ReactionDetail{detail}, newReactions...)
						updated = true
						break
					}
				}
			}

			// Limit to 5
			if len(newReactions) > 5 {
				newReactions = newReactions[:5]
			}
			escarbot.MessageCache[update.Chat.ID][i].RecentReactions = newReactions

			if updated {
				log.Printf("Updated individual reactions for message %d in cache", update.MessageID)
				// Broadcast update
				if escarbot.OnMessageCached != nil {
					escarbot.OnMessageCached(escarbot.MessageCache[update.Chat.ID][i])
				}
			}
			return
		}
	}
}

// banAndCleanup bans a user and deletes relevant messages
func banAndCleanup(escarbot *EscarBot, chatID int64, user tgbotapi.User, messageIDs ...int) {
	banUser(escarbot, chatID, user)
	deleteMessages(escarbot, chatID, messageIDs...)
}

// addMessageToCache adds a message to cache for future searches
func addMessageToCache(escarbot *EscarBot, message *tgbotapi.Message) {
	if message == nil {
		return
	}

	escarbot.CacheMutex.Lock()
	// Check if already in cache (avoid duplicates)
	chatMessages := escarbot.MessageCache[message.Chat.ID]
	for _, msg := range chatMessages {
		if msg.MessageID == message.MessageID {
			escarbot.CacheMutex.Unlock()
			return
		}
	}

	// Handle Chat Cache
	chatInfo, exists := escarbot.ChatCache[message.Chat.ID]
	escarbot.CacheMutex.Unlock()

	if !exists {
		chatTitle := message.Chat.Title
		if chatTitle == "" {
			chatTitle = message.Chat.FirstName
		}

		chatPhotoURL := ""
		// Fetch full chat info to get photo (Network call outside mutex)
		chatConfig := tgbotapi.ChatInfoConfig{
			ChatConfig: tgbotapi.ChatConfig{
				ChatID: message.Chat.ID,
			},
		}
		fullChat, err := escarbot.Bot.GetChat(chatConfig)
		if err == nil && fullChat.Photo != nil {
			chatPhotoURL = "/api/media?file_id=" + fullChat.Photo.SmallFileID
		}

		chatInfo = ChatInfo{
			ID:       message.Chat.ID,
			Title:    chatTitle,
			PhotoURL: chatPhotoURL,
		}

		escarbot.CacheMutex.Lock()
		escarbot.ChatCache[message.Chat.ID] = chatInfo
		escarbot.CacheMutex.Unlock()
	}

	escarbot.CacheMutex.Lock()
	defer escarbot.CacheMutex.Unlock()

	// Re-verify if it was added while we were fetching chat info
	chatMessages = escarbot.MessageCache[message.Chat.ID]
	for _, msg := range chatMessages {
		if msg.MessageID == message.MessageID {
			return
		}
	}

	// Get available reactions for this chat
	reactions := getAvailableReactions(escarbot, message.Chat.ID)

	fromUsername := ""
	fromFirstName := "Channel"
	if message.From != nil {
		fromUsername = message.From.UserName
		fromFirstName = message.From.FirstName
	} else if message.SenderChat != nil {
		fromFirstName = message.SenderChat.Title
	}

	var mediaURL, mediaType string
	if len(message.Photo) > 0 {
		mediaType = "photo"
		// Use the largest photo
		photo := message.Photo[len(message.Photo)-1]
		mediaURL = "/api/media?file_id=" + photo.FileID
	} else if message.Sticker != nil {
		mediaType = "sticker"
		mediaURL = "/api/media?file_id=" + message.Sticker.FileID
	}

	cached := CachedMessage{
		MessageID:          message.MessageID,
		ChatID:             message.Chat.ID,
		ChatTitle:          chatInfo.Title,
		ChatPhotoURL:       chatInfo.PhotoURL,
		FromUsername:       fromUsername,
		FromFirstName:      fromFirstName,
		Text:               message.Text,
		Caption:            message.Caption,
		MediaURL:           mediaURL,
		MediaType:          mediaType,
		Entities:           message.Entities,
		ThreadID:           message.MessageThreadID,
		IsTopicMessage:     message.IsTopicMessage,
		AvailableReactions: reactions,
		// Reactions field left empty - not available in current Message struct
	}

	// Prepend to keep newest messages first
	escarbot.MessageCache[message.Chat.ID] = append([]CachedMessage{cached}, chatMessages...)

	// Keep only last N messages (remove from end)
	if len(escarbot.MessageCache[message.Chat.ID]) > escarbot.MaxCacheSize {
		escarbot.MessageCache[message.Chat.ID] = escarbot.MessageCache[message.Chat.ID][:escarbot.MaxCacheSize]
	}

	// Broadcast message if callback is set
	if escarbot.OnMessageCached != nil {
		escarbot.OnMessageCached(cached)
	}
}

// updateMessageInCache updates an existing message in the cache with its new content and saves history
func updateMessageInCache(escarbot *EscarBot, message *tgbotapi.Message) {
	if message == nil {
		return
	}

	escarbot.CacheMutex.Lock()
	defer escarbot.CacheMutex.Unlock()

	chatMessages, ok := escarbot.MessageCache[message.Chat.ID]
	if !ok {
		addMessageToCache(escarbot, message)
		return
	}

	for i, msg := range chatMessages {
		if msg.MessageID == message.MessageID {
			// Save current state to history
			historyItem := MessageHistory{
				Text:     msg.Text,
				Caption:  msg.Caption,
				EditDate: message.EditDate,
			}
			escarbot.MessageCache[message.Chat.ID][i].History = append(escarbot.MessageCache[message.Chat.ID][i].History, historyItem)

			// Update content
			escarbot.MessageCache[message.Chat.ID][i].Text = message.Text
			escarbot.MessageCache[message.Chat.ID][i].Caption = message.Caption
			escarbot.MessageCache[message.Chat.ID][i].Entities = message.Entities

			// Broadcast update
			if escarbot.OnMessageCached != nil {
				escarbot.OnMessageCached(escarbot.MessageCache[message.Chat.ID][i])
			}
			return
		}
	}
	// If not in cache, add it as new (though we won't have history)
	addMessageToCache(escarbot, message)
}

// updateReactionsInCache updates reaction counts for a cached message
func updateReactionsInCache(escarbot *EscarBot, update *tgbotapi.MessageReactionCountUpdated) {
	if update == nil {
		return
	}

	escarbot.CacheMutex.Lock()
	defer escarbot.CacheMutex.Unlock()

	chatMessages, ok := escarbot.MessageCache[update.Chat.ID]
	if !ok {
		return
	}

	for i, msg := range chatMessages {
		if msg.MessageID == update.MessageID {
			escarbot.MessageCache[update.Chat.ID][i].Reactions = update.Reactions

			log.Printf("Updated reactions for message %d in cache", update.MessageID)

			// Broadcast update
			if escarbot.OnMessageCached != nil {
				escarbot.OnMessageCached(escarbot.MessageCache[update.Chat.ID][i])
			}
			return
		}
	}
}
