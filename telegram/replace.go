package telegram

import (
	"fmt"
	"regexp"
	"strings"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"
)

type Replacer struct {
	Regex  *regexp.Regexp
	Format string
}

const (
	parseMode   = "markdown"
	linkMessage = "[🔗](%s) Da %s."
	regexFlags  = "(?i)(?m)"
)

var replacers = []Replacer{
	/*
		{
			Regex:  regexp.MustCompile(regexFlags + `(?:(?:https?:)?\/\/)?(?:(?:www|m)\.)?(?:(?:youtube(?:-nocookie)?\.com|youtu.be))(?:\/(?:[\w\-]+\?v=|embed\/|live\/|v\/|shorts\/)?)([\w\-]+)`),
			Format: "https://y.outube.duckdns.org/%s",
		},
	*/
	{
		Regex:  regexp.MustCompile(regexFlags + `https?:\/\/(?:www\.)?twitter\.com\/(?:#!\/)?(.*)\/status(?:es)?\/([^\/\?\s]+)`),
		Format: "https://fxtwitter.com/%s/status/%s",
	},
	{
		Regex:  regexp.MustCompile(regexFlags + `https?:\/\/(?:www\.)?x\.com\/(?:#!\/)?(.*)\/status(?:es)?\/([^\/\?\s]+)`),
		Format: "https://fixupx.com/%s/status/%s",
	},
	{
		Regex:  regexp.MustCompile(regexFlags + `https?:\/\/(?:www\.)?bsky\.app\/profile\/([^\/\s]+)\/post\/([^\/\s]+)`),
		Format: "https://fxbsky.app/profile/%s/post/%s",
	},
	{
		Regex:  regexp.MustCompile(regexFlags + `https?:\/\/(?:www\.)?instagram\.com\/(?:reels?|p)\/([\w\-]{11})[\/\?\w=&]*`),
		Format: "https://kkinstagram.com/p/%s",
	},
	{
		Regex:  regexp.MustCompile(regexFlags + `https?:\/\/(?:(?:www)|(?:vm))?\.?tiktok\.com\/@([\w.]+)\/(?:video)\/(\d{19,})`),
		Format: "https://www.vxtiktok.com/@%s/video/%s",
	},
	{
		Regex:  regexp.MustCompile(regexFlags + `https?:\/\/(?:(?:www)|(?:vm))?\.?tiktok\.com\/(?:t\/)?([\w]{9})\/?`),
		Format: "https://vm.vxtiktok.com/%s/",
	},
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

func parseText(text string, entities []tgbotapi.MessageEntity) (links []string) {
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

	for _, replacer := range replacers {
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

func handleLinks(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	links := []string{}

	if len(message.Entities) > 0 {
		textLinks := parseText(message.Text, message.Entities)
		links = append(links, textLinks...)
	}

	if len(message.CaptionEntities) > 0 {
		captionLinks := parseText(message.Caption, message.CaptionEntities)
		links = append(links, captionLinks...)
	}

	if len(links) == 0 {
		return
	}

	user := getUserMention(*message.From)

	for _, link := range links {
		text := fmt.Sprintf(linkMessage, link, user)
		msg := tgbotapi.NewMessage(message.Chat.ID, text)
		msg.MessageThreadID = message.MessageThreadID
		msg.ParseMode = parseMode
		bot.Send(msg)
	}

}
