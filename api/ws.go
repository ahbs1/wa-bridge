package api

import (
	"sync"

	"github.com/gorilla/websocket"
)

type WSHub struct {
	clients map[*websocket.Conn]bool
	mu      sync.RWMutex
}

var Hub *WSHub

func NewWSHub() *WSHub {
	Hub = &WSHub{
		clients: make(map[*websocket.Conn]bool),
	}
	return Hub
}

func (h *WSHub) Add(conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[conn] = true
}

func (h *WSHub) Remove(conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, conn)
	conn.Close()
}

func (h *WSHub) Broadcast(data interface{}) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for conn := range h.clients {
		if err := conn.WriteJSON(data); err != nil {
			go h.Remove(conn)
		}
	}
}
