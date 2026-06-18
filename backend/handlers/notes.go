package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/db"
	"tessera/middleware"
)

// CreateNote adds a note to a workspace.
func (h *API) CreateNote(c *gin.Context) {
	wsID, ok := parseID(c, "id")
	if !ok || !h.requireMember(c, wsID) {
		return
	}
	var req struct {
		Title     string     `json:"title" binding:"required"`
		Body      string     `json:"body"`
		ProjectID *uuid.UUID `json:"project_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	uid := middleware.CurrentUser(c)
	note, err := h.q.CreateNote(c, db.CreateNoteParams{
		WorkspaceID: wsID,
		ProjectID:   req.ProjectID,
		AuthorID:    &uid,
		Title:       req.Title,
		Body:        req.Body,
		Slug:        h.uniqueNoteSlug(c, wsID, req.Title),
	})
	if err != nil {
		fail(c)
		return
	}
	h.broadcast(wsID, "note.created", note)
	c.JSON(http.StatusCreated, note)
}

// ListNotes lists a workspace's notes (most recently updated first).
func (h *API) ListNotes(c *gin.Context) {
	wsID, ok := parseID(c, "id")
	if !ok || !h.requireMember(c, wsID) {
		return
	}
	notes, err := h.q.ListNotes(c, wsID)
	if err != nil {
		fail(c)
		return
	}
	c.JSON(http.StatusOK, notes)
}

// GetNote returns a single note.
func (h *API) GetNote(c *gin.Context) {
	note, ok := h.loadNote(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, note)
}

// UpdateNote edits a note's title/body.
func (h *API) UpdateNote(c *gin.Context) {
	note, ok := h.loadNote(c)
	if !ok {
		return
	}
	var req struct {
		Title string `json:"title" binding:"required"`
		Body  string `json:"body"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	updated, err := h.q.UpdateNote(c, db.UpdateNoteParams{ID: note.ID, Title: req.Title, Body: req.Body})
	if err != nil {
		fail(c)
		return
	}
	h.broadcast(note.WorkspaceID, "note.updated", updated)
	c.JSON(http.StatusOK, updated)
}

// DeleteNote removes a note.
func (h *API) DeleteNote(c *gin.Context) {
	note, ok := h.loadNote(c)
	if !ok {
		return
	}
	if err := h.q.DeleteNote(c, note.ID); err != nil {
		fail(c)
		return
	}
	h.broadcast(note.WorkspaceID, "note.deleted", gin.H{"id": note.ID})
	c.Status(http.StatusNoContent)
}

// loadNote fetches a note (path param :id) and authorizes via its workspace.
func (h *API) loadNote(c *gin.Context) (db.Note, bool) {
	id, ok := parseID(c, "id")
	if !ok {
		return db.Note{}, false
	}
	note, err := h.q.GetNote(c, id)
	if notFound(c, err) {
		return db.Note{}, false
	}
	if err != nil {
		fail(c)
		return db.Note{}, false
	}
	if !h.requireMember(c, note.WorkspaceID) {
		return db.Note{}, false
	}
	return note, true
}
