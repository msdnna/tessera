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
// workspace/board they're viewing (scoping is wired in a later phase). Actor,
// when set, carries the id of the user who triggered the event so clients can
// attribute board-activity ("X created …"); it is absent for system/worker
// actions (GitLab sync, recurrence).
type Event struct {
	Scope string          `json:"scope"`
	Type  string          `json:"type"`
	Actor string          `json:"actor,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
}

// Hub fans out Events to every connected client.
type Hub struct {
	mu         sync.RWMutex
	clients    map[*Client]struct{}
	register   chan *Client
	unregister chan *Client
	broadcast  chan Event
	// done is closed by Close to stop Run and release every client. Clients
	// select on it when handing themselves to Run so a connection racing with
	// shutdown parks nobody on an unread channel.
	done      chan struct{}
	closeOnce sync.Once
}

// NewHub returns an initialised Hub ready to Run.
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]struct{}),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan Event, 64),
		done:       make(chan struct{}),
	}
}

// Close stops Run and disconnects every client with a normal close frame, so
// browsers reconnect to the replacement process instead of reporting a broken
// socket. Safe to call more than once; safe to call without a running Run.
func (h *Hub) Close() {
	h.closeOnce.Do(func() { close(h.done) })
}

// Run owns the client set; call it once in its own goroutine. It returns once
// Close is called.
func (h *Hub) Run() {
	for {
		select {
		case <-h.done:
			// Closing each send channel makes the write pumps emit a close
			// frame and exit. Run owns the map, so nothing else closes these.
			h.mu.Lock()
			for c := range h.clients {
				delete(h.clients, c)
				close(c.send)
			}
			h.mu.Unlock()
			return
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
