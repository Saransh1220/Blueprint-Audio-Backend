package websocket

import (
	"log"

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
}

func NewHub() *Hub {
	return &Hub{
		broadcast:  make(chan []byte),
		unicast:    make(chan UnicastMessage),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[*Client]bool),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
			log.Printf("[WebSocket Hub] Client registered: %v (User: %s)", client.conn.RemoteAddr(), client.userID)
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
				log.Printf("[WebSocket Hub] Client unregistered: %v (User: %s)", client.conn.RemoteAddr(), client.userID)
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
		}
	}
}

func (h *Hub) BroadcastMessage(message []byte) {
	h.broadcast <- message
}

func (h *Hub) SendToUser(userID uuid.UUID, message []byte) {
	h.unicast <- UnicastMessage{UserID: userID, Message: message}
}
