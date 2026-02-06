package telegram

import (
	"fmt"
	"regexp"
	"strings"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"
)

type Replacer struct {
	Name   string
	Regex  *regexp.Regexp
	Format string
}

const (
	parseMode   = "markdown"
	linkMessage = "[🔗](%s) Da %s."
	regexFlags  = "(?i)(?m)"
)

var replacers = []Replacer{
	{
		Name:   "Twitter",
		Regex:  regexp.MustCompile(regexFlags + `https?:\/\/(?:www\.)?twitter\.com\/(?:#!\/)?(.*)\/status(?:es)?\/([^\/\?\s]+)`),
		Format: "https://fxtwitter.com/%s/status/%s",
	},
	{
		Name:   "X",
		Regex:  regexp.MustCompile(regexFlags + `https?:\/\/(?:www\.)?x\.com\/(?:#!\/)?(.*)\/status(?:es)?\/([^\/\?\s]+)`),
		Format: "https://fixupx.com/%s/status/%s",
	},
	{
		Name:   "Bluesky",
		Regex:  regexp.MustCompile(regexFlags + `https?:\/\/(?:www\.)?bsky\.app\/(profile\/[^\?\s]+)`),
		Format: "https://xbsky.app/%s",
	},
	{
		Name:   "Instagram",
		Regex:  regexp.MustCompile(regexFlags + `https?:\/\/(?:www\.)?instagram\.com\/(?:reels?|p)\/([\w\-]{11})[\/\?\w=&]*`),
		Format: "https://kkinstagram.com/p/%s",
	},
	{
		Name:   "TikTok",
		Regex:  regexp.MustCompile(regexFlags + `https?:\/\/(?:(?:www)|(?:vm))?\.?tiktok\.com\/@([\w.]+)\/(?:video)\/(\d{19,})`),
		Format: "https://www.kktiktok.com/@%s/video/%s",
	},
	{
		Name:   "TikTok Short",
		Regex:  regexp.MustCompile(regexFlags + `https?:\/\/(?:(?:www)|(?:vm))?\.?tiktok\.com\/(?:t\/)?([\w]{9})\/?`),
		Format: "https://vm.kktiktok.com/%s/",
	},
	{
		Name:   "Reddit",
		Regex:  regexp.MustCompile(regexFlags + `https?:\/\/(?:(?:www|old)\.)?reddit\.com\/((?:r|u|user)\/[^\?\s]+)`),
		Format: "https://rxddit.com/%s",
	},
}

func GetReplacers() []Replacer {
	return replacers
}

func isInSpoiler(entities []tgbotapi.MessageEntity, offset, length int) bool {
	for _, s := range entities {
		if s.Type != "spoiler" {
			continue
		}

		if offset < s.Offset+s.Length && offset+length > s.Offset {
			return true
		}
	}
	return false
}

func parseText(escarbot *EscarBot, text string, entities []tgbotapi.MessageEntity) (links []string) {
	var rawLinks string
	runes := []rune(text) // Convert to runes to handle emojis

	for _, e := range entities {
		if e.Type == "text_link" {
			if isInSpoiler(entities, e.Offset, len(e.URL)) {
				continue
			}
			rawLinks += e.URL + "\n"
		} else if e.Type == "url" {
			if isInSpoiler(entities, e.Offset, e.Length) {
				continue
			}

			rawLinks += string(runes[e.Offset:e.Offset+e.Length]) + "\n"
		}
	}

	escarbot.StateMutex.RLock()
	enabledReplacers := make(map[string]bool)
	for k, v := range escarbot.EnabledReplacers {
		enabledReplacers[k] = v
	}
	escarbot.StateMutex.RUnlock()

	for _, replacer := range replacers {
		// Check if this replacer is enabled
		if enabled, exists := enabledReplacers[replacer.Name]; exists && !enabled {
			continue
		}

		foundMatches := replacer.Regex.FindStringSubmatch(rawLinks)
		if len(foundMatches) == 0 {
			continue
		}
		captureGroups := foundMatches[1:]

		var formatArgs []interface{}
		for _, match := range captureGroups {
			if match != "" {
				formatArgs = append(formatArgs, match)
			}
		}

		formatted := fmt.Sprintf(replacer.Format, formatArgs...)
		links = append(links, formatted)
	}
	return links
}

func getUserMention(user tgbotapi.User) string {
	var name string
	if user.UserName == "" {
		name = strings.TrimSpace(user.FirstName + " " + user.LastName)
	} else {
		name = "@" + user.UserName
	}
	return fmt.Sprintf("[%s](tg://user?id=%d)", name, user.ID)
}

func handleLinks(escarbot *EscarBot, message *tgbotapi.Message) {
	links := []string{}

	if len(message.Entities) > 0 {
		textLinks := parseText(escarbot, message.Text, message.Entities)
		links = append(links, textLinks...)
	}

	if len(message.CaptionEntities) > 0 {
		captionLinks := parseText(escarbot, message.Caption, message.CaptionEntities)
		links = append(links, captionLinks...)
	}

	if len(links) == 0 {
		return
	}

	user := getUserMention(*message.From)
	bot := escarbot.Bot

	for _, link := range links {
		text := fmt.Sprintf(linkMessage, link, user)
		msg := tgbotapi.NewMessage(message.Chat.ID, text)
		msg.MessageThreadID = message.MessageThreadID
		msg.ParseMode = parseMode
		bot.Send(msg)
	}

}
