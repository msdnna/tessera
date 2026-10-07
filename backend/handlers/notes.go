package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/db"
	"tessera/middleware"
)

// Note visibility (0081, #3037). A private note is its author's alone: no other
// member reads it in a list, a search, a task's section or a socket event, and
// asking for it by id answers 404 rather than 403, so its existence does not
// leak either. Notes made before 0081 were shared with the workspace and stay so.
const (
	noteShared  = "workspace"
	notePrivate = "private"
)

func validNoteVisibility(v string) bool { return v == noteShared || v == notePrivate }

// readTriStateID reads a nullable id that a PATCH may leave out: absent (nil
// raw) keeps the current value, `null` clears it, a string sets it.
func readTriStateID(raw json.RawMessage, cur *uuid.UUID) (*uuid.UUID, bool, error) {
	if raw == nil {
		return cur, false, nil
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, true, nil
	}
	var id uuid.UUID
	if err := json.Unmarshal(raw, &id); err != nil {
		return nil, false, err
	}
	return &id, true, nil
}

// broadcastNote sends a note event to the people allowed to read the note: the
// whole workspace for a shared one, the author alone for a private one.
func (h *API) broadcastNote(n db.Note, eventType string, payload any) {
	if n.Visibility == notePrivate {
		if n.AuthorID != nil {
			h.broadcastToUser(n.WorkspaceID, *n.AuthorID, eventType, payload)
		}
		return
	}
	h.broadcast(n.WorkspaceID, eventType, payload)
}

// CreateNote adds a note to a workspace. `visibility` defaults to shared — the
// behaviour every client had before 0081; the v2 capture asks for private.
func (h *API) CreateNote(c *gin.Context) {
	wsID, ok := parseID(c, "id")
	if !ok || !h.requireMember(c, wsID) {
		return
	}
	var req struct {
		Title      string     `json:"title" binding:"required"`
		Body       string     `json:"body"`
		ProjectID  *uuid.UUID `json:"project_id"`
		TaskID     *uuid.UUID `json:"task_id"`
		Visibility *string    `json:"visibility"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	visibility := noteShared
	if req.Visibility != nil {
		if !validNoteVisibility(*req.Visibility) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unknown visibility"})
			return
		}
		visibility = *req.Visibility
	}
	// The project arrives in the body, so the route's requireMember vouches for
	// the workspace and nothing vouches for the project. Unchecked, the note is
	// one DELETE away from being gone: notes.project_id is ON DELETE CASCADE.
	if req.ProjectID != nil && !h.projectInWorkspace(c, *req.ProjectID, wsID) {
		return
	}
	if req.TaskID != nil && !h.taskInWorkspace(c, *req.TaskID, wsID) {
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
		TaskID:      req.TaskID,
		Visibility:  visibility,
	})
	if err != nil {
		fail(c, err)
		return
	}
	h.broadcastNote(note, "note.created", note)
	c.JSON(http.StatusCreated, note)
}

// ListNotes lists the workspace's notes the caller may read (most recently
// updated first): the shared ones and the caller's own private ones.
func (h *API) ListNotes(c *gin.Context) {
	wsID, ok := parseID(c, "id")
	if !ok || !h.requireMember(c, wsID) {
		return
	}
	notes, err := h.q.ListNotes(c, db.ListNotesParams{
		WorkspaceID: wsID, Viewer: middleware.CurrentUser(c),
	})
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, orEmpty(notes))
}

// GetNote returns a single note.
func (h *API) GetNote(c *gin.Context) {
	note, ok := h.loadNote(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, note)
}

// UpdateNote edits a note. title/body are replaced as before (every client
// sends both); task_id and visibility are optional — absent keeps the current
// value, task_id null unlinks. Only the author may change who sees the note.
func (h *API) UpdateNote(c *gin.Context) {
	note, ok := h.loadNote(c)
	if !ok {
		return
	}
	var req struct {
		Title      string          `json:"title" binding:"required"`
		Body       string          `json:"body"`
		TaskID     json.RawMessage `json:"task_id"`
		Visibility *string         `json:"visibility"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	taskID, taskSet, err := readTriStateID(req.TaskID, note.TaskID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task_id"})
		return
	}
	if taskSet && taskID != nil && !h.taskInWorkspace(c, *taskID, note.WorkspaceID) {
		return
	}
	visibility := note.Visibility
	if req.Visibility != nil && *req.Visibility != note.Visibility {
		if !validNoteVisibility(*req.Visibility) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unknown visibility"})
			return
		}
		if note.AuthorID == nil || *note.AuthorID != middleware.CurrentUser(c) {
			c.JSON(http.StatusForbidden, gin.H{"error": "only the author changes who sees a note"})
			return
		}
		visibility = *req.Visibility
	}
	updated, err := h.q.UpdateNote(c, db.UpdateNoteParams{
		ID: note.ID, Title: req.Title, Body: req.Body, TaskID: taskID, Visibility: visibility,
	})
	if err != nil {
		fail(c, err)
		return
	}
	// Going private takes the note away from everyone else: they get a delete,
	// and the author (who received that delete too) the updated note after it.
	if note.Visibility == noteShared && updated.Visibility == notePrivate {
		h.broadcast(note.WorkspaceID, "note.deleted", gin.H{"id": note.ID})
	}
	h.broadcastNote(updated, "note.updated", updated)
	c.JSON(http.StatusOK, updated)
}

// DeleteNote removes a note.
func (h *API) DeleteNote(c *gin.Context) {
	note, ok := h.loadNote(c)
	if !ok {
		return
	}
	if err := h.q.DeleteNote(c, note.ID); err != nil {
		fail(c, err)
		return
	}
	h.broadcastNote(note, "note.deleted", gin.H{"id": note.ID})
	c.Status(http.StatusNoContent)
}

// ListTaskNotes lists the notes tied to a task that the caller may read (the
// «Напоминания и заметки» section of the task modal, #2998). Only notes of the
// task's own workspace: a task that moved away keeps its old notes' task_id,
// and those notes stay with the workspace they were written in.
func (h *API) ListTaskNotes(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	t, wsID, ok := h.loadTask(c, id)
	if !ok {
		return
	}
	notes, err := h.q.ListTaskNotes(c, db.ListTaskNotesParams{
		TaskID: &t.ID, Viewer: middleware.CurrentUser(c),
	})
	if err != nil {
		fail(c, err)
		return
	}
	out := make([]db.Note, 0, len(notes))
	for _, n := range notes {
		if n.WorkspaceID == wsID {
			out = append(out, n)
		}
	}
	c.JSON(http.StatusOK, out)
}

// loadNote fetches a note (path param :id) and authorizes via its workspace. A
// private note of somebody else is a 404 — the same answer as no note at all.
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
		fail(c, err)
		return db.Note{}, false
	}
	if !h.requireMember(c, note.WorkspaceID) {
		return db.Note{}, false
	}
	if note.Visibility == notePrivate &&
		(note.AuthorID == nil || *note.AuthorID != middleware.CurrentUser(c)) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return db.Note{}, false
	}
	return note, true
}
