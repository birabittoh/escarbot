package telegram

import (
	"log"
	"strings"
	"sync"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"
)

// Set di chat in cui il prossimo messaggio deve essere cancellato
var (
	pendingDelete = make(map[int64]int) // chatID -> joinMessageID
	pendingMutex  sync.Mutex
)

// handleNewChatMembers gestisce l'ingresso di nuovi membri nel gruppo
func handleNewChatMembers(escarbot *EscarBot, message *tgbotapi.Message) {
	if message.NewChatMembers == nil {
		return
	}

	for _, user := range message.NewChatMembers {
		// Ignora i bot
		if user.IsBot {
			continue
		}

		// Controlla il canale personale dell'utente
		if hasAdultChannel(escarbot.Bot, user.ID) {
			log.Printf("Utente %d (%s) ha un canale 18+, procedo con il ban", user.ID, user.UserName)
			banUser(escarbot.Bot, message.Chat.ID, user.ID)
			markForDeletion(message.Chat.ID, message.MessageID)
		}
	}
}

// hasAdultChannel controlla se l'utente ha un canale personale con "18+" nel nome
func hasAdultChannel(bot *tgbotapi.BotAPI, userID int64) bool {
	chatConfig := tgbotapi.ChatInfoConfig{
		ChatConfig: tgbotapi.ChatConfig{
			ChatID: userID,
		},
	}

	chat, err := bot.GetChat(chatConfig)
	if err != nil {
		log.Printf("Impossibile ottenere info utente %d: %v", userID, err)
		return false
	}

	// Controlla il canale personale (Personal Channel)
	if chat.PersonalChat != nil {
		channelName := chat.PersonalChat.Title
		if strings.Contains(channelName, "18+") {
			log.Printf("Trovato canale personale con 18+: %s", channelName)
			return true
		}
	}

	return false
}

// banUser banna un utente dal gruppo
func banUser(bot *tgbotapi.BotAPI, chatID int64, userID int64) {
	banConfig := tgbotapi.BanChatMemberConfig{
		ChatMemberConfig: tgbotapi.ChatMemberConfig{
			ChatID: chatID,
			UserID: userID,
		},
		RevokeMessages: true,
	}

	_, err := bot.Request(banConfig)
	if err != nil {
		log.Printf("Errore nel bannare utente %d: %v", userID, err)
	} else {
		log.Printf("Utente %d bannato con successo", userID)
	}
}

// markForDeletion segna una chat per la cancellazione del prossimo messaggio
func markForDeletion(chatID int64, joinMessageID int) {
	pendingMutex.Lock()
	defer pendingMutex.Unlock()
	pendingDelete[chatID] = joinMessageID
}

// checkAndDeleteMessage controlla se il messaggio deve essere cancellato (es. messaggio di benvenuto)
func checkAndDeleteMessage(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	pendingMutex.Lock()
	joinMessageID, exists := pendingDelete[message.Chat.ID]
	if exists && message.MessageID > joinMessageID {
		delete(pendingDelete, message.Chat.ID)
		pendingMutex.Unlock()

		deleteMsg := tgbotapi.NewDeleteMessage(message.Chat.ID, message.MessageID)
		_, err := bot.Request(deleteMsg)
		if err != nil {
			log.Printf("Errore nel cancellare messaggio di benvenuto: %v", err)
		} else {
			log.Printf("Messaggio di benvenuto cancellato (chat %d)", message.Chat.ID)
		}
		return
	}
	pendingMutex.Unlock()
}
