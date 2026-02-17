package webui

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"text/template"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/birabittoh/escarbot/telegram"
)

type WebUI struct {
	Server   *http.ServeMux
	EscarBot *telegram.EscarBot

	port string
}

var indexTemplate = template.Must(template.New("index.html").Funcs(template.FuncMap{
	"toJSON": func(v interface{}) (string, error) {
		bytes, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return string(bytes), nil
	},
}).ParseFiles("index.html"))

func indexHandler(bot *telegram.EscarBot) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bot.StateMutex.RLock()
		defer bot.StateMutex.RUnlock()

		data := struct {
			*telegram.EscarBot
			AllReplacers []telegram.Replacer
		}{
			bot,
			telegram.GetReplacers(),
		}
		buf := &bytes.Buffer{}
		err := indexTemplate.Execute(buf, data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		buf.WriteTo(w)
	}
}

func toggleBotProperty(w http.ResponseWriter, r *http.Request) bool {
	r.ParseForm()
	res := r.Form.Get("toggle")
	return res == "on"
}

func getChatID(w http.ResponseWriter, r *http.Request) (int64, error) {
	r.ParseForm()
	res := r.Form.Get("id")
	return strconv.ParseInt(res, 10, 64)
}

func linksHandler(bot *telegram.EscarBot) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bot.StateMutex.Lock()
		defer bot.StateMutex.Unlock()
		bot.LinkDetection = toggleBotProperty(w, r)
		UpdateBoolEnvVar("LINK_DETECTION", bot.LinkDetection)
	}
}

func channelForwardHandler(bot *telegram.EscarBot) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bot.StateMutex.Lock()
		defer bot.StateMutex.Unlock()
		bot.ChannelForward = toggleBotProperty(w, r)
		UpdateBoolEnvVar("CHANNEL_FORWARD", bot.ChannelForward)
	}
}

func adminForwardHandler(bot *telegram.EscarBot) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bot.StateMutex.Lock()
		defer bot.StateMutex.Unlock()
		bot.AdminForward = toggleBotProperty(w, r)
		UpdateBoolEnvVar("ADMIN_FORWARD", bot.AdminForward)
	}
}

func autoBanHandler(bot *telegram.EscarBot) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bot.StateMutex.Lock()
		defer bot.StateMutex.Unlock()
		bot.AutoBan = toggleBotProperty(w, r)
		UpdateBoolEnvVar("AUTO_BAN", bot.AutoBan)
	}
}

func captchaHandler(bot *telegram.EscarBot) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bot.StateMutex.Lock()
		defer bot.StateMutex.Unlock()
		bot.Captcha = toggleBotProperty(w, r)
		UpdateBoolEnvVar("CAPTCHA", bot.Captcha)
	}
}

func captchaConfigHandler(bot *telegram.EscarBot) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()

		timeoutStr := r.Form.Get("timeout")
		maxRetriesStr := r.Form.Get("maxRetries")
		captchaText := r.Form.Get("captchaText")

		bot.StateMutex.Lock()
		if val, err := strconv.Atoi(timeoutStr); err == nil {
			bot.CaptchaTimeout = val
			UpdateEnvVar("CAPTCHA_TIMEOUT", timeoutStr)
		}
		if val, err := strconv.Atoi(maxRetriesStr); err == nil {
			bot.CaptchaMaxRetries = val
			UpdateEnvVar("CAPTCHA_MAX_RETRIES", maxRetriesStr)
		}
		bot.CaptchaText = captchaText
		UpdateEnvVar("CAPTCHA_TEXT", captchaText)
		bot.StateMutex.Unlock()
	}
}

func welcomeMessageHandler(bot *telegram.EscarBot) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bot.StateMutex.Lock()
		defer bot.StateMutex.Unlock()
		bot.WelcomeMessage = toggleBotProperty(w, r)
		UpdateBoolEnvVar("WELCOME_MESSAGE", bot.WelcomeMessage)
	}
}

func welcomeContentHandler(bot *telegram.EscarBot) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()

		welcomeText := r.Form.Get("welcomeText")
		welcomePhoto := r.Form.Get("welcomePhoto")
		welcomeLinks := r.Form.Get("welcomeLinks")

		bot.StateMutex.Lock()
		bot.WelcomeText = welcomeText
		bot.WelcomePhoto = welcomePhoto
		bot.WelcomeLinks = welcomeLinks
		bot.StateMutex.Unlock()

		UpdateEnvVar("WELCOME_TEXT", welcomeText)
		UpdateEnvVar("WELCOME_PHOTO", welcomePhoto)
		UpdateEnvVar("WELCOME_LINKS", welcomeLinks)

		log.Printf("Welcome content updated: text=%d chars, photo=%s, links=%d chars",
			len(welcomeText), welcomePhoto, len(welcomeLinks))
	}
}

func channelHandler(bot *telegram.EscarBot) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		res, err := getChatID(w, r)
		if err != nil {
			log.Println(err)
			return
		}
		bot.StateMutex.Lock()
		bot.ChannelID = res
		bot.StateMutex.Unlock()
		UpdateEnvVar("CHANNEL_ID", strconv.FormatInt(res, 10))
	}
}

func groupHandler(bot *telegram.EscarBot) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		res, err := getChatID(w, r)
		if err != nil {
			log.Println(err)
			return
		}
		bot.StateMutex.Lock()
		bot.GroupID = res
		bot.StateMutex.Unlock()
		UpdateEnvVar("GROUP_ID", strconv.FormatInt(res, 10))
	}
}

