package telegram

import (
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"
)

const maxCacheSize = 100
const startupGracePeriod = 5 * time.Minute

type EscarBot struct {
	Bot                *tgbotapi.BotAPI
	Power              bool
	LinkDetection      bool
	ChannelForward     bool
	AdminForward       bool
	AutoBan            bool
	Captcha            bool
	CaptchaTimeout     int
	CaptchaMaxRetries  int
	WelcomeMessage     bool
	ChannelID          int64
	GroupID            int64
	AdminID            int64
	LogChannelID       int64
	BannedWords        []string
	AvailableReactions map[int64][]string
	MessageCache       map[int64][]CachedMessage
	ChatCache          map[int64]ChatInfo
	CacheMutex         sync.Mutex
	ReactionMutex      sync.Mutex
	StateMutex         sync.RWMutex
	MaxCacheSize       int
	OnMessageCached    func(CachedMessage) // Callback for when a message is cached
	WelcomeText        string
	WelcomeLinks       string
	WelcomePhoto       string
	CaptchaText        string
	EnabledReplacers   map[string]bool
	PendingCaptchas    map[int64]*PendingCaptcha
	CaptchaMutex       sync.RWMutex
	JoinProcessedCache map[int64]*JoinProcessedEntry
	JoinCacheMutex     sync.Mutex
	VerifiedUsers      map[int64]bool
	VerifiedMutex      sync.RWMutex
	StartupTime        time.Time
}

// JoinProcessedEntry represents a join event that was already processed
type JoinProcessedEntry struct {
	Time      time.Time
	JoinMsgID int
	IsBanned  bool
}

// MessageHistory represents a previous version of a message
type MessageHistory struct {
	Text     string `json:"text"`
	Caption  string `json:"caption,omitempty"`
	EditDate int    `json:"edit_date"`
}

// ReactionDetail represents an individual reaction by a user
type ReactionDetail struct {
	User  string `json:"user"`
	Emoji string `json:"emoji"`
}

// ChatInfo represents information about a Telegram chat
type ChatInfo struct {
	ID       int64  `json:"id,string"`
	Title    string `json:"title"`
	PhotoURL string `json:"photo_url,omitempty"`
}

// CachedMessage represents a message stored in cache
type CachedMessage struct {
	MessageID          int                      `json:"message_id"`
	ChatID             int64                    `json:"chat_id,string"`
	ChatTitle          string                   `json:"chat_title,omitempty"`
	ChatPhotoURL       string                   `json:"chat_photo_url,omitempty"`
	FromUsername       string                   `json:"from_username"`
	FromFirstName      string                   `json:"from_first_name"`
	Text               string                   `json:"text"`
	Caption            string                   `json:"caption,omitempty"`
	MediaURL           string                   `json:"media_url,omitempty"`
	MediaType          string                   `json:"media_type,omitempty"`
	Entities           []tgbotapi.MessageEntity `json:"entities,omitempty"`
	ThreadID           int                      `json:"thread_id,omitempty"`
	IsTopicMessage     bool                     `json:"is_topic_message"`
	AvailableReactions []string                 `json:"available_reactions,omitempty"`
	Reactions          []tgbotapi.ReactionCount `json:"reactions,omitempty"`
	RecentReactions    []ReactionDetail         `json:"recent_reactions,omitempty"`
	BotReaction        string                   `json:"bot_reaction,omitempty"`
	History            []MessageHistory         `json:"history,omitempty"`
}

