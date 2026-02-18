package webui

import (
	"log"
	"net/http"
	"sync"

	"github.com/birabittoh/escarbot/telegram"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for local development
	},
}

// MessageHub manages WebSocket connections and broadcasts messages
type MessageHub struct {
	clients    map[*websocket.Conn]bool
	broadcast  chan telegram.CachedMessage
	register   chan *websocket.Conn
	unregister chan *websocket.Conn
	mu         sync.Mutex
}

// Global hub instance
var hub *MessageHub

// InitMessageHub initializes the message hub
func InitMessageHub() {
	hub = &MessageHub{
		clients:    make(map[*websocket.Conn]bool),
		broadcast:  make(chan telegram.CachedMessage, 100),
		register:   make(chan *websocket.Conn),
		unregister: make(chan *websocket.Conn),
	}

	go hub.run()
}

// run handles hub operations
func (h *MessageHub) run() {
	for {
		select {
		case conn := <-h.register:
			h.mu.Lock()
			h.clients[conn] = true
			h.mu.Unlock()
			log.Printf("WebSocket client connected (total: %d)", len(h.clients))

		case conn := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[conn]; ok {
				delete(h.clients, conn)
				conn.Close()
			}
			h.mu.Unlock()
			log.Printf("WebSocket client disconnected (total: %d)", len(h.clients))

		case message := <-h.broadcast:
			h.mu.Lock()
			for conn := range h.clients {
				err := conn.WriteJSON(message)
				if err != nil {
					log.Printf("Error broadcasting message: %v", err)
					conn.Close()
					delete(h.clients, conn)
				}
			}
			h.mu.Unlock()
		}
	}
}

// BroadcastMessage sends a message to all connected clients
func BroadcastMessage(msg telegram.CachedMessage) {
	if hub != nil {
		select {
		case hub.broadcast <- msg:
		default:
			log.Println("Broadcast channel full, dropping message")
		}
	}
}

// wsHandler handles WebSocket connections
func wsHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Error upgrading to WebSocket: %v", err)
		return
	}

	hub.register <- conn

	// Keep connection alive and handle disconnection
	go func() {
		defer func() {
			hub.unregister <- conn
		}()

		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				break
			}
		}
	}()
}
