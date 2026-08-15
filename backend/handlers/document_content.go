package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"tessera/internal/db"
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
	c.JSON(http.StatusOK, gin.H{
		"id":         updated.ID,
		"updated_at": updated.UpdatedAt,
		"preview":    updated.Preview,
	})
}
