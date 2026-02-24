package webui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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

func toggleBotProperty(r *http.Request) bool {
	r.ParseForm()
	res := r.Form.Get("toggle")
	return res == "on"
}

func getChatID(r *http.Request) (int64, error) {
	r.ParseForm()
	res := r.Form.Get("id")
	return strconv.ParseInt(res, 10, 64)
}

func linksHandler(bot *telegram.EscarBot) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bot.StateMutex.Lock()
		defer bot.StateMutex.Unlock()
		bot.LinkDetection = toggleBotProperty(r)
		UpdateBoolEnvVar("LINK_DETECTION", bot.LinkDetection)
	}
}

func channelForwardHandler(bot *telegram.EscarBot) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bot.StateMutex.Lock()
		defer bot.StateMutex.Unlock()
		bot.ChannelForward = toggleBotProperty(r)
		UpdateBoolEnvVar("CHANNEL_FORWARD", bot.ChannelForward)
	}
}

func adminForwardHandler(bot *telegram.EscarBot) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bot.StateMutex.Lock()
		defer bot.StateMutex.Unlock()
		bot.AdminForward = toggleBotProperty(r)
		UpdateBoolEnvVar("ADMIN_FORWARD", bot.AdminForward)
	}
}

func autoBanHandler(bot *telegram.EscarBot) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bot.StateMutex.Lock()
		defer bot.StateMutex.Unlock()
		bot.AutoBan = toggleBotProperty(r)
		UpdateBoolEnvVar("AUTO_BAN", bot.AutoBan)
	}
}

func captchaHandler(bot *telegram.EscarBot) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bot.StateMutex.Lock()
		defer bot.StateMutex.Unlock()
		bot.Captcha = toggleBotProperty(r)
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
		bot.WelcomeMessage = toggleBotProperty(r)
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
		res, err := getChatID(r)
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
		res, err := getChatID(r)
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
		res, err := getChatID(r)
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

		words := r.Form["word"]

		var filteredWords []string
		for _, word := range words {
			trimmed := strings.TrimSpace(word)
			if trimmed != "" {
				filteredWords = append(filteredWords, trimmed)
			}
		}

		bot.StateMutex.Lock()
		bot.BannedWords = filteredWords
		bot.StateMutex.Unlock()

		wordsStr := strings.Join(filteredWords, ",")
		UpdateEnvVar("BANNED_WORDS", wordsStr)

		log.Printf("Banned words updated: %v", filteredWords)

		http.Redirect(w, r, "/", http.StatusFound)
	}
}

func chatsHandler(bot *telegram.EscarBot) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		chats := bot.Cache.GetAllChats()

		// Filter blacklisted chats
		bot.StateMutex.RLock()
		blacklist := bot.ChatBlacklist
		bot.StateMutex.RUnlock()

		filteredChats := make([]telegram.ChatInfo, 0)
		for _, chat := range chats {
			isBlacklisted := false
			for _, blacklistedID := range blacklist {
				if chat.ID == blacklistedID {
					isBlacklisted = true
					break
				}
			}
			if !isBlacklisted {
				filteredChats = append(filteredChats, chat)
			}
		}

		jsonData, _ := json.Marshal(filteredChats)
		log.Printf("Chats request: returning %d chats (filtered from %d).", len(filteredChats), len(chats))
		w.Write(jsonData)
	}
}

func messageCacheHandler(bot *telegram.EscarBot) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		allMessages := bot.Cache.GetAllMessages()
		chatIDs := make([]int64, 0, len(allMessages))
		for id := range allMessages {
			chatIDs = append(chatIDs, id)
		}

		jsonData, _ := json.Marshal(allMessages)
		log.Printf("Message cache request from %s: returning messages for %d chats: %v. JSON length: %d", r.RemoteAddr, len(allMessages), chatIDs, len(jsonData))
		w.Write(jsonData)
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

		reactions := []tgbotapi.ReactionType{}
		if emoji != "" {
			reactions = append(reactions, tgbotapi.ReactionType{
				Type:  "emoji",
				Emoji: emoji,
			})
		}

		reactionConfig := tgbotapi.NewSetMessageReaction(chatID, messageID, reactions, false)
		_, err = bot.Bot.Request(reactionConfig)
		if err != nil {
			log.Printf("Error setting reaction: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		msg, ok := bot.Cache.SetBotReaction(chatID, messageID, emoji)
		if ok && bot.OnMessageCached != nil {
			bot.OnMessageCached(msg)
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

func mediaHandler(bot *telegram.EscarBot) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fileID := r.URL.Query().Get("file_id")
		if fileID == "" {
			http.Error(w, "Missing file_id", http.StatusBadRequest)
			return
		}

		file, err := bot.Bot.GetFile(tgbotapi.FileConfig{FileID: fileID})
		if err != nil {
			log.Printf("Error getting file info: %v", err)
			http.Error(w, "Error getting file info", http.StatusInternalServerError)
			return
		}

		directURL, err := bot.Bot.GetFileDirectURL(file.FileID)
		if err != nil {
			log.Printf("Error getting direct URL: %v", err)
			http.Error(w, "Error getting direct URL", http.StatusInternalServerError)
			return
		}

		resp, err := http.Get(directURL)
		if err != nil {
			log.Printf("Error fetching media: %v", err)
			http.Error(w, "Error fetching media", http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			http.Error(w, fmt.Sprintf("Error fetching media from Telegram: status %d", resp.StatusCode), resp.StatusCode)
			return
		}

		w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
		w.Header().Set("Cache-Control", "public, max-age=3600")

		io.Copy(w, resp.Body)
	}
}

func NewWebUI(port string, bot *telegram.EscarBot) WebUI {
	InitMessageHub()

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
	r.HandleFunc("/api/chats", chatsHandler(bot))
	r.HandleFunc("/api/messageCache", messageCacheHandler(bot))
	r.HandleFunc("/api/media", mediaHandler(bot))
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
