package realtime

import (
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait = 10 * time.Second
	pongWait  = 60 * time.Second
	// Ping well inside common proxy idle timeouts (nginx/Caddy default to ~60s;
	// the org install sits behind a third-party proxy we don't control): a 25s
	// keepalive keeps an otherwise-quiet socket from being reaped mid-session.
	// Must stay < pongWait so a missed pong still trips the read deadline.
	pingPeriod = 25 * time.Second
)

// Client is one WebSocket connection. Clients are currently read-only
// consumers of broadcasts; the read pump exists to process control frames
// (pong) and detect disconnects.
type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan Event
}

// NewClient wraps a WebSocket connection as a hub Client.
func NewClient(hub *Hub, conn *websocket.Conn) *Client {
	return &Client{hub: hub, conn: conn, send: make(chan Event, 32)}
}

// Start registers the client and launches its read/write pumps.
func (c *Client) Start() {
	c.hub.register <- c
	go c.writePump()
	go c.readPump()
}

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
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

func (c *Client) writePump() {
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
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
