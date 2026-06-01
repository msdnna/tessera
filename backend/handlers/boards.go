package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/db"
)

// ── Boards ─────────────────────────────────────────────────

// CreateBoard adds a board to a project.
func (h *API) CreateBoard(c *gin.Context) {
	projectID, ok := parseID(c, "id")
	if !ok {
		return
	}
	p, err := h.q.GetProject(c, projectID)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c)
		return
	}
	if !h.requireMember(c, p.WorkspaceID) {
		return
	}
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	max, err := h.q.MaxBoardPosition(c, projectID)
	if err != nil {
		fail(c)
		return
	}
	b, err := h.q.CreateBoard(c, db.CreateBoardParams{
		ProjectID: projectID, Name: req.Name, Position: positionBetween(&max, nil),
	})
	if err != nil {
		fail(c)
		return
	}
	h.broadcast(p.WorkspaceID, "board.created", b)
	c.JSON(http.StatusCreated, b)
}

// ListBoards lists a project's boards.
func (h *API) ListBoards(c *gin.Context) {
	projectID, ok := parseID(c, "id")
	if !ok {
		return
	}
	p, err := h.q.GetProject(c, projectID)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c)
		return
	}
	if !h.requireMember(c, p.WorkspaceID) {
		return
	}
	boards, err := h.q.ListBoards(c, projectID)
	if err != nil {
		fail(c)
		return
	}
	c.JSON(http.StatusOK, boards)
}

// GetBoard returns a single board.
func (h *API) GetBoard(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	wsID, err := h.q.WorkspaceIDForBoard(c, id)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c)
		return
	}
	if !h.requireMember(c, wsID) {
		return
	}
	b, err := h.q.GetBoard(c, id)
	if err != nil {
		fail(c)
		return
	}
	c.JSON(http.StatusOK, b)
}

// UpdateBoard renames a board.
func (h *API) UpdateBoard(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	wsID, err := h.q.WorkspaceIDForBoard(c, id)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c)
		return
	}
	if !h.requireMember(c, wsID) {
		return
	}
	b, err := h.q.GetBoard(c, id)
	if err != nil {
		fail(c)
		return
	}
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	updated, err := h.q.UpdateBoard(c, db.UpdateBoardParams{ID: id, Name: req.Name, Position: b.Position})
	if err != nil {
		fail(c)
		return
	}
	h.broadcast(wsID, "board.updated", updated)
	c.JSON(http.StatusOK, updated)
}

// DeleteBoard removes a board and its columns/tasks.
func (h *API) DeleteBoard(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	wsID, err := h.q.WorkspaceIDForBoard(c, id)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c)
		return
	}
	if !h.requireMember(c, wsID) {
		return
	}
	if err := h.q.DeleteBoard(c, id); err != nil {
		fail(c)
		return
	}
	h.broadcast(wsID, "board.deleted", gin.H{"id": id})
	c.Status(http.StatusNoContent)
}

// ── Columns ────────────────────────────────────────────────

// CreateColumn adds a column (status) to a board.
func (h *API) CreateColumn(c *gin.Context) {
	boardID, ok := parseID(c, "id")
	if !ok {
		return
	}
	wsID, err := h.q.WorkspaceIDForBoard(c, boardID)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c)
		return
	}
	if !h.requireMember(c, wsID) {
		return
	}
	var req struct {
		Name  string `json:"name" binding:"required"`
		Color string `json:"color"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	max, err := h.q.MaxColumnPosition(c, boardID)
	if err != nil {
		fail(c)
		return
	}
	col, err := h.q.CreateColumn(c, db.CreateColumnParams{
		BoardID: boardID, Name: req.Name, Color: req.Color, Position: positionBetween(&max, nil),
	})
	if err != nil {
		fail(c)
		return
	}
	h.broadcast(wsID, "column.created", col)
	c.JSON(http.StatusCreated, col)
}

// ListColumns lists a board's columns.
func (h *API) ListColumns(c *gin.Context) {
	boardID, ok := parseID(c, "id")
	if !ok {
		return
	}
	wsID, err := h.q.WorkspaceIDForBoard(c, boardID)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c)
		return
	}
	if !h.requireMember(c, wsID) {
		return
	}
	cols, err := h.q.ListColumns(c, boardID)
	if err != nil {
		fail(c)
		return
	}
	c.JSON(http.StatusOK, cols)
}

// UpdateColumn edits a column's name/color.
func (h *API) UpdateColumn(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	wsID, err := h.q.WorkspaceIDForColumn(c, id)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c)
		return
	}
	if !h.requireMember(c, wsID) {
		return
	}
	col, err := h.q.GetColumn(c, id)
	if err != nil {
		fail(c)
		return
	}
	var req struct {
		Name  string `json:"name" binding:"required"`
		Color string `json:"color"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	updated, err := h.q.UpdateColumn(c, db.UpdateColumnParams{
		ID: id, Name: req.Name, Color: req.Color, Position: col.Position,
	})
	if err != nil {
		fail(c)
		return
	}
	h.broadcast(wsID, "column.updated", updated)
	c.JSON(http.StatusOK, updated)
}

// MoveColumn repositions a column between two neighbours (server computes the
// midpoint position from before_id / after_id).
func (h *API) MoveColumn(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	wsID, err := h.q.WorkspaceIDForColumn(c, id)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c)
		return
	}
	if !h.requireMember(c, wsID) {
		return
	}
	col, err := h.q.GetColumn(c, id)
	if err != nil {
		fail(c)
		return
	}
	var req struct {
		BeforeID *uuid.UUID `json:"before_id"`
		AfterID  *uuid.UUID `json:"after_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	prev, next, ok := h.neighborColumnPositions(c, req.BeforeID, req.AfterID)
	if !ok {
		return
	}
	updated, err := h.q.UpdateColumn(c, db.UpdateColumnParams{
		ID: id, Name: col.Name, Color: col.Color, Position: positionBetween(prev, next),
	})
	if err != nil {
		fail(c)
		return
	}
	h.broadcast(wsID, "column.moved", updated)
	c.JSON(http.StatusOK, updated)
}

// DeleteColumn removes a column and its tasks.
func (h *API) DeleteColumn(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	wsID, err := h.q.WorkspaceIDForColumn(c, id)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c)
		return
	}
	if !h.requireMember(c, wsID) {
		return
	}
	if err := h.q.DeleteColumn(c, id); err != nil {
		fail(c)
		return
	}
	h.broadcast(wsID, "column.deleted", gin.H{"id": id})
	c.Status(http.StatusNoContent)
}

// neighborColumnPositions resolves the positions of the before/after columns.
// Returns (nil, nil) bounds when an id is absent; writes 400 and returns ok=false
// if a referenced column can't be loaded.
func (h *API) neighborColumnPositions(c *gin.Context, beforeID, afterID *uuid.UUID) (prev, next *float64, ok bool) {
	if beforeID != nil {
		col, err := h.q.GetColumn(c, *beforeID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid before_id"})
			return nil, nil, false
		}
		prev = &col.Position
	}
	if afterID != nil {
		col, err := h.q.GetColumn(c, *afterID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid after_id"})
			return nil, nil, false
		}
		next = &col.Position
	}
	return prev, next, true
}
