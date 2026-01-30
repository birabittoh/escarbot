package webui

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"text/template"

	"github.com/birabittoh/escarbot/telegram"
)

type WebUI struct {
	Server   *http.ServeMux
	EscarBot *telegram.EscarBot

	port string
}

var indexTemplate = template.Must(template.ParseFiles("index.html"))

func indexHandler(bot *telegram.EscarBot) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		buf := &bytes.Buffer{}
		err := indexTemplate.Execute(buf, bot)
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
		bot.LinkDetection = toggleBotProperty(w, r)
		UpdateBoolEnvVar("LINK_DETECTION", bot.LinkDetection)
	}
}

func channelForwardHandler(bot *telegram.EscarBot) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bot.ChannelForward = toggleBotProperty(w, r)
		UpdateBoolEnvVar("CHANNEL_FORWARD", bot.ChannelForward)
	}
}

func adminForwardHandler(bot *telegram.EscarBot) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bot.AdminForward = toggleBotProperty(w, r)
		UpdateBoolEnvVar("ADMIN_FORWARD", bot.AdminForward)
	}
}

func autoBanHandler(bot *telegram.EscarBot) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bot.AutoBan = toggleBotProperty(w, r)
		UpdateBoolEnvVar("AUTO_BAN", bot.AutoBan)
	}
}

func channelHandler(bot *telegram.EscarBot) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		res, err := getChatID(w, r)
		if err != nil {
			log.Println(err)
			return
		}
		bot.ChannelID = res
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
		bot.GroupID = res
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
		bot.AdminID = res
		UpdateEnvVar("ADMIN_ID", strconv.FormatInt(res, 10))
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
		bot.BannedWords = filteredWords

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

		// Get messages for the group chat
		messages := bot.MessageCache[bot.GroupID]

		response := []telegram.CachedMessage{}
		// Return last 10 messages in reverse order (newest first)
		start := max(len(messages)-10, 0)

		for i := len(messages) - 1; i >= start; i-- {
			msg := messages[i]

			// Truncate long messages
			if len(msg.Text) > 100 {
				msg.Text = msg.Text[:100] + "..."
			}

			response = append(response, msg)
		}

		json.NewEncoder(w).Encode(response)
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
	r.HandleFunc("/setChannel", channelHandler(bot))
	r.HandleFunc("/setGroup", groupHandler(bot))
	r.HandleFunc("/setAdmin", adminHandler(bot))
	r.HandleFunc("/setBannedWords", bannedWordsHandler(bot))
	r.HandleFunc("/api/messageCache", messageCacheHandler(bot))
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