func adminHandler(bot *telegram.EscarBot) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		res, err := getChatID(w, r)
		if err != nil {
			log.Println(err)
			return
		}
		bot.StateMutex.Lock()
		bot.AdminID = res
		bot.StateMutex.Unlock()
		UpdateEnvVar("ADMIN_ID", strconv.FormatInt(res, 10))
	}
}

func replacerHandler(bot *telegram.EscarBot) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		name := r.Form.Get("name")
		enabled := r.Form.Get("toggle") == "on"

		if name == "" {
			http.Error(w, "Missing name", http.StatusBadRequest)
			return
		}

		bot.StateMutex.Lock()
		bot.EnabledReplacers[name] = enabled
		bot.StateMutex.Unlock()
		envKey := "REPLACER_" + strings.ReplaceAll(strings.ToUpper(name), " ", "_") + "_ENABLED"
		UpdateBoolEnvVar(envKey, enabled)
	}
}

func bannedWordsHandler(bot *telegram.EscarBot) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()

		// Get all the words from the form
		words := r.Form["word"]

		// Filter out empty strings
		var filteredWords []string
		for _, word := range words {
			trimmed := strings.TrimSpace(word)
			if trimmed != "" {
				filteredWords = append(filteredWords, trimmed)
			}
		}

		// Update bot's banned words
		bot.StateMutex.Lock()
		bot.BannedWords = filteredWords
		bot.StateMutex.Unlock()

		// Persist to .env
		wordsStr := strings.Join(filteredWords, ",")
		UpdateEnvVar("BANNED_WORDS", wordsStr)

		log.Printf("Banned words updated: %v", filteredWords)

		http.Redirect(w, r, "/", http.StatusFound)
	}
}

func messageCacheHandler(bot *telegram.EscarBot) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		bot.CacheMutex.Lock()
		defer bot.CacheMutex.Unlock()

		// Messages are already ordered (newest first), just truncate long texts
		response := make([]telegram.CachedMessage, len(bot.MessageCache))
		for i, msg := range bot.MessageCache {
			// Truncate long messages
			if len(msg.Text) > 100 {
				msg.Text = msg.Text[:100] + "..."
			}
			response[i] = msg
		}

		log.Printf("Message cache request: returning %d total messages", len(response))
		json.NewEncoder(w).Encode(response)
	}
}

func setReactionHandler(bot *telegram.EscarBot) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()

		chatID, err := strconv.ParseInt(r.Form.Get("chat_id"), 10, 64)
		if err != nil {
			http.Error(w, "Invalid chat_id", http.StatusBadRequest)
			return
		}

		messageID, err := strconv.Atoi(r.Form.Get("message_id"))
		if err != nil {
			http.Error(w, "Invalid message_id", http.StatusBadRequest)
			return
		}

		emoji := r.Form.Get("emoji")
		if emoji == "" {
			http.Error(w, "Missing emoji", http.StatusBadRequest)
			return
		}

		// Create reaction
		reaction := tgbotapi.ReactionType{
			Type:  "emoji",
			Emoji: emoji,
		}

		reactionConfig := tgbotapi.NewSetMessageReaction(chatID, messageID, []tgbotapi.ReactionType{reaction}, false)
		_, err = bot.Bot.Request(reactionConfig)
		if err != nil {
			log.Printf("Error setting reaction: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

func NewWebUI(port string, bot *telegram.EscarBot) WebUI {
	// Initialize WebSocket hub
	InitMessageHub()

	// Register callback for broadcasting new messages
	bot.OnMessageCached = func(msg telegram.CachedMessage) {
		BroadcastMessage(msg)
	}

	go telegram.BotPoll(bot)

	r := http.NewServeMux()
	r.HandleFunc("/", indexHandler(bot))
	r.HandleFunc("/setLinks", linksHandler(bot))
	r.HandleFunc("/setChannelForward", channelForwardHandler(bot))
	r.HandleFunc("/setAdminForward", adminForwardHandler(bot))
	r.HandleFunc("/setAutoBan", autoBanHandler(bot))
	r.HandleFunc("/setCaptcha", captchaHandler(bot))
	r.HandleFunc("/setCaptchaConfig", captchaConfigHandler(bot))
	r.HandleFunc("/setWelcomeMessage", welcomeMessageHandler(bot))
	r.HandleFunc("/setWelcomeContent", welcomeContentHandler(bot))
	r.HandleFunc("/setChannel", channelHandler(bot))
	r.HandleFunc("/setGroup", groupHandler(bot))
	r.HandleFunc("/setAdmin", adminHandler(bot))
	r.HandleFunc("/setReplacer", replacerHandler(bot))
	r.HandleFunc("/setBannedWords", bannedWordsHandler(bot))
	r.HandleFunc("/api/messageCache", messageCacheHandler(bot))
	r.HandleFunc("/setReaction", setReactionHandler(bot))
	r.HandleFunc("/ws", wsHandler)

	return WebUI{
		Server:   r,
		EscarBot: bot,
		port:     port,
	}
}

func (webui *WebUI) Poll() {
	log.Println("Serving on port", webui.port)
	err := http.ListenAndServe(":"+webui.port, webui.Server)
	if err != nil {
		log.Fatal(err)
	}
}
