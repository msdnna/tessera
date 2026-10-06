package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/db"
	"tessera/middleware"
)

const (
	// maxBoardViews caps one user's saved layouts on one board, for the reason
	// maxDocTemplates caps the template gallery: the views are a flat dropdown a
	// person picks from, and a picker of a thousand entries is not a picker.
	// Nothing else bounded this table — the config is an opaque blob the frontend
	// owns, the name is half the unique key so a fresh one always inserts, and
	// there is no retention pass anywhere — so a member could keep POSTing
	// distinct names and grow the database until the disk ran out.
	maxBoardViews = 50

	// maxBoardViewName bounds the label, truncating rather than refusing — the
	// same treatment a document template's title gets, and for the same reason:
	// the name is a label in a dropdown, and a call that fails because a label was
	// long is worse than a label that got shortened.
	maxBoardViewName = 200

	// maxBoardViewConfigBytes bounds the opaque blob. snapshotToolbar() in
	// KanbanBoard sends a grouping mode, a handful of sort levels, the filter set
	// and one boolean per column — kilobytes at the very outside, so this leaves
	// an order of magnitude of slack for a shape the frontend owns and may grow.
	// Without it the only ceiling was the route's 1 MiB body budget, applied once
	// per save with nothing counting the saves.
	maxBoardViewConfigBytes = 64 << 10
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
// same-named one. Bounded three ways — how many views, how long a name, how big a
// config — see the constants above for why each one has to be here.
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
	name := truncateRunes(req.Name, maxBoardViewName)
	if strings.TrimSpace(name) == "" {
		// binding:"required" only rejects the empty string, and a view named out of
		// spaces is an entry in the dropdown nobody can tell apart from the next one.
		c.JSON(http.StatusBadRequest, gin.H{"error": "name must not be blank"})
		return
	}
	cfg := req.Config
	if len(cfg) == 0 {
		cfg = json.RawMessage("{}")
	}
	if len(cfg) > maxBoardViewConfigBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "view config is too large"})
		return
	}
	uid := middleware.CurrentUser(c)
	// Counted before the upsert, and only a *new* row is refused. The board
	// autosaves the loaded view on every toolbar change, so a user sitting at the
	// cap has to keep being able to overwrite what they already have — a cap that
	// froze their current layout would break the feature instead of bounding it.
	count, err := h.q.CountBoardViews(c, db.CountBoardViewsParams{BoardID: boardID, UserID: uid})
	if err != nil {
		fail(c, err)
		return
	}
	if count >= maxBoardViews {
		taken, err := h.q.BoardViewNameExists(c, db.BoardViewNameExistsParams{
			BoardID: boardID, UserID: uid, Name: name,
		})
		if err != nil {
			fail(c, err)
			return
		}
		if !taken {
			c.JSON(http.StatusConflict, gin.H{"error": "view limit reached"})
			return
		}
	}
	v, err := h.q.UpsertBoardView(c, db.UpsertBoardViewParams{
		BoardID: boardID, UserID: uid, Name: name, Config: cfg,
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
