package webui

import (
	"bytes"
	"log"
	"net/http"
	"strconv"
	"text/template"

	"github.com/BiRabittoh/escarbot/telegram"
)

type WebUI struct {
	Server   *http.ServeMux
	EscarBot *telegram.EscarBot

	port string
}

var indexTemplate = template.Must(template.ParseFiles("index.html"))

const toggleFormName = "toggle"

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

func toggleBotProperty(w http.ResponseWriter, r *http.Request, bot *telegram.EscarBot) bool {
	r.ParseForm()
	res := r.Form.Get("toggle")
	http.Redirect(w, r, "/", http.StatusFound)
	return res == "on"
}

func getChatID(w http.ResponseWriter, r *http.Request, bot *telegram.EscarBot) (int64, error) {
	r.ParseForm()
	res := r.Form.Get("id")
	http.Redirect(w, r, "/", http.StatusFound)
	return strconv.ParseInt(res, 10, 64)
}

func linksHandler(bot *telegram.EscarBot) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bot.LinkDetection = toggleBotProperty(w, r, bot)
	}
}

func channelForwardHandler(bot *telegram.EscarBot) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bot.ChannelForward = toggleBotProperty(w, r, bot)
	}
}

func adminForwardHandler(bot *telegram.EscarBot) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bot.AdminForward = toggleBotProperty(w, r, bot)
	}
}

func channelHandler(bot *telegram.EscarBot) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		res, err := getChatID(w, r, bot)
		if err != nil {
			log.Println(err)
			return
		}
		bot.ChannelID = res
	}
}

func groupHandler(bot *telegram.EscarBot) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		res, err := getChatID(w, r, bot)
		if err != nil {
			log.Println(err)
			return
		}
		bot.GroupID = res
	}
}

func adminHandler(bot *telegram.EscarBot) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		res, err := getChatID(w, r, bot)
		if err != nil {
			log.Println(err)
			return
		}
		bot.AdminID = res
	}
}

func NewWebUI(port string, bot *telegram.EscarBot) WebUI {

	go telegram.BotPoll(bot)

	r := http.NewServeMux()
	r.HandleFunc("/", indexHandler(bot))
	r.HandleFunc("/setLinks", linksHandler(bot))
	r.HandleFunc("/setChannelForward", channelForwardHandler(bot))
	r.HandleFunc("/setAdminForward", adminForwardHandler(bot))
	r.HandleFunc("/setChannel", channelHandler(bot))
	r.HandleFunc("/setGroup", groupHandler(bot))
	r.HandleFunc("/setAdmin", adminHandler(bot))

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
