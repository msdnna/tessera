// Package realtime is the WebSocket fan-out layer for live board updates.
//
// Phase 0 lays the architecture: a single Hub broadcasts Events to every
// connected client. Phase 1+ adds JWT auth on connect and per-workspace/board
// scoping (clients only receive Events whose Scope they're subscribed to).
package realtime

import (
	"encoding/json"
	"log"
	"sync"
)

// Event is a server→client message. Scope lets clients filter to the
// workspace/board they're viewing (scoping is wired in a later phase).
type Event struct {
	Scope string          `json:"scope"`
	Type  string          `json:"type"`
	Data  json.RawMessage `json:"data,omitempty"`
}

type Hub struct {
	mu         sync.RWMutex
	clients    map[*Client]struct{}
	register   chan *Client
	unregister chan *Client
	broadcast  chan Event
}

func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]struct{}),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan Event, 64),
	}
}

// Run owns the client set; call it once in its own goroutine.
func (h *Hub) Run() {
	for {
		select {
		case c := <-h.register:
			h.mu.Lock()
			h.clients[c] = struct{}{}
			h.mu.Unlock()
		case c := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[c]; ok {
				delete(h.clients, c)
				close(c.send)
			}
			h.mu.Unlock()
		case ev := <-h.broadcast:
			h.mu.RLock()
			for c := range h.clients {
				select {
				case c.send <- ev:
				default:
					// Slow client — drop the event for it rather than block
					// the whole hub. A persistently slow client gets evicted
					// by its own write deadline.
					log.Printf("realtime: dropping event for slow client")
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Broadcast queues an event for fan-out to all connected clients.
func (h *Hub) Broadcast(ev Event) {
	select {
	case h.broadcast <- ev:
	default:
		log.Printf("realtime: broadcast buffer full, dropping event %q", ev.Type)
	}
}
