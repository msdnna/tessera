package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"tessera/internal/db"
	"tessera/internal/docroom"
	"tessera/internal/realtime"
	"tessera/middleware"
)

// bearerSubprotocol is the WebSocket subprotocol browsers use to carry the
// bearer credential. The browser WebSocket API cannot set request headers, and
// putting the token in the query string would write it straight into the access
// log — so the token rides as the second entry of Sec-WebSocket-Protocol
// (`new WebSocket(url, ['bearer', token])`), which is a header and is not
// logged. The server must echo the selected subprotocol back or Chrome fails
// the handshake, hence upgrader.Subprotocols below.
const bearerSubprotocol = "bearer"

// WSHandler upgrades HTTP requests to WebSocket connections and registers them
// with the realtime hub. The connection is authenticated and scoped to the
// user's workspaces *before* the upgrade, so an unauthenticated client never
// gets a socket and an authenticated one never sees another workspace's events.
type WSHandler struct {
	hub      *realtime.Hub
	rooms    *docroom.Rooms // per-document presence/locks (ConnectDocument)
	q        *db.Queries
	secret   string
	upgrader websocket.Upgrader
}

// NewWSHandler returns a WSHandler backed by the given hub and document-room
// registry. allowedOrigins is the browser-origin allowlist (the CORS origin plus
// any desktop origins); requests from other browser origins are refused to block
// cross-site WebSocket hijacking. Native clients (Android, CLI) send no Origin at
// all and are admitted on their bearer credential alone.
func NewWSHandler(hub *realtime.Hub, rooms *docroom.Rooms, q *db.Queries, secret string, allowedOrigins ...string) *WSHandler {
	allowed, wildcard := originAllowlist(allowedOrigins)
	return &WSHandler{
		hub:    hub,
		rooms:  rooms,
		q:      q,
		secret: secret,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			Subprotocols:    []string{bearerSubprotocol},
			CheckOrigin: func(r *http.Request) bool {
				origin := r.Header.Get("Origin")
				if origin == "" {
					return true // non-browser client; no CSWSH risk
				}
				return wildcard || allowed[origin]
			},
		},
	}
}

// originAllowlist mirrors middleware.CORS's parsing so the WebSocket and the
// REST surface admit exactly the same origins.
func originAllowlist(origins []string) (map[string]bool, bool) {
	allowed := make(map[string]bool, len(origins))
	wildcard := false
	for _, o := range origins {
		if o == "" || o == "*" {
			wildcard = true
			continue
		}
		allowed[o] = true
	}
	return allowed, wildcard
}

// Connect authenticates the request, resolves the user's workspace scopes and
// upgrades to a WebSocket registered with the hub.
func (h *WSHandler) Connect(c *gin.Context) {
	uid, ok := h.authenticate(c)
	if !ok {
		// Answer before the upgrade so the client gets a real status code
		// instead of an opaque handshake failure.
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		return
	}

	scopes, err := h.scopesFor(c, uid)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return // Upgrade already wrote an error response.
	}
	realtime.NewClient(h.hub, conn, uid, scopes).Start()
}

// authenticate resolves the bearer credential from either the Authorization
// header (native clients) or the bearer subprotocol (browsers).
func (h *WSHandler) authenticate(c *gin.Context) (uuid.UUID, bool) {
	tok := ""
	if hdr := c.GetHeader("Authorization"); strings.HasPrefix(hdr, "Bearer ") {
		tok = strings.TrimPrefix(hdr, "Bearer ")
	} else {
		tok = subprotocolToken(c.GetHeader("Sec-WebSocket-Protocol"))
	}
	if tok == "" {
		return uuid.Nil, false
	}
	// nil toucher: a WS connection authenticates once, so throttling its
	// last_used_at write buys nothing.
	return middleware.ResolveBearer(c, h.secret, h.q, tok, nil)
}

// subprotocolToken pulls the credential out of a `bearer, <token>` subprotocol
// list, returning "" when the header isn't in that form.
func subprotocolToken(header string) string {
	parts := strings.Split(header, ",")
	if len(parts) < 2 || strings.TrimSpace(parts[0]) != bearerSubprotocol {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

// scopesFor returns the workspace ids the user may receive events for.
func (h *WSHandler) scopesFor(c *gin.Context, uid uuid.UUID) ([]string, error) {
	rows, err := h.q.ListWorkspacesForUser(c, uid)
	if err != nil {
		return nil, err
	}
	scopes := make([]string, 0, len(rows))
	for _, w := range rows {
		scopes = append(scopes, w.ID.String())
	}
	return scopes, nil
}
