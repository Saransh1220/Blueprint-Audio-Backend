package websocket

import (
	"log"
	"sync"

	"github.com/google/uuid"
)

type UnicastMessage struct {
	UserID  uuid.UUID
	Message []byte
}

// Hub maintains the set of active clients and broadcasts messages to the
// clients.
type Hub struct {
	// Registered clients.
	clients map[*Client]bool

	// Inbound messages from the clients.
	broadcast chan []byte

	// Unicast messages
	unicast chan UnicastMessage

	// Register requests from the clients.
	register chan *Client

	// Unregister requests from clients.
	unregister chan *Client

	// Channel to signal termination
	stop     chan struct{}
	stopOnce sync.Once
}

func NewHub() *Hub {
	return &Hub{
		broadcast:  make(chan []byte),
		unicast:    make(chan UnicastMessage),
		register:   make(chan *Client),
		unregister: make(chan *Client),

		clients: make(map[*Client]bool),
		stop:    make(chan struct{}),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
			addr := "test"
			if client.conn != nil {
				addr = client.conn.RemoteAddr().String()
			}
			log.Printf("[WebSocket Hub] Client registered: %v (User: %s)", addr, client.userID)
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
				addr := "test"
				if client.conn != nil {
					addr = client.conn.RemoteAddr().String()
				}
				log.Printf("[WebSocket Hub] Client unregistered: %v (User: %s)", addr, client.userID)
			}
		case message := <-h.broadcast:
			log.Printf("[WebSocket Hub] Broadcasting message to %d clients", len(h.clients))
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
		case msg := <-h.unicast:
			log.Printf("[WebSocket Hub] Sending unicast to user: %s", msg.UserID)
			for client := range h.clients {
				if client.userID == msg.UserID {
					select {
					case client.send <- msg.Message:
					default:
						close(client.send)
						delete(h.clients, client)
					}
				}
			}
		case <-h.stop:
			log.Println("[WebSocket Hub] Stopping hub")
			for client := range h.clients {
				close(client.send)
				delete(h.clients, client)
			}
			return
		}
	}
}

func (h *Hub) BroadcastMessage(message []byte) {
	select {
	case h.broadcast <- message:
	case <-h.stop:
	}
}

func (h *Hub) SendToUser(userID uuid.UUID, message []byte) {
	select {
	case h.unicast <- UnicastMessage{UserID: userID, Message: message}:
	case <-h.stop:
	}
}

func (h *Hub) Stop() {
	h.stopOnce.Do(func() {
		close(h.stop)
	})
}
