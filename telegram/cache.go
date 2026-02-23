package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/redis/go-redis/v9"
)

const (
	keyPrefixMessages  = "escarbot:messages:"
	keyPrefixChat      = "escarbot:chat:"
	keyChatIDs         = "escarbot:chat_ids"
	keyPrefixReactions = "escarbot:reactions:"
	keyPrefixCaptcha   = "escarbot:captcha:"
	keyPrefixJoin      = "escarbot:join:"
	joinTTL            = time.Minute
)

// pendingCaptchaRecord is the serialisable part of PendingCaptcha (no timer).
type pendingCaptchaRecord struct {
	UserID        int64  `json:"user_id"`
	ChatID        int64  `json:"chat_id"`
	CorrectAnswer string `json:"correct_answer"`
	CaptchaMsgID  int    `json:"captcha_msg_id"`
	JoinMsgID     int    `json:"join_msg_id"`
	Attempts      int    `json:"attempts"`
}

// Cache handles all bot caching, backed by Valkey/Redis when an address is
// provided, or falling back to in-memory storage when addr is empty.
type Cache struct {
	client *redis.Client
	ctx    context.Context

	// In-memory storage (used when client is nil).
	mu        sync.RWMutex
	messages  map[int64][]CachedMessage
	chats     map[int64]ChatInfo
	reactions map[int64][]string
	captchas  map[int64]*pendingCaptchaRecord
	joins     map[int64]*JoinProcessedEntry

	// Timers are always kept in-memory regardless of backend.
	timerMu sync.Mutex
	timers  map[int64]*time.Timer
}

// NewCache creates a Cache backed by Valkey at addr, or in-memory if addr is
// empty.
func NewCache(addr string) *Cache {
	c := &Cache{
		ctx:       context.Background(),
		messages:  make(map[int64][]CachedMessage),
		chats:     make(map[int64]ChatInfo),
		reactions: make(map[int64][]string),
		captchas:  make(map[int64]*pendingCaptchaRecord),
		joins:     make(map[int64]*JoinProcessedEntry),
		timers:    make(map[int64]*time.Timer),
	}
	if addr != "" {
		c.client = redis.NewClient(&redis.Options{Addr: addr})
		if err := c.client.Ping(c.ctx).Err(); err != nil {
			log.Printf("Warning: failed to connect to Valkey at %s: %v (falling back to in-memory)", addr, err)
			c.client = nil
		} else {
			log.Printf("Connected to Valkey at %s", addr)
		}
	}
	return c
}

// ── Message cache ─────────────────────────────────────────────────────────────

