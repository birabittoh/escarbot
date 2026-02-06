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
			if got := parseText(tt.text, entities); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseText() = %v, want %v", got, tt.want)
			}
		})
	}
}
