package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"tessera/internal/db"
	"tessera/internal/docroom"
)

const (
	docWriteWait = 10 * time.Second
	docPongWait  = 60 * time.Second
	docPingEvery = 25 * time.Second
	// A lock request is a type plus a block id; nothing legitimate on this socket
	// is longer, and the limit is what keeps an open document from being a free
	// memory-write primitive.
	docReadLimit = 512
)

// ConnectDocument upgrades to the per-document collaboration socket: presence
// ("who is in this document") and soft per-block locks ("who is typing where").
//
// Authorisation happens before the upgrade, against the document's workspace —
// a document is reachable by id alone, so the membership check is the only thing
// standing between a valid token and someone else's workspace.
//
// The socket carries no document body — only the news that it changed
// (docroom.TypeContentSaved), which the client answers with a normal GET. Edits
// themselves still go through PATCH /documents/:id/content and its updated_at
// guard; the announcement is what keeps two people editing different blocks from
// colliding on that guard, and what gets a colleague's text onto the other
// screen at all. Merging two edits to the *same* block is still out of scope:
// that block is held by one caret at a time (the locks below).
func (h *WSHandler) ConnectDocument(c *gin.Context) {
	uid, ok := h.authenticate(c)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		return
	}
	docID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	doc, err := h.q.GetDocument(c, docID)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if _, err := h.q.GetMembershipRole(c, db.GetMembershipRoleParams{
		WorkspaceID: doc.WorkspaceID, UserID: uid,
	}); err != nil {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "not a member of this workspace"})
		return
	}

	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return // Upgrade already wrote an error response.
	}
	p := docroom.NewParticipant(uid, h.displayName(c, uid))
	room := h.rooms.Join(doc.ID, p)
	go h.docWritePump(conn, p)
	go h.docReadPump(conn, p, room, doc.ID)
}

// displayName resolves what other people in the room see. Falls back to the
// email's local part, and then to "Участник": presence with a blank badge is
// worse than presence with an approximate name.
func (h *WSHandler) displayName(c *gin.Context, uid uuid.UUID) string {
	u, err := h.q.GetUserByID(c, uid)
	if err != nil {
		return "Участник"
	}
	if name := strings.TrimSpace(u.Name); name != "" {
		return name
	}
	if local, _, ok := strings.Cut(u.Email, "@"); ok && local != "" {
		return local
	}
	return "Участник"
}

// docReadPump handles lock/unlock requests until the socket dies, then takes the
// participant out of the room — which is what releases its locks immediately
// rather than at the TTL.
func (h *WSHandler) docReadPump(conn *websocket.Conn, p *docroom.Participant, room *docroom.Room, docID uuid.UUID) {
	defer func() {
		h.rooms.Leave(docID, p)
		_ = conn.Close()
	}()
	conn.SetReadLimit(docReadLimit)
	_ = conn.SetReadDeadline(time.Now().Add(docPongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(docPongWait))
	})
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var msg struct {
			Type    string `json:"type"`
			BlockID string `json:"block_id"`
		}
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue // malformed frame; not worth dropping the connection over
		}
		switch msg.Type {
		case docroom.TypeLock:
			room.Lock(p, msg.BlockID, time.Now())
		case docroom.TypeUnlock:
			room.Unlock(p, msg.BlockID, time.Now())
		}
	}
}

// docWritePump writes room snapshots and keeps the socket alive through proxies.
func (h *WSHandler) docWritePump(conn *websocket.Conn, p *docroom.Participant) {
	ticker := time.NewTicker(docPingEvery)
	defer func() {
		ticker.Stop()
		_ = conn.Close()
	}()
	for {
		select {
		case frame, ok := <-p.Out():
			_ = conn.SetWriteDeadline(time.Now().Add(docWriteWait))
			if !ok {
				// The room evicted us (slow client) or the process is shutting
				// down. A close frame makes the client reconnect and re-read the
				// room instead of sitting on a stale snapshot.
				_ = conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := conn.WriteMessage(websocket.TextMessage, frame); err != nil {
				return
			}
		case <-ticker.C:
			_ = conn.SetWriteDeadline(time.Now().Add(docWriteWait))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
