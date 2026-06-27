// Package ws provides a WebSocket hub for pushing real-time events (host status,
// session activity) to connected dashboards.
package ws

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"

	"github.com/fleet-terminal/backend/internal/app"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// Event is a typed message broadcast to clients.
type Event struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// Hub fans out events to all connected clients.
type Hub struct {
	mu      sync.RWMutex
	clients map[*client]struct{}
}

// NewHub constructs an empty Hub.
func NewHub() *Hub { return &Hub{clients: make(map[*client]struct{})} }

// Broadcast sends an event to every connected client (best-effort, non-blocking).
func (h *Hub) Broadcast(eventType string, data any) {
	payload, err := json.Marshal(Event{Type: eventType, Data: data})
	if err != nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		select {
		case c.send <- payload:
		default:
			// Slow client: drop the message rather than block the hub.
		}
	}
}

func (h *Hub) add(c *client) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
}

func (h *Hub) remove(c *client) {
	h.mu.Lock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		close(c.send)
	}
	h.mu.Unlock()
}

type client struct {
	conn *websocket.Conn
	send chan []byte
}

// Mount attaches the events WebSocket endpoint. Clients authenticate with a
// short-lived access token passed as a query parameter.
func Mount(r chi.Router, d *app.Deps, hub *Hub) {
	r.Get("/events/ws", func(w http.ResponseWriter, req *http.Request) {
		token := req.URL.Query().Get("token")
		if _, err := d.Auth.AuthenticateToken(req.Context(), token); err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		conn, err := upgrader.Upgrade(w, req, nil)
		if err != nil {
			return
		}
		c := &client{conn: conn, send: make(chan []byte, 32)}
		hub.add(c)
		go c.writePump()
		c.readPump(hub)
	})
}

func (c *client) readPump(h *Hub) {
	defer func() {
		h.remove(c)
		_ = c.conn.Close()
	}()
	for {
		// We don't expect inbound messages; reading detects disconnects.
		if _, _, err := c.conn.ReadMessage(); err != nil {
			return
		}
	}
}

func (c *client) writePump() {
	for msg := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			return
		}
	}
}
