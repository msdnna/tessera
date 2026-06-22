package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"tessera/internal/realtime"
)

// WSHandler upgrades HTTP requests to WebSocket connections and registers them
// with the realtime hub.
type WSHandler struct {
	hub      *realtime.Hub
	upgrader websocket.Upgrader
}

// NewWSHandler returns a WSHandler backed by the given hub.
func NewWSHandler(hub *realtime.Hub) *WSHandler {
	return &WSHandler{
		hub: hub,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			// TODO(phase1): restrict to the web origin and authenticate the
			// connection (JWT via query param or subprotocol) before upgrading.
			CheckOrigin: func(_ *http.Request) bool { return true },
		},
	}
}

// Connect upgrades the request to a WebSocket and registers it with the hub so
// it receives live board updates.
func (h *WSHandler) Connect(c *gin.Context) {
	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return // Upgrade already wrote an error response.
	}
	realtime.NewClient(h.hub, conn).Start()
}
