package webui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/birabittoh/escarbot/telegram"
)

func TestChatsHandlerFiltering(t *testing.T) {
	bot := &telegram.EscarBot{
		Cache:         telegram.NewCache(""), // in-memory
		ChatBlacklist: []int64{123},
	}

	// Add some chats to cache
	bot.Cache.SetChatInfo(123, telegram.ChatInfo{ID: 123, Title: "Blacklisted Chat"})
	bot.Cache.SetChatInfo(456, telegram.ChatInfo{ID: 456, Title: "Allowed Chat"})

	req, err := http.NewRequest("GET", "/api/chats", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := chatsHandler(bot)

	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	var chats []telegram.ChatInfo
	if err := json.Unmarshal(rr.Body.Bytes(), &chats); err != nil {
		t.Fatal(err)
	}

	if len(chats) != 1 {
		t.Errorf("expected 1 chat, got %d", len(chats))
	}

	if chats[0].ID != 456 {
		t.Errorf("expected chat 456, got %d", chats[0].ID)
	}
}
