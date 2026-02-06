package telegram

import (
	"reflect"
	"strings"
	"testing"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"
)

func TestParseText(t *testing.T) {
	tests := []struct {
		name string
		text string
		url  string
		want []string
	}{
		{
			name: "Twitter link",
			text: "Check this out: https://twitter.com/jack/status/20",
			url:  "https://twitter.com/jack/status/20",
			want: []string{"https://fxtwitter.com/jack/status/20"},
		},
		{
			name: "Reddit r/ link",
			text: "Reddit post: https://www.reddit.com/r/shittymoviedetails/comments/160onpq",
			url:  "https://www.reddit.com/r/shittymoviedetails/comments/160onpq",
			want: []string{"https://rxddit.com/r/shittymoviedetails/comments/160onpq"},
		},
		{
			name: "Reddit old link",
			text: "Old Reddit: https://old.reddit.com/r/shittymoviedetails/comments/160onpq",
			url:  "https://old.reddit.com/r/shittymoviedetails/comments/160onpq",
			want: []string{"https://rxddit.com/r/shittymoviedetails/comments/160onpq"},
		},
		{
			name: "Reddit profile u/ link",
			text: "User profile: https://www.reddit.com/u/someuser",
			url:  "https://www.reddit.com/u/someuser",
			want: []string{"https://rxddit.com/u/someuser"},
		},
		{
			name: "Reddit profile user/ link",
			text: "User profile: https://www.reddit.com/user/someuser",
			url:  "https://www.reddit.com/user/someuser",
			want: []string{"https://rxddit.com/user/someuser"},
		},
		{
			name: "Bluesky post link",
			text: "Bluesky post: https://bsky.app/profile/coldunwanted.net/post/3lc5qjx2n2c27",
			url:  "https://bsky.app/profile/coldunwanted.net/post/3lc5qjx2n2c27",
			want: []string{"https://xbsky.app/profile/coldunwanted.net/post/3lc5qjx2n2c27"},
		},
		{
			name: "Bluesky profile link",
			text: "Bluesky profile: https://bsky.app/profile/coldunwanted.net",
			url:  "https://bsky.app/profile/coldunwanted.net",
			want: []string{"https://xbsky.app/profile/coldunwanted.net"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			offset := strings.Index(tt.text, tt.url)
			if offset == -1 {
				t.Fatalf("URL not found in text")
			}
			entities := []tgbotapi.MessageEntity{
				{Type: "url", Offset: offset, Length: len(tt.url)},
			}
			bot := &EscarBot{
				EnabledReplacers: make(map[string]bool),
			}
			if got := parseText(bot, tt.text, entities); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseText() = %v, want %v", got, tt.want)
			}
		})
	}
}
