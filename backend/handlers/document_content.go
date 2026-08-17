package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"tessera/internal/db"
	"tessera/internal/docroom"
	"tessera/middleware"
)

// UpdateDocumentContent saves the document body.
//
// Content has its own endpoint rather than riding along in the metadata PATCH:
// the list query and the realtime payload both deliberately exclude content
// (D1), and folding it into the same handler is how that separation quietly
// erodes.
//
// The write is guarded by the updated_at the client last saw. Autosave means a
// document can be open in two places at once, and without the guard the last
// debounce to arrive silently overwrites whatever the other side wrote.
// Merging concurrent edits per block is D4; refusing to lose them is this
// endpoint's job.
func (h *API) UpdateDocumentContent(c *gin.Context) {
	doc, ok := h.loadDocument(c)
	if !ok {
		return
	}
	var req struct {
		Content   json.RawMessage `json:"content" binding:"required"`
		UpdatedAt time.Time       `json:"updated_at" binding:"required"`
		// The document socket connection this save came from, so the announcement
		// below skips its own author. Optional: Android, the MCP tools and curl
		// have no socket, and an empty value correctly announces to everyone.
		ConnID string `json:"conn_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	clean, preview, err := validateDocContent(req.Content)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	updated, err := h.q.UpdateDocumentContent(c, db.UpdateDocumentContentParams{
		ID:        doc.ID,
		Content:   clean,
		Preview:   preview,
		UpdatedAt: req.UpdatedAt,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// The row exists (loadDocument just read it), so a miss here means the
		// timestamp moved: someone else saved in between.
		c.JSON(http.StatusConflict, gin.H{
			"error":      "document changed elsewhere",
			"updated_at": doc.UpdatedAt,
		})
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	// Journal the save (#2731). Deliberately after the write and deliberately
	// not fatal: the text is already safe in documents.content, and answering
	// 500 to a successful save because its history entry failed would make the
	// editor look broken and push the client into retrying a write it does not
	// need to repeat.
	if err := h.snapshotDocument(c, doc, updated, middleware.CurrentUser(c)); err != nil {
		log.Printf("documents: version snapshot for %s failed: %v", updated.ID, err)
	}
	h.broadcast(doc.WorkspaceID, "document.updated", documentMeta(updated))
	// Then tell the people who have this document *open*. The broadcast above goes
	// to the workspace hub and drives the list; it deliberately carries no content
	// and nobody in the editor listens to it, which is exactly why a colleague's
	// text used to never arrive and their own next save met a 409.
	h.notifyDocSaved(doc.ID, req.ConnID, middleware.CurrentUser(c), updated.UpdatedAt)
	c.JSON(http.StatusOK, gin.H{
		"id":         updated.ID,
		"updated_at": updated.UpdatedAt,
		"preview":    updated.Preview,
	})
}

// notifyDocSaved announces a save to everyone in the document's room but the
// connection that made it.
//
// An unparseable conn_id is treated as "no connection" rather than as an error:
// the worst outcome is that the sender gets its own nudge and refetches a
// document it already has, which costs one request. Refusing the save over it
// would cost the user their text.
func (h *API) notifyDocSaved(docID uuid.UUID, connID string, userID uuid.UUID, at time.Time) {
	if h.docRooms == nil {
		return
	}
	except, err := uuid.Parse(connID)
	if err != nil {
		except = uuid.Nil
	}
	h.docRooms.Send(docID, except, docroom.ContentSavedMsg{
		Type:      docroom.TypeContentSaved,
		UpdatedAt: at,
		ByConn:    connID,
		ByUser:    userID.String(),
	})
}