// getAvailableReactions returns the available reactions for a chat
// If not cached, it fetches them from Telegram API
func getAvailableReactions(escarbot *EscarBot, chatID int64) []string {
	// Check if already cached
	escarbot.ReactionMutex.Lock()
	if escarbot.AvailableReactions == nil {
		escarbot.AvailableReactions = make(map[int64][]string)
	}
	if reactions, exists := escarbot.AvailableReactions[chatID]; exists {
		escarbot.ReactionMutex.Unlock()
		return reactions
	}
	escarbot.ReactionMutex.Unlock()

	// Not cached, fetch from API
	defaultReactions := []string{"👍", "👎", "❤️", "🔥", "🥰", "👏", "😁", "🤔", "🤯", "😱", "🤬", "😢", "🎉", "🤩", "🤮", "💩", "🙏", "👌", "🕊", "🤡", "🥱", "🥴", "😍", "🐳", "❤️‍🔥", "🌚", "🌭", "💯", "🤣", "⚡", "🍌", "🏆", "💔", "🤨", "😐", "🍓", "🍾", "💋", "🖕", "😈", "😴", "😭", "🤓", "👻", "👨‍💻", "👀", "🎃", "🙈", "😇", "😨"}

	chatConfig := tgbotapi.ChatInfoConfig{
		ChatConfig: tgbotapi.ChatConfig{
			ChatID: chatID,
		},
	}
	chatInfo, err := escarbot.Bot.GetChat(chatConfig)

	var reactions []string
	if err != nil {
		log.Printf("Warning: could not get chat info for reactions (chat %d): %v", chatID, err)
		reactions = defaultReactions
	} else if len(chatInfo.AvailableReactions) == 0 {
		// Empty means all reactions are allowed
		reactions = defaultReactions
	} else {
		// Extract emoji reactions
		for _, reaction := range chatInfo.AvailableReactions {
			if reaction.Type == "emoji" && reaction.Emoji != "" {
				reactions = append(reactions, reaction.Emoji)
			}
		}
		if len(reactions) == 0 {
			reactions = defaultReactions
		}
	}

	// Cache the result
	escarbot.ReactionMutex.Lock()
	escarbot.AvailableReactions[chatID] = reactions
	escarbot.ReactionMutex.Unlock()
	log.Printf("Loaded %d available reactions for chat %d", len(reactions), chatID)

	return reactions
}

func NewBot(botToken string, channelId string, groupId string, adminId, logChannelId string) *EscarBot {
	bot, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		log.Printf("Warning: failed to initialize bot with token: %v", err)
		// For testing/development, create a mock bot if token is dummy or invalid
		bot = &tgbotapi.BotAPI{Self: tgbotapi.User{UserName: "OfflineBot"}}
	} else {
		log.Printf("Authorized on account %s", bot.Self.UserName)
	}

	//bot.Debug = true

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

	logChannelIdInt, err := strconv.ParseInt(logChannelId, 10, 64)
	if err != nil {
		log.Fatal("Error while converting LOG_CHANNEL_ID to int64:", err)
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
	captcha := getBoolEnv("CAPTCHA", true)
	captchaTimeoutStr := os.Getenv("CAPTCHA_TIMEOUT")
	captchaTimeout := 120
	if captchaTimeoutStr != "" {
		if val, err := strconv.Atoi(captchaTimeoutStr); err == nil {
			captchaTimeout = val
		}
	}

	captchaMaxRetriesStr := os.Getenv("CAPTCHA_MAX_RETRIES")
	captchaMaxRetries := 2
	if captchaMaxRetriesStr != "" {
		if val, err := strconv.Atoi(captchaMaxRetriesStr); err == nil {
			captchaMaxRetries = val
		}
	}

	welcomeMessage := getBoolEnv("WELCOME_MESSAGE", true)

	// Initialize enabled replacers
	enabledReplacers := make(map[string]bool)
	for _, replacer := range GetReplacers() {
		envKey := "REPLACER_" + strings.ReplaceAll(strings.ToUpper(replacer.Name), " ", "_") + "_ENABLED"
		enabledReplacers[replacer.Name] = getBoolEnv(envKey, true)
	}

	// Get available reactions for the group
	availableReactionsMap := make(map[int64][]string)

	escarbot := &EscarBot{
		Bot:                bot,
		Power:              true,
		LinkDetection:      linkDetection,
		ChannelForward:     channelForward,
		AdminForward:       adminForward,
		AutoBan:            autoBan,
		Captcha:            captcha,
		CaptchaTimeout:     captchaTimeout,
		CaptchaMaxRetries:  captchaMaxRetries,
		WelcomeMessage:     welcomeMessage,
		ChannelID:          channelIdInt,
		GroupID:            groupIdInt,
		AdminID:            adminIdInt,
		LogChannelID:       logChannelIdInt,
		BannedWords:        bannedWords,
		AvailableReactions: availableReactionsMap,
		MessageCache:       make(map[int64][]CachedMessage),
		ChatCache:          make(map[int64]ChatInfo),
		MaxCacheSize:       maxCacheSize,
		WelcomeText:        os.Getenv("WELCOME_TEXT"),
		WelcomeLinks:       os.Getenv("WELCOME_LINKS"),
		WelcomePhoto:       os.Getenv("WELCOME_PHOTO"),
		CaptchaText:        os.Getenv("CAPTCHA_TEXT"),
		EnabledReplacers:   enabledReplacers,
		PendingCaptchas:    make(map[int64]*PendingCaptcha),
		JoinProcessedCache: make(map[int64]*JoinProcessedEntry),
		VerifiedUsers:      make(map[int64]bool),
		StartupTime:        time.Now(),
	}

	availableReactionsMap[groupIdInt] = getAvailableReactions(escarbot, groupIdInt)

	return escarbot
}

