package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/db"
	"tessera/middleware"
)

// boardViewView is the JSON shape returned to the client. config is surfaced as
// raw JSON (not base64-encoded bytes) since it's an opaque frontend blob.
type boardViewView struct {
	ID        uuid.UUID       `json:"id"`
	BoardID   uuid.UUID       `json:"board_id"`
	Name      string          `json:"name"`
	Config    json.RawMessage `json:"config"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

func toBoardViewView(v db.BoardView) boardViewView {
	cfg := json.RawMessage(v.Config)
	if len(cfg) == 0 {
		cfg = json.RawMessage("{}")
	}
	return boardViewView{
		ID: v.ID, BoardID: v.BoardID, Name: v.Name, Config: cfg,
		CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt,
	}
}

// ListBoardViews returns the current user's saved views for a board.
func (h *API) ListBoardViews(c *gin.Context) {
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
	rows, err := h.q.ListBoardViews(c, db.ListBoardViewsParams{BoardID: boardID, UserID: middleware.CurrentUser(c)})
	if err != nil {
		fail(c, err)
		return
	}
	out := make([]boardViewView, 0, len(rows))
	for _, v := range rows {
		out = append(out, toBoardViewView(v))
	}
	c.JSON(http.StatusOK, out)
}

// SaveBoardView creates the current user's view for a board, or overwrites the
// same-named one.
func (h *API) SaveBoardView(c *gin.Context) {
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
		Name   string          `json:"name" binding:"required"`
		Config json.RawMessage `json:"config"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	cfg := req.Config
	if len(cfg) == 0 {
		cfg = json.RawMessage("{}")
	}
	v, err := h.q.UpsertBoardView(c, db.UpsertBoardViewParams{
		BoardID: boardID, UserID: middleware.CurrentUser(c), Name: req.Name, Config: cfg,
	})
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, toBoardViewView(v))
}

// DeleteBoardView removes one of the current user's saved views.
func (h *API) DeleteBoardView(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	v, err := h.q.GetBoardView(c, id)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	if v.UserID != middleware.CurrentUser(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "not your view"})
		return
	}
	if err := h.q.DeleteBoardView(c, id); err != nil {
		fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