// GetMessages returns all cached messages for the given chat.
func (c *Cache) GetMessages(chatID int64) []CachedMessage {
	if c.client != nil {
		key := fmt.Sprintf("%s%d", keyPrefixMessages, chatID)
		val, err := c.client.Get(c.ctx, key).Result()
		if err == redis.Nil {
			return nil
		} else if err != nil {
			log.Printf("Cache: get messages chat %d: %v", chatID, err)
			return nil
		}
		var msgs []CachedMessage
		if err := json.Unmarshal([]byte(val), &msgs); err != nil {
			log.Printf("Cache: unmarshal messages chat %d: %v", chatID, err)
			return nil
		}
		return msgs
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	src := c.messages[chatID]
	out := make([]CachedMessage, len(src))
	copy(out, src)
	return out
}

// setMessages writes the message slice for a chat to the backend.
func (c *Cache) setMessages(chatID int64, msgs []CachedMessage) {
	if c.client != nil {
		key := fmt.Sprintf("%s%d", keyPrefixMessages, chatID)
		data, err := json.Marshal(msgs)
		if err != nil {
			log.Printf("Cache: marshal messages chat %d: %v", chatID, err)
			return
		}
		if err := c.client.Set(c.ctx, key, data, 0).Err(); err != nil {
			log.Printf("Cache: set messages chat %d: %v", chatID, err)
		}
		return
	}
	c.mu.Lock()
	c.messages[chatID] = msgs
	c.mu.Unlock()
}

// AddMessage prepends msg to the chat's message list (capped at maxSize).
// Returns true if the message was actually added (not a duplicate).
func (c *Cache) AddMessage(chatID int64, msg CachedMessage, maxSize int) bool {
	if c.client != nil {
		msgs := c.GetMessages(chatID)
		for _, m := range msgs {
			if m.MessageID == msg.MessageID {
				return false
			}
		}
		msgs = append([]CachedMessage{msg}, msgs...)
		if len(msgs) > maxSize {
			msgs = msgs[:maxSize]
		}
		c.setMessages(chatID, msgs)
		return true
	}
	// Hold a single lock for the whole operation in in-memory mode.
	c.mu.Lock()
	defer c.mu.Unlock()
	msgs := c.messages[chatID]
	for _, m := range msgs {
		if m.MessageID == msg.MessageID {
			return false
		}
	}
	msgs = append([]CachedMessage{msg}, msgs...)
	if len(msgs) > maxSize {
		msgs = msgs[:maxSize]
	}
	c.messages[chatID] = msgs
	return true
}

// UpdateMessage edits an existing cached message's text/caption and saves a
// history entry. Returns the updated message and true when found.
func (c *Cache) UpdateMessage(chatID int64, updated *tgbotapi.Message) (CachedMessage, bool) {
	if c.client != nil {
		msgs := c.GetMessages(chatID)
		for i, m := range msgs {
			if m.MessageID != updated.MessageID {
				continue
			}
			if m.Text == updated.Text && m.Caption == updated.Caption {
				return m, false
			}
			msgs[i].History = append(msgs[i].History, MessageHistory{
				Text:     m.Text,
				Caption:  m.Caption,
				EditDate: updated.EditDate,
			})
			msgs[i].Text = updated.Text
			msgs[i].Caption = updated.Caption
			msgs[i].Entities = updated.Entities
			c.setMessages(chatID, msgs)
			return msgs[i], true
		}
		return CachedMessage{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	msgs := c.messages[chatID]
	for i, m := range msgs {
		if m.MessageID != updated.MessageID {
			continue
		}
		if m.Text == updated.Text && m.Caption == updated.Caption {
			return m, false
		}
		msgs[i].History = append(msgs[i].History, MessageHistory{
			Text:     m.Text,
			Caption:  m.Caption,
			EditDate: updated.EditDate,
		})
		msgs[i].Text = updated.Text
		msgs[i].Caption = updated.Caption
		msgs[i].Entities = updated.Entities
		return msgs[i], true
	}
	return CachedMessage{}, false
}

// UpdateReactions updates the aggregate reaction counts for a cached message.
func (c *Cache) UpdateReactions(chatID int64, msgID int, reactions []tgbotapi.ReactionCount) (CachedMessage, bool) {
	if c.client != nil {
		msgs := c.GetMessages(chatID)
		for i, m := range msgs {
			if m.MessageID == msgID {
				msgs[i].Reactions = reactions
				c.setMessages(chatID, msgs)
				return msgs[i], true
			}
		}
		return CachedMessage{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	msgs := c.messages[chatID]
	for i, m := range msgs {
		if m.MessageID == msgID {
			msgs[i].Reactions = reactions
			return msgs[i], true
		}
	}
	return CachedMessage{}, false
}

// UpdateIndividualReaction updates a single user's emoji reaction on a message.
func (c *Cache) UpdateIndividualReaction(chatID int64, msgID int, userName string, newReactions []tgbotapi.ReactionType) (CachedMessage, bool) {
	update := func(msgs []CachedMessage) ([]CachedMessage, CachedMessage, bool) {
		for i, m := range msgs {
			if m.MessageID != msgID {
				continue
			}
			changed := false
			updated := []ReactionDetail{}
			for _, r := range msgs[i].RecentReactions {
				if r.User != userName {
					updated = append(updated, r)
				} else {
					changed = true
				}
			}
			for _, r := range newReactions {
				if r.Type == "emoji" {
					updated = append([]ReactionDetail{{User: userName, Emoji: r.Emoji}}, updated...)
					changed = true
					break
				}
			}
			if len(updated) > 5 {
				updated = updated[:5]
			}
			msgs[i].RecentReactions = updated
			return msgs, msgs[i], changed
		}
		return msgs, CachedMessage{}, false
	}

	if c.client != nil {
		msgs := c.GetMessages(chatID)
		msgs, result, changed := update(msgs)
		if changed {
			c.setMessages(chatID, msgs)
		}
		return result, changed
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	msgs := c.messages[chatID]
	msgs, result, changed := update(msgs)
	if changed {
		c.messages[chatID] = msgs
	}
	return result, changed
}

// SetBotReaction records the bot's own reaction on a cached message.
func (c *Cache) SetBotReaction(chatID int64, msgID int, emoji string) (CachedMessage, bool) {
	if c.client != nil {
		msgs := c.GetMessages(chatID)
		for i, m := range msgs {
			if m.MessageID == msgID {
				msgs[i].BotReaction = emoji
				c.setMessages(chatID, msgs)
				return msgs[i], true
			}
		}
		return CachedMessage{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	msgs := c.messages[chatID]
	for i, m := range msgs {
		if m.MessageID == msgID {
			msgs[i].BotReaction = emoji
			return msgs[i], true
		}
	}
	return CachedMessage{}, false
}

// GetAllMessages returns all cached messages keyed by chat ID.
func (c *Cache) GetAllMessages() map[int64][]CachedMessage {
	if c.client != nil {
		ids, err := c.client.SMembers(c.ctx, keyChatIDs).Result()
		if err != nil {
			log.Printf("Cache: list chat IDs: %v", err)
			return nil
		}
		result := make(map[int64][]CachedMessage, len(ids))
		for _, idStr := range ids {
			chatID, err := strconv.ParseInt(idStr, 10, 64)
			if err != nil {
				continue
			}
			msgs := c.GetMessages(chatID)
			if len(msgs) > 0 {
				result[chatID] = msgs
			}
		}
		return result
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make(map[int64][]CachedMessage, len(c.messages))
	for chatID, msgs := range c.messages {
		cp := make([]CachedMessage, len(msgs))
		copy(cp, msgs)
		result[chatID] = cp
	}
	return result
}

// ── Chat cache ────────────────────────────────────────────────────────────────

// GetChatInfo returns cached chat metadata.
func (c *Cache) GetChatInfo(chatID int64) (ChatInfo, bool) {
	if c.client != nil {
		key := fmt.Sprintf("%s%d", keyPrefixChat, chatID)
		val, err := c.client.Get(c.ctx, key).Result()
		if err == redis.Nil {
			return ChatInfo{}, false
		} else if err != nil {
			log.Printf("Cache: get chat %d: %v", chatID, err)
			return ChatInfo{}, false
		}
		var info ChatInfo
		if err := json.Unmarshal([]byte(val), &info); err != nil {
			log.Printf("Cache: unmarshal chat %d: %v", chatID, err)
			return ChatInfo{}, false
		}
		return info, true
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	info, ok := c.chats[chatID]
	return info, ok
}

// SetChatInfo stores chat metadata and records the chat ID in the index set.
func (c *Cache) SetChatInfo(chatID int64, info ChatInfo) {
	if c.client != nil {
		key := fmt.Sprintf("%s%d", keyPrefixChat, chatID)
		data, err := json.Marshal(info)
		if err != nil {
			log.Printf("Cache: marshal chat %d: %v", chatID, err)
			return
		}
		pipe := c.client.Pipeline()
		pipe.Set(c.ctx, key, data, 0)
		pipe.SAdd(c.ctx, keyChatIDs, chatID)
		if _, err := pipe.Exec(c.ctx); err != nil {
			log.Printf("Cache: set chat %d: %v", chatID, err)
		}
		return
	}
	c.mu.Lock()
	c.chats[chatID] = info
	c.mu.Unlock()
}

// GetAllChats returns all cached chat metadata.
func (c *Cache) GetAllChats() []ChatInfo {
	if c.client != nil {
		ids, err := c.client.SMembers(c.ctx, keyChatIDs).Result()
		if err != nil {
			log.Printf("Cache: list chat IDs: %v", err)
			return nil
		}
		result := make([]ChatInfo, 0, len(ids))
		for _, idStr := range ids {
			chatID, err := strconv.ParseInt(idStr, 10, 64)
			if err != nil {
				continue
			}
			if info, ok := c.GetChatInfo(chatID); ok {
				result = append(result, info)
			}
		}
		return result
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]ChatInfo, 0, len(c.chats))
	for _, info := range c.chats {
		result = append(result, info)
	}
	return result
}

// ── Available reactions ────────────────────────────────────────────────────────

// GetReactions returns the allowed emoji reactions for a chat.
func (c *Cache) GetReactions(chatID int64) ([]string, bool) {
	if c.client != nil {
		key := fmt.Sprintf("%s%d", keyPrefixReactions, chatID)
		val, err := c.client.Get(c.ctx, key).Result()
		if err == redis.Nil {
			return nil, false
		} else if err != nil {
			log.Printf("Cache: get reactions chat %d: %v", chatID, err)
			return nil, false
		}
		var reactions []string
		if err := json.Unmarshal([]byte(val), &reactions); err != nil {
			log.Printf("Cache: unmarshal reactions chat %d: %v", chatID, err)
			return nil, false
		}
		return reactions, true
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	reactions, ok := c.reactions[chatID]
	return reactions, ok
}

// SetReactions stores the allowed emoji reactions for a chat.
func (c *Cache) SetReactions(chatID int64, reactions []string) {
	if c.client != nil {
		key := fmt.Sprintf("%s%d", keyPrefixReactions, chatID)
		data, err := json.Marshal(reactions)
		if err != nil {
			log.Printf("Cache: marshal reactions chat %d: %v", chatID, err)
			return
		}
		if err := c.client.Set(c.ctx, key, data, 0).Err(); err != nil {
			log.Printf("Cache: set reactions chat %d: %v", chatID, err)
		}
		return
	}
	c.mu.Lock()
	c.reactions[chatID] = reactions
	c.mu.Unlock()
}

// ── Captcha cache ─────────────────────────────────────────────────────────────

// GetCaptcha returns the pending captcha state for a user.
func (c *Cache) GetCaptcha(userID int64) (*PendingCaptcha, bool) {
	var record pendingCaptchaRecord

	if c.client != nil {
		key := fmt.Sprintf("%s%d", keyPrefixCaptcha, userID)
		val, err := c.client.Get(c.ctx, key).Result()
		if err == redis.Nil {
			return nil, false
		} else if err != nil {
			log.Printf("Cache: get captcha user %d: %v", userID, err)
			return nil, false
		}
		if err := json.Unmarshal([]byte(val), &record); err != nil {
			log.Printf("Cache: unmarshal captcha user %d: %v", userID, err)
			return nil, false
		}
	} else {
		c.mu.RLock()
		r, ok := c.captchas[userID]
		c.mu.RUnlock()
		if !ok {
			return nil, false
		}
		record = *r
	}

	c.timerMu.Lock()
	timer := c.timers[userID]
	c.timerMu.Unlock()

	return &PendingCaptcha{
		UserID:          record.UserID,
		ChatID:          record.ChatID,
		CorrectAnswer:   record.CorrectAnswer,
		CaptchaMsgID:    record.CaptchaMsgID,
		JoinMsgID:       record.JoinMsgID,
		Attempts:        record.Attempts,
		ExpirationTimer: timer,
	}, true
}

// SetCaptcha stores a pending captcha. The ExpirationTimer is kept in-memory;
// when using Valkey the record is stored with the given TTL.
func (c *Cache) SetCaptcha(userID int64, captcha *PendingCaptcha, ttl time.Duration) {
	record := pendingCaptchaRecord{
		UserID:        captcha.UserID,
		ChatID:        captcha.ChatID,
		CorrectAnswer: captcha.CorrectAnswer,
		CaptchaMsgID:  captcha.CaptchaMsgID,
		JoinMsgID:     captcha.JoinMsgID,
		Attempts:      captcha.Attempts,
	}
	if c.client != nil {
		key := fmt.Sprintf("%s%d", keyPrefixCaptcha, userID)
		data, err := json.Marshal(record)
		if err != nil {
			log.Printf("Cache: marshal captcha user %d: %v", userID, err)
			return
		}
		if err := c.client.Set(c.ctx, key, data, ttl).Err(); err != nil {
			log.Printf("Cache: set captcha user %d: %v", userID, err)
		}
	} else {
		c.mu.Lock()
		c.captchas[userID] = &record
		c.mu.Unlock()
	}
	if captcha.ExpirationTimer != nil {
		c.timerMu.Lock()
		c.timers[userID] = captcha.ExpirationTimer
		c.timerMu.Unlock()
	}
}

// DeleteCaptcha removes a pending captcha and its associated timer.
func (c *Cache) DeleteCaptcha(userID int64) {
	if c.client != nil {
		key := fmt.Sprintf("%s%d", keyPrefixCaptcha, userID)
		if err := c.client.Del(c.ctx, key).Err(); err != nil {
			log.Printf("Cache: delete captcha user %d: %v", userID, err)
		}
	} else {
		c.mu.Lock()
		delete(c.captchas, userID)
		c.mu.Unlock()
	}
	c.timerMu.Lock()
	delete(c.timers, userID)
	c.timerMu.Unlock()
}

// HasCaptcha reports whether a captcha is pending for the given user.
func (c *Cache) HasCaptcha(userID int64) bool {
	if c.client != nil {
		key := fmt.Sprintf("%s%d", keyPrefixCaptcha, userID)
		n, err := c.client.Exists(c.ctx, key).Result()
		if err != nil {
			log.Printf("Cache: exists captcha user %d: %v", userID, err)
			return false
		}
		return n > 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.captchas[userID]
	return ok
}

// UpdateCaptchaJoinMsgID sets JoinMsgID on a pending captcha if it was zero.
func (c *Cache) UpdateCaptchaJoinMsgID(userID int64, joinMsgID int) {
	if joinMsgID == 0 {
		return
	}
	pending, ok := c.GetCaptcha(userID)
	if !ok || pending.JoinMsgID != 0 {
		return
	}
	pending.JoinMsgID = joinMsgID
	var remaining time.Duration
	if c.client != nil {
		key := fmt.Sprintf("%s%d", keyPrefixCaptcha, userID)
		remaining, _ = c.client.TTL(c.ctx, key).Result()
	}
	c.SetCaptcha(userID, pending, remaining)
}

// ── Join processed cache ──────────────────────────────────────────────────────

// GetJoinEntry returns the join deduplication record for a user.
func (c *Cache) GetJoinEntry(userID int64) (*JoinProcessedEntry, bool) {
	if c.client != nil {
		key := fmt.Sprintf("%s%d", keyPrefixJoin, userID)
		val, err := c.client.Get(c.ctx, key).Result()
		if err == redis.Nil {
			return nil, false
		} else if err != nil {
			log.Printf("Cache: get join entry user %d: %v", userID, err)
			return nil, false
		}
		var entry JoinProcessedEntry
		if err := json.Unmarshal([]byte(val), &entry); err != nil {
			log.Printf("Cache: unmarshal join entry user %d: %v", userID, err)
			return nil, false
		}
		return &entry, true
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.joins[userID]
	if !ok {
		return nil, false
	}
	cp := *entry
	return &cp, true
}

// SetJoinEntry stores a join deduplication record with a 1-minute TTL.
func (c *Cache) SetJoinEntry(userID int64, entry *JoinProcessedEntry) {
	if c.client != nil {
		key := fmt.Sprintf("%s%d", keyPrefixJoin, userID)
		data, err := json.Marshal(entry)
		if err != nil {
			log.Printf("Cache: marshal join entry user %d: %v", userID, err)
			return
		}
		if err := c.client.Set(c.ctx, key, data, joinTTL).Err(); err != nil {
			log.Printf("Cache: set join entry user %d: %v", userID, err)
		}
		return
	}
	c.mu.Lock()
	c.joins[userID] = entry
	c.mu.Unlock()
}

// UpdateJoinEntryBanned marks a join record as banned.
func (c *Cache) UpdateJoinEntryBanned(userID int64) {
	entry, ok := c.GetJoinEntry(userID)
	if !ok {
		return
	}
	entry.IsBanned = true
	c.SetJoinEntry(userID, entry)
}

// UpdateJoinEntryMsgID sets JoinMsgID on an existing join record if it was zero.
func (c *Cache) UpdateJoinEntryMsgID(userID int64, joinMsgID int) (*JoinProcessedEntry, bool) {
	entry, ok := c.GetJoinEntry(userID)
	if !ok {
		return nil, false
	}
	if joinMsgID != 0 && entry.JoinMsgID == 0 {
		entry.JoinMsgID = joinMsgID
		c.SetJoinEntry(userID, entry)
	}
	return entry, true
}

// CleanupJoinEntries removes stale entries in in-memory mode (Redis handles
// TTL expiry automatically).
func (c *Cache) CleanupJoinEntries() {
	if c.client != nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, entry := range c.joins {
		if time.Since(entry.Time) > joinTTL {
			delete(c.joins, id)
		}
	}
}