// getBoolEnv reads a boolean environment variable with a default value
func getBoolEnv(key string, defaultValue bool) bool {
	value, err := strconv.ParseBool(os.Getenv(key))
	if err != nil {
		return defaultValue
	}
	return value
}

func BotPoll(escarbot *EscarBot) {
	if escarbot.Bot.Token == "" || escarbot.Bot.Self.UserName == "OfflineBot" {
		log.Println("Offline mode or invalid token, skipping BotPoll")
		return
	}
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	u.AllowedUpdates = []string{"message", "callback_query", "channel_post", "chat_member", "edited_message", "edited_channel_post", "message_reaction", "message_reaction_count"}

	bot := escarbot.Bot
	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		escarbot.StateMutex.RLock()
		linkDetection := escarbot.LinkDetection
		adminForward := escarbot.AdminForward
		channelForward := escarbot.ChannelForward
		escarbot.StateMutex.RUnlock()

		msg := update.Message
		if msg != nil { // If we got a message
			escarbot.StateMutex.RLock()
			captchaEnabled := escarbot.Captcha
			groupID := escarbot.GroupID
			escarbot.StateMutex.RUnlock()

			// If captcha is enabled, delete messages from users who haven't completed it yet
			if captchaEnabled && msg.Chat.ID == groupID && msg.From != nil && msg.NewChatMembers == nil && msg.LeftChatMember == nil {
				if isUserPendingCaptcha(escarbot, msg.From.ID) {
					deleteMessages(escarbot, msg.Chat.ID, msg.MessageID)
					continue
				}

				// Check for unverified users who may have bypassed join detection
				// (e.g. Telegram Premium users joining silently)
				if !msg.From.IsBot && !isUserVerified(escarbot, msg.From.ID) {
					if time.Since(escarbot.StartupTime) < startupGracePeriod {
						// During startup grace period, auto-verify existing members
						addVerifiedUser(escarbot, msg.From.ID)
					} else {
						// After grace period, check if user is an admin/creator
						memberConfig := tgbotapi.GetChatMemberConfig{
							ChatConfigWithUser: tgbotapi.ChatConfigWithUser{
								ChatConfig: tgbotapi.ChatConfig{
									ChatID: groupID,
								},
								UserID: msg.From.ID,
							},
						}
						member, err := escarbot.Bot.GetChatMember(memberConfig)
						if err == nil && (member.IsAdministrator() || member.IsCreator()) {
							addVerifiedUser(escarbot, msg.From.ID)
						} else {
							// Unverified non-admin user - trigger captcha flow
							log.Printf("Unverified user %d detected sending message, triggering captcha", msg.From.ID)
							deleteMessages(escarbot, msg.Chat.ID, msg.MessageID)
							processJoin(escarbot, msg.Chat.ID, *msg.From, 0)
							continue
						}
					}
				}
			}

			AddMessageToCache(escarbot, msg)

			handleNewChatMembers(escarbot, msg)

			if linkDetection {
				handleLinks(escarbot, msg)
			}
			if adminForward {
				forwardToAdmin(escarbot, msg)
			}
		}
		if update.CallbackQuery != nil {
			HandleCaptchaCallback(escarbot, update.CallbackQuery)
		}
		if update.ChatMember != nil {
			handleChatMemberUpdate(escarbot, update.ChatMember)
		}
		if update.ChannelPost != nil { // If we got a channel post
			AddMessageToCache(escarbot, update.ChannelPost)
			if channelForward {
				channelPostHandler(escarbot, update.ChannelPost)
			}
		}
		if update.EditedMessage != nil {
			UpdateMessageInCache(escarbot, update.EditedMessage)
		}
		if update.EditedChannelPost != nil {
			UpdateMessageInCache(escarbot, update.EditedChannelPost)
		}
		if update.MessageReactionCount != nil {
			updateReactionsInCache(escarbot, update.MessageReactionCount)
		}
		if update.MessageReaction != nil {
			updateIndividualReactionInCache(escarbot, update.MessageReaction)
		}
	}
}
