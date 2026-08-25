package realtime

import (
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"tessera/internal/observability"
)

// EventResync tells a client it missed at least one event (its buffer overflowed)
// and should reload its current view. It carries no data — the client refetches.
const EventResync = "resync"

const (
	writeWait = 10 * time.Second
	pongWait  = 60 * time.Second
	// Ping well inside common proxy idle timeouts (nginx/Caddy default to ~60s;
	// the org install sits behind a third-party proxy we don't control): a 25s
	// keepalive keeps an otherwise-quiet socket from being reaped mid-session.
	// Must stay < pongWait so a missed pong still trips the read deadline.
	pingPeriod = 25 * time.Second
)

// Client is one authenticated WebSocket connection. Clients are read-only
// consumers of broadcasts; the read pump exists to process control frames
// (pong) and detect disconnects.
//
// scopes is the set of workspace ids the user belonged to at connect time and
// is immutable for the life of the connection — Hub.DropUser closes the socket
// on any membership change so the set is re-read on reconnect rather than
// mutated under the fan-out loop.
type Client struct {
	hub    *Hub
	conn   *websocket.Conn
	send   chan Event
	userID uuid.UUID
	scopes map[string]struct{}
	// needResync is set by the hub when it has to drop an event for this client
	// (send buffer full). The write pump then emits a single resync marker at the
	// next opportunity, so a recovered-but-still-connected client refills its view
	// without waiting for a reconnect.
	needResync atomic.Bool
}

// NewClient wraps an authenticated WebSocket connection as a hub Client that
// receives events for the given workspace scopes only.
func NewClient(hub *Hub, conn *websocket.Conn, userID uuid.UUID, scopes []string) *Client {
	set := make(map[string]struct{}, len(scopes))
	for _, s := range scopes {
		set[s] = struct{}{}
	}
	return &Client{hub: hub, conn: conn, send: make(chan Event, 32), userID: userID, scopes: set}
}

// canSee reports whether this client may receive an event with the given scope.
// An empty scope means "no workspace" and is delivered to nobody: every domain
// event carries its workspace id, so a scopeless event is a bug, not a
// broadcast-to-all.
func (c *Client) canSee(scope string) bool {
	if scope == "" {
		return false
	}
	_, ok := c.scopes[scope]
	return ok
}

// Start registers the client and launches its read/write pumps. A connection
// that arrives after the hub has been closed is dropped instead of blocking
// forever on a channel nobody reads.
func (c *Client) Start() {
	select {
	case c.hub.register <- c:
	case <-c.hub.done:
		_ = c.conn.Close()
		return
	}
	go c.writePump()
	go c.readPump()
}

func (c *Client) readPump() {
	// Per-connection goroutine: report a panic instead of crashing the server; the
	// unregister/close cleanup below still runs on the way out.
	defer observability.Recover("ws-read")
	defer func() {
		select {
		case c.hub.unregister <- c:
		case <-c.hub.done:
			// Hub is gone; it already closed our send channel.
		}
		_ = c.conn.Close()
	}()
	c.conn.SetReadLimit(512)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			break
		}
	}
}

// flushResync emits a single resync marker if the hub flagged this client as
// having missed an event. The flag is cleared only after a successful write, so
// a write failure (which tears the connection down) leaves it set — harmless,
// because the client refetches on the reconnect that follows anyway.
func (c *Client) flushResync() error {
	if !c.needResync.Load() {
		return nil
	}
	_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
	if err := c.conn.WriteJSON(Event{Type: EventResync}); err != nil {
		return err
	}
	c.needResync.Store(false)
	return nil
}

func (c *Client) writePump() {
	defer observability.Recover("ws-write")
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()
	for {
		select {
		case ev, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// Hub closed the channel.
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteJSON(ev); err != nil {
				return
			}
			if err := c.flushResync(); err != nil {
				return
			}
		case <-ticker.C:
			if err := c.flushResync(); err != nil {
				return
			}
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
