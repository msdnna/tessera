// Package realtime is the WebSocket fan-out layer for live board updates.
//
// A single Hub owns every connected client and fans Events out to those
// authorised to see them: the connection is authenticated before the upgrade
// (see handlers.WSHandler) and each Client carries the set of workspace ids its
// user is a member of, so an Event is delivered only to clients whose scope set
// contains Event.Scope.
package realtime

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/google/uuid"
)

// Event is a server→client message. Scope is the workspace id the event belongs
// to and is authoritative for delivery: the hub drops the event for any client
// not a member of that workspace. Actor, when set, carries the id of the user
// who triggered the event so clients can attribute board-activity ("X created
// …"); it is absent for system/worker actions (GitLab sync, recurrence).
type Event struct {
	Scope string          `json:"scope"`
	Type  string          `json:"type"`
	Actor string          `json:"actor,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
}

// Hub fans out Events to the clients authorised to receive them.
type Hub struct {
	mu         sync.RWMutex
	clients    map[*Client]struct{}
	register   chan *Client
	unregister chan *Client
	dropUser   chan uuid.UUID
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
		dropUser:   make(chan uuid.UUID, 8),
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
		case uid := <-h.dropUser:
			h.mu.Lock()
			for c := range h.clients {
				if c.userID == uid {
					delete(h.clients, c)
					close(c.send)
				}
			}
			h.mu.Unlock()
		case ev := <-h.broadcast:
			h.mu.RLock()
			for c := range h.clients {
				if !c.canSee(ev.Scope) {
					continue
				}
				select {
				case c.send <- ev:
				default:
					// Slow client — drop the event for it rather than block
					// the whole hub. Flag it so the write pump sends a resync
					// marker (the client reloads instead of silently staying
					// stale); a persistently slow client is still evicted by its
					// own write deadline, and refetches on the reconnect.
					c.needResync.Store(true)
					log.Printf("realtime: dropping event for slow client; flagged resync")
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Broadcast queues an event for fan-out to the clients scoped to ev.Scope.
func (h *Hub) Broadcast(ev Event) {
	select {
	case h.broadcast <- ev:
	default:
		log.Printf("realtime: broadcast buffer full, dropping event %q", ev.Type)
	}
}

// ClientCount returns the number of live sockets, for the /admin/metrics probe.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// DropUser closes every socket belonging to a user, forcing their clients to
// reconnect and re-read their workspace set. Membership is snapshotted at
// connect time, so this is what makes a revoked membership take effect on live
// connections instead of at the next page load. Callers fire it after any
// membership change (add/remove/role/workspace delete); reconnection is
// automatic on every client, so a dropped socket is invisible to the user.
func (h *Hub) DropUser(userID uuid.UUID) {
	select {
	case h.dropUser <- userID:
	default:
		// Run is busy; the membership change still lands on the next
		// reconnect rather than blocking the request that triggered it.
		log.Printf("realtime: drop-user buffer full, skipping drop for %s", userID)
	}
}
