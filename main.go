package main

import (
	"log"
	"net/http"

	"github.com/gorilla/websocket"
)

// WebSocket upgrader
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Allow all origins for development
		return true
	},
}

// chan is a unique Go "channel" for passing byte arrays
// it is a thread-safe queue for goroutines that can send/receive from
type Hub struct {
	clients   map[*websocket.Conn]bool
	broadcast chan []byte
	// Register and unregister act like "command queues", when a new client connects, we send it through register
	// When a client disconnects we send it through unregister
	register   chan *websocket.Conn
	unregister chan *websocket.Conn
}

func newHub() *Hub {
	return &Hub{
		clients:    make(map[*websocket.Conn]bool),
		broadcast:  make(chan []byte),
		register:   make(chan *websocket.Conn),
		unregister: make(chan *websocket.Conn),
	}
}

func (h *Hub) run() {
	for {
		select {
		case conn := <-h.register:
			h.clients[conn] = true
			log.Printf("Client connected. Total: %d", len(h.clients))

		case conn := <-h.unregister:
			if _, ok := h.clients[conn]; ok {
				delete(h.clients, conn)
				conn.Close()
				log.Printf("Client disconnected. Total: %d", len(h.clients))
			}

		case message := <-h.broadcast:
			// Send message to all connected clients
			for conn := range h.clients {
				err := conn.WriteMessage(websocket.TextMessage, message)
				if err != nil {
					log.Printf("Error writing message: %v", err)
					delete(h.clients, conn)
					conn.Close()
				}
			}
		}
	}
}

func handleWebSocket(hub *Hub, w http.ResponseWriter, r *http.Request) {
	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	// Register new client
	hub.register <- conn

	// Handle connection in a goroutine
	go func() {
		defer func() {
			hub.unregister <- conn
		}()

		for {
			// Read message from client
			_, message, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("WebSocket error: %v", err)
				}
				break
			}

			log.Printf("Received: %s", message)

			// Broadcast message to all clients
			hub.broadcast <- message
		}
	}()
}

func main() {
	// Create and start the hub
	hub := newHub()
	go hub.run()

	// WebSocket endpoint
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		handleWebSocket(hub, w, r)
	})

	// Simple health check
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("WebSocket server running"))
	})

	log.Println("WebSocket server starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
