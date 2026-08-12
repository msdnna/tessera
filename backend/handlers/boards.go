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
		fail(c, err)
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
	maxPos, err := h.q.MaxBoardPosition(c, projectID)
	if err != nil {
		fail(c, err)
		return
	}
	// Transactional: a board without its seeded columns (or without a done
	// column) can't hold a task — a partial create leaves an unusable board.
	slug := h.uniqueBoardSlug(c, projectID, req.Name)
	var b db.Board
	if err := h.inTx(c, func(q *db.Queries) error {
		var err error
		b, err = q.CreateBoard(c, db.CreateBoardParams{
			ProjectID: projectID, Name: req.Name, Slug: slug,
			Position: positionBetween(&maxPos, nil),
		})
		if err != nil {
			return err
		}
		// Seed the board with a default set of status columns, remembering the
		// last one so it can become the board's "done" column.
		var lastColID *uuid.UUID
		for i, dc := range defaultColumns {
			pos := float64(i+1) * positionGap
			col, err := q.CreateColumn(c, db.CreateColumnParams{
				BoardID: b.ID, Name: dc.name, Color: dc.color, Position: pos,
			})
			if err != nil {
				return err
			}
			id := col.ID
			lastColID = &id
		}
		// The rightmost seeded column ("Готово") closes tasks by default.
		if lastColID != nil {
			updated, err := q.SetBoardDoneColumn(c, db.SetBoardDoneColumnParams{
				ID: b.ID, DoneColumnID: lastColID,
			})
			if err != nil {
				return err
			}
			b = updated
		}
		return nil
	}); err != nil {
		fail(c, err)
		return
	}

	h.broadcast(p.WorkspaceID, "board.created", b)
	c.JSON(http.StatusCreated, b)
}

// doneColumnName is the default name for the task-completing column (used only
// when seeding a new board's columns; the live "done" column is tracked
// per-board via boards.done_column_id).
const doneColumnName = "Готово"

// defaultColumns are created for every new board.
var defaultColumns = []struct{ name, color string }{
	{"К работе", "#9aa0aa"},
	{"В процессе", "#2f80ed"},
	{"На рассмотрении", "#7c5cff"},
	{doneColumnName, "#18a058"},
}

// doneColumnID is the board's task-completing column, or nil when the board has
// none. There is deliberately no rightmost-column fallback: it made clearing the
// done column a no-op whenever that column was also the rightmost one (#2588).
// Legacy NULLs were pinned to their rightmost column by migration 0046.
func doneColumnID(board db.Board) *uuid.UUID { return board.DoneColumnID }

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
		fail(c, err)
		return
	}
	if !h.requireMember(c, p.WorkspaceID) {
		return
	}
	boards, err := h.q.ListBoards(c, projectID)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, boards)
}

// ResolveBoardBySlug resolves a /project/<projectSlug>/board/<boardSlug> pair to
// its board (board slugs are unique only within a project).
func (h *API) ResolveBoardBySlug(c *gin.Context) {
	proj, err := h.q.GetProjectBySlug(c, c.Query("project"))
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	if !h.requireMember(c, proj.WorkspaceID) {
		return
	}
	b, err := h.q.GetBoardInProjectBySlug(c, db.GetBoardInProjectBySlugParams{ProjectID: proj.ID, Slug: c.Query("board")})
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, b)
}

// GetBoard returns a single board by UUID or human-readable slug.
func (h *API) GetBoard(c *gin.Context) {
	// Accept either a UUID or a human-readable slug (/board/<slug> links).
	param := c.Param("id")
	var b db.Board
	var err error
	if id, perr := uuid.Parse(param); perr == nil {
		b, err = h.q.GetBoard(c, id)
	} else {
		b, err = h.q.GetBoardBySlug(c, param)
	}
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	wsID, err := h.q.WorkspaceIDForBoard(c, b.ID)
	if err != nil {
		fail(c, err)
		return
	}
	if !h.requireMember(c, wsID) {
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
		fail(c, err)
		return
	}
	if !h.requireMember(c, wsID) {
		return
	}
	b, err := h.q.GetBoard(c, id)
	if err != nil {
		fail(c, err)
		return
	}
	var req struct {
		Name     string  `json:"name" binding:"required"`
		Icon     *string `json:"icon"`
		Color    *string `json:"color"`
		IconMode *string `json:"icon_mode"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// Icon/colour/mode are optional (tri-state): absent keeps the current value so
	// a rename-only call (e.g. the sidebar) doesn't wipe the board's icon.
	icon, color, iconMode := b.Icon, b.Color, b.IconMode
	if req.Icon != nil {
		icon = *req.Icon
	}
	if req.Color != nil {
		color = *req.Color
	}
	if req.IconMode != nil {
		iconMode = normIconMode(*req.IconMode)
	}
	updated, err := h.q.UpdateBoard(c, db.UpdateBoardParams{
		ID: id, Name: req.Name, Position: b.Position, Icon: icon, Color: color, IconMode: iconMode,
	})
	if err != nil {
		fail(c, err)
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
		fail(c, err)
		return
	}
	if !h.requireMember(c, wsID) {
		return
	}
	if err := h.q.DeleteBoard(c, id); err != nil {
		fail(c, err)
		return
	}
	h.broadcast(wsID, "board.deleted", gin.H{"id": id})
	c.Status(http.StatusNoContent)
}

// SetDoneColumn configures which column auto-completes tasks (or clears it,
// reverting to the rightmost-column fallback, when column_id is null).
func (h *API) SetDoneColumn(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	wsID, err := h.q.WorkspaceIDForBoard(c, id)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	if !h.requireMember(c, wsID) {
		return
	}
	var req struct {
		ColumnID *uuid.UUID `json:"column_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// A non-null target must be a column on this board.
	if req.ColumnID != nil {
		col, err := h.q.GetColumn(c, *req.ColumnID)
		if err != nil || col.BoardID != id {
			c.JSON(http.StatusBadRequest, gin.H{"error": "column does not belong to this board"})
			return
		}
	}
	updated, err := h.q.SetBoardDoneColumn(c, db.SetBoardDoneColumnParams{ID: id, DoneColumnID: req.ColumnID})
	if err != nil {
		fail(c, err)
		return
	}
	h.broadcast(wsID, "board.updated", updated)
	c.JSON(http.StatusOK, updated)
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
		fail(c, err)
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
	maxPos, err := h.q.MaxColumnPosition(c, boardID)
	if err != nil {
		fail(c, err)
		return
	}
	col, err := h.q.CreateColumn(c, db.CreateColumnParams{
		BoardID: boardID, Name: req.Name, Color: req.Color, Position: positionBetween(&maxPos, nil),
	})
	if err != nil {
		fail(c, err)
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
		fail(c, err)
		return
	}
	if !h.requireMember(c, wsID) {
		return
	}
	cols, err := h.q.ListColumns(c, boardID)
	if err != nil {
		fail(c, err)
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
		fail(c, err)
		return
	}
	if !h.requireMember(c, wsID) {
		return
	}
	col, err := h.q.GetColumn(c, id)
	if err != nil {
		fail(c, err)
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
		fail(c, err)
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
		fail(c, err)
		return
	}
	if !h.requireMember(c, wsID) {
		return
	}
	col, err := h.q.GetColumn(c, id)
	if err != nil {
		fail(c, err)
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
		fail(c, err)
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
		fail(c, err)
		return
	}
	if !h.requireMember(c, wsID) {
		return
	}
	if err := h.q.DeleteColumn(c, id); err != nil {
		fail(c, err)
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
