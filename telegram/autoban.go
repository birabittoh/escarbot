package telegram

import (
	"fmt"
	"html"
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
	replaced = strings.ReplaceAll(replaced, "{USER_NAME}", html.EscapeString(user.FirstName))
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
	if user.IsBot {
		return
	}

	// Cleanup stale in-memory entries (no-op when Valkey handles TTL).
	escarbot.Cache.CleanupJoinEntries()

	// De-duplication: if processed within the last 10 seconds, skip.
	if lastProcessed, exists := escarbot.Cache.GetJoinEntry(user.ID); exists {
		if time.Since(lastProcessed.Time) < 10*time.Second {
			// Update join message ID in both join and captcha caches if needed.
			if joinMsgID != 0 && lastProcessed.JoinMsgID == 0 {
				updated, _ := escarbot.Cache.UpdateJoinEntryMsgID(user.ID, joinMsgID)
				if updated != nil && updated.IsBanned {
					deleteMessages(escarbot, chatID, joinMsgID)
					log.Printf("Deleted late join message %d for banned user %d", joinMsgID, user.ID)
				}
			}
			escarbot.Cache.UpdateCaptchaJoinMsgID(user.ID, joinMsgID)
			return
		}
	}

	entry := &JoinProcessedEntry{
		Time:      time.Now(),
		JoinMsgID: joinMsgID,
		IsBanned:  false,
	}
	escarbot.Cache.SetJoinEntry(user.ID, entry)

	escarbot.StateMutex.RLock()
	autoBan := escarbot.AutoBan
	captcha := escarbot.Captcha
	welcomeMessage := escarbot.WelcomeMessage
	escarbot.StateMutex.RUnlock()

	if autoBan && hasBannedContent(escarbot, user.ID) {
		log.Printf("User %d (%s) has banned content in personal channel, proceeding with ban", user.ID, user.UserName)
		banAndCleanup(escarbot, chatID, user, joinMsgID)
		escarbot.Cache.UpdateJoinEntryBanned(user.ID)
		return
	}

	if captcha {
		restrictUser(escarbot, chatID, user.ID)
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
		msg.ParseMode = "HTML"
		if len(buttons) > 0 {
			msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(buttons...)
		}
		welcomeMsg = msg
	} else if welcomeText != "" {
		photo := tgbotapi.NewPhoto(chatID, tgbotapi.FileURL(welcomePhoto))
		photo.Caption = replacePlaceholders(escarbot, welcomeText, user)
		photo.ParseMode = "HTML"
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
		ChatConfig: tgbotapi.ChatConfig{ChatID: userID},
	}

	chat, err := escarbot.Bot.GetChat(chatConfig)
	if err != nil {
		log.Printf("Unable to get user info %d: %v", userID, err)
		return false
	}

	if chat.PersonalChat != nil {
		channelName := strings.ToLower(chat.PersonalChat.Title)

		escarbot.StateMutex.RLock()
		bannedWords := make([]string, len(escarbot.BannedWords))
		copy(bannedWords, escarbot.BannedWords)
		escarbot.StateMutex.RUnlock()

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
			ChatConfig: tgbotapi.ChatConfig{ChatID: chatID},
			UserID:     user.ID,
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

	userName := "Anonymous"
	if update.User != nil {
		userName = update.User.FirstName
	} else if update.ActorChat != nil {
		userName = update.ActorChat.Title
	}

	msg, updated := escarbot.Cache.UpdateIndividualReaction(update.Chat.ID, update.MessageID, userName, update.NewReaction)
	if updated && escarbot.OnMessageCached != nil {
		escarbot.OnMessageCached(msg)
	}
}

// banAndCleanup bans a user and deletes relevant messages
func banAndCleanup(escarbot *EscarBot, chatID int64, user tgbotapi.User, messageIDs ...int) {
	banUser(escarbot, chatID, user)
	deleteMessages(escarbot, chatID, messageIDs...)
}

// AddMessageToCache adds a message to the recent messages cache
func AddMessageToCache(escarbot *EscarBot, message *tgbotapi.Message) {
	if message == nil {
		return
	}

	// Fetch chat info if not already cached.
	chatInfo, exists := escarbot.Cache.GetChatInfo(message.Chat.ID)
	if !exists {
		chatTitle := message.Chat.Title
		if chatTitle == "" {
			chatTitle = message.Chat.FirstName
		}

		photoURL := ""
		chatConfig := tgbotapi.ChatInfoConfig{ChatConfig: tgbotapi.ChatConfig{ChatID: message.Chat.ID}}
		fullChat, err := escarbot.Bot.GetChat(chatConfig)
		if err == nil && fullChat.Photo != nil {
			photoURL = "/api/media?file_id=" + fullChat.Photo.SmallFileID
		}

		chatInfo = ChatInfo{
			ID:       message.Chat.ID,
			Title:    chatTitle,
			PhotoURL: photoURL,
		}
		escarbot.Cache.SetChatInfo(message.Chat.ID, chatInfo)
	}

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
	}

	added := escarbot.Cache.AddMessage(message.Chat.ID, cached, escarbot.MaxCacheSize)
	if !added {
		return
	}

	log.Printf("Added message %d from chat %d (%s) to cache",
		message.MessageID, message.Chat.ID, chatInfo.Title)

	if escarbot.OnMessageCached != nil {
		escarbot.OnMessageCached(cached)
	}
}

// UpdateMessageInCache updates a message in the recent messages cache
func UpdateMessageInCache(escarbot *EscarBot, message *tgbotapi.Message) {
	if message == nil {
		return
	}

	updated, found := escarbot.Cache.UpdateMessage(message.Chat.ID, message)
	if !found {
		AddMessageToCache(escarbot, message)
		return
	}
	if escarbot.OnMessageCached != nil && found {
		escarbot.OnMessageCached(updated)
	}
}

// updateReactionsInCache updates reaction counts for a cached message
func updateReactionsInCache(escarbot *EscarBot, update *tgbotapi.MessageReactionCountUpdated) {
	if update == nil {
		return
	}

	msg, found := escarbot.Cache.UpdateReactions(update.Chat.ID, update.MessageID, update.Reactions)
	if found {
		log.Printf("Updated reactions for message %d in cache", update.MessageID)
		if escarbot.OnMessageCached != nil {
			escarbot.OnMessageCached(msg)
		}
	}
}
