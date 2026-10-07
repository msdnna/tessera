package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/db"
	"tessera/middleware"
)

const (
	// maxBoardViews caps one user's saved layouts on one board — and, separately,
	// a board's shared list — for the reason maxDocTemplates caps the template
	// gallery: the views are a flat list a person picks from, and a picker of a
	// thousand entries is not a picker. Nothing else bounded this table — the
	// config is an opaque blob the frontend owns, the name is half the unique key
	// so a fresh one always inserts, and there is no retention pass anywhere — so
	// a member could keep POSTing distinct names and grow the database until the
	// disk ran out.
	maxBoardViews = 50

	// maxBoardViewName bounds the label, truncating rather than refusing — the
	// same treatment a document template's title gets, and for the same reason:
	// the name is a label in a list, and a call that fails because a label was
	// long is worse than a label that got shortened.
	maxBoardViewName = 200

	// maxBoardViewConfigBytes bounds the opaque blob. The board's view config is a
	// grouping mode, a handful of sort levels, the filter set, the card settings
	// and one boolean per column — kilobytes at the very outside, so this leaves
	// an order of magnitude of slack for a shape the frontend owns and may grow.
	maxBoardViewConfigBytes = 64 << 10

	// Board view scopes (#2988): one person's, or the whole board's.
	viewScopeUser  = "user"
	viewScopeBoard = "board"

	// evBoardViews tells the board's readers that its shared list changed.
	// Personal views are nobody else's business and are not announced.
	evBoardViews = "board_views.changed"
)

var errViewNameTaken = errors.New("view name taken")

// boardViewView is the JSON shape returned to the client. config is surfaced as
// raw JSON (not base64-encoded bytes) since it's an opaque frontend blob.
//
// `editable` is decided for the caller: their own views always, the shared ones
// only for a workspace owner/admin — everybody else sees those with a lock.
type boardViewView struct {
	ID          uuid.UUID       `json:"id"`
	BoardID     uuid.UUID       `json:"board_id"`
	Name        string          `json:"name"`
	Config      json.RawMessage `json:"config"`
	Scope       string          `json:"scope"`
	IsDefault   bool            `json:"is_default"`
	Position    float64         `json:"position"`
	CreatedBy   *uuid.UUID      `json:"created_by"`
	CreatorName string          `json:"creator_name"`
	Editable    bool            `json:"editable"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

func rawConfig(b []byte) json.RawMessage {
	if len(b) == 0 {
		return json.RawMessage("{}")
	}
	return json.RawMessage(b)
}

func toBoardViewView(v db.BoardView, creator string, editable bool) boardViewView {
	return boardViewView{
		ID: v.ID, BoardID: v.BoardID, Name: v.Name, Config: rawConfig(v.Config),
		Scope: v.Scope, IsDefault: v.IsDefault, Position: v.Position,
		CreatedBy: v.CreatedBy, CreatorName: creator, Editable: editable,
		CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt,
	}
}

// isWorkspaceManager is requireManager without the 403: a question, not a gate.
func (h *API) isWorkspaceManager(c *gin.Context, wsID uuid.UUID) bool {
	switch h.memberRole(c, wsID) {
	case "owner", "admin":
		return true
	}
	return false
}

// boardViewWorkspace resolves the board's workspace and checks membership.
func (h *API) boardViewWorkspace(c *gin.Context, boardID uuid.UUID) (uuid.UUID, bool) {
	wsID, err := h.q.WorkspaceIDForBoard(c, boardID)
	if notFound(c, err) {
		return uuid.Nil, false
	}
	if err != nil {
		fail(c, err)
		return uuid.Nil, false
	}
	if !h.requireMember(c, wsID) {
		return uuid.Nil, false
	}
	return wsID, true
}

// canEditBoardView: a personal view is its owner's alone, a shared one is the
// workspace owner's/admins'.
func (h *API) canEditBoardView(c *gin.Context, v db.BoardView, wsID uuid.UUID) bool {
	if v.Scope == viewScopeBoard {
		return h.isWorkspaceManager(c, wsID)
	}
	return v.UserID != nil && *v.UserID == middleware.CurrentUser(c)
}

// ListBoardViews returns the caller's own views for a board followed by the
// board's shared ones.
func (h *API) ListBoardViews(c *gin.Context) {
	boardID, ok := parseID(c, "id")
	if !ok {
		return
	}
	wsID, ok := h.boardViewWorkspace(c, boardID)
	if !ok {
		return
	}
	uid := middleware.CurrentUser(c)
	rows, err := h.q.ListBoardViewsForUser(c, db.ListBoardViewsForUserParams{BoardID: boardID, UserID: &uid})
	if err != nil {
		fail(c, err)
		return
	}
	manager := h.isWorkspaceManager(c, wsID)
	out := make([]boardViewView, 0, len(rows))
	for _, r := range rows {
		v := db.BoardView{
			ID: r.ID, BoardID: r.BoardID, UserID: r.UserID, Name: r.Name, Config: r.Config,
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Scope: r.Scope, IsDefault: r.IsDefault,
			Position: r.Position, CreatedBy: r.CreatedBy,
		}
		editable := r.Scope == viewScopeUser || manager
		out = append(out, toBoardViewView(v, r.CreatorName, editable))
	}
	c.JSON(http.StatusOK, out)
}

// boardViewName trims and bounds a name; "" means it was blank.
func boardViewName(raw string) string {
	name := truncateRunes(raw, maxBoardViewName)
	if strings.TrimSpace(name) == "" {
		return ""
	}
	return name
}

// SaveBoardView creates a view, or overwrites the same-named one in the same
// list. `scope` defaults to "user" — the request the old toolbar and the Android
// client make, unchanged. A shared view ("board") is created by a workspace
// owner/admin. Bounded three ways — how many views, how long a name, how big a
// config — see the constants above for why each one has to be here.
func (h *API) SaveBoardView(c *gin.Context) {
	boardID, ok := parseID(c, "id")
	if !ok {
		return
	}
	wsID, ok := h.boardViewWorkspace(c, boardID)
	if !ok {
		return
	}
	var req struct {
		Name      string          `json:"name" binding:"required"`
		Config    json.RawMessage `json:"config"`
		Scope     string          `json:"scope"`
		IsDefault bool            `json:"is_default"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	name := boardViewName(req.Name)
	if name == "" {
		// binding:"required" only rejects the empty string, and a view named out of
		// spaces is an entry in the list nobody can tell apart from the next one.
		c.JSON(http.StatusBadRequest, gin.H{"error": "name must not be blank"})
		return
	}
	scope := req.Scope
	if scope == "" {
		scope = viewScopeUser
	}
	if scope != viewScopeUser && scope != viewScopeBoard {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scope must be user or board"})
		return
	}
	if scope == viewScopeBoard && !h.requireManager(c, wsID) {
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

	var v db.BoardView
	err := h.inTx(c, func(q *db.Queries) error {
		var (
			count int64
			err   error
		)
		// Counted before the upsert, and only a *new* row is refused: a user sitting
		// at the cap has to keep being able to overwrite what they already have — a
		// cap that froze their current layout would break the feature instead of
		// bounding it.
		if scope == viewScopeBoard {
			count, err = q.CountSharedBoardViews(c, boardID)
		} else {
			count, err = q.CountBoardViews(c, db.CountBoardViewsParams{BoardID: boardID, UserID: &uid})
		}
		if err != nil {
			return err
		}
		if count >= maxBoardViews {
			var taken bool
			if scope == viewScopeBoard {
				taken, err = q.SharedBoardViewNameExists(c, db.SharedBoardViewNameExistsParams{BoardID: boardID, Name: name})
			} else {
				taken, err = q.BoardViewNameExists(c, db.BoardViewNameExistsParams{BoardID: boardID, UserID: &uid, Name: name})
			}
			if err != nil {
				return err
			}
			if !taken {
				return errViewLimit
			}
		}
		if scope == viewScopeBoard {
			end, err := q.MaxSharedBoardViewPosition(c, boardID)
			if err != nil {
				return err
			}
			v, err = q.UpsertSharedBoardView(c, db.UpsertSharedBoardViewParams{
				BoardID: boardID, Name: name, Config: cfg, Position: end + positionGap, CreatedBy: &uid,
			})
			if err != nil {
				return err
			}
		} else {
			end, err := q.MaxUserBoardViewPosition(c, db.MaxUserBoardViewPositionParams{BoardID: boardID, UserID: &uid})
			if err != nil {
				return err
			}
			v, err = q.UpsertBoardView(c, db.UpsertBoardViewParams{
				BoardID: boardID, UserID: &uid, Name: name, Config: cfg, Position: end + positionGap,
			})
			if err != nil {
				return err
			}
		}
		if req.IsDefault {
			v, err = setBoardViewDefault(c, q, v, true)
		}
		return err
	})
	if errors.Is(err, errViewLimit) {
		c.JSON(http.StatusConflict, gin.H{"error": "view limit reached"})
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	if v.Scope == viewScopeBoard {
		h.broadcastAs(c, wsID, evBoardViews, gin.H{"board_id": boardID})
	}
	c.JSON(http.StatusOK, toBoardViewView(v, h.userName(c, uid), true))
}

var errViewLimit = errors.New("view limit reached")

// setBoardViewDefault stars (or un-stars) a view, clearing the star of its
// list's previous default first — one per list, which a partial unique index
// also enforces.
func setBoardViewDefault(c *gin.Context, q *db.Queries, v db.BoardView, on bool) (db.BoardView, error) {
	if on {
		var err error
		if v.Scope == viewScopeBoard {
			err = q.ClearSharedBoardViewDefault(c, v.BoardID)
		} else {
			err = q.ClearUserBoardViewDefault(c, db.ClearUserBoardViewDefaultParams{BoardID: v.BoardID, UserID: v.UserID})
		}
		if err != nil {
			return v, err
		}
	}
	return q.UpdateBoardView(c, db.UpdateBoardViewParams{
		ID: v.ID, Name: v.Name, Config: v.Config, Scope: v.Scope, UserID: v.UserID,
		Position: v.Position, IsDefault: on,
	})
}

// UpdateBoardView is the partial edit of one view (#2987, #3015, #3016): rename,
// re-save the config, star it, move it in its list (before_id / after_id — the
// neighbours it lands between, like every other move here) or make it shared /
// personal. Absent fields keep their value.
//
// Rights: a personal view is its owner's; a shared one is the workspace
// owner's/admins'. Sharing a personal view needs both (it is yours, and the
// shared list is the managers'); taking a shared view back makes it the
// caller's own.
func (h *API) UpdateBoardView(c *gin.Context) {
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
	wsID, ok := h.boardViewWorkspace(c, v.BoardID)
	if !ok {
		return
	}
	if !h.canEditBoardView(c, v, wsID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "not your view"})
		return
	}
	var req struct {
		Name      *string          `json:"name"`
		Config    *json.RawMessage `json:"config"`
		Scope     *string          `json:"scope"`
		IsDefault *bool            `json:"is_default"`
		BeforeID  *uuid.UUID       `json:"before_id"`
		AfterID   *uuid.UUID       `json:"after_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	uid := middleware.CurrentUser(c)
	wasShared := v.Scope == viewScopeBoard
	next := v
	if req.Name != nil {
		name := boardViewName(*req.Name)
		if name == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "name must not be blank"})
			return
		}
		next.Name = name
	}
	if req.Config != nil {
		cfg := *req.Config
		if len(cfg) == 0 {
			cfg = json.RawMessage("{}")
		}
		if len(cfg) > maxBoardViewConfigBytes {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "view config is too large"})
			return
		}
		next.Config = cfg
	}
	moved := false
	if req.Scope != nil && *req.Scope != v.Scope {
		switch *req.Scope {
		case viewScopeBoard:
			// Already known to be the owner (canEditBoardView); the shared list is
			// the managers'.
			if !h.requireManager(c, wsID) {
				return
			}
			next.Scope, next.UserID = viewScopeBoard, nil
		case viewScopeUser:
			next.Scope, next.UserID = viewScopeUser, &uid
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "scope must be user or board"})
			return
		}
		// A view changing lists arrives at the end of its new one, un-starred: the
		// star meant "default for me" or "for everybody", and neither carries over.
		next.IsDefault = false
		moved = true
	}

	err = h.inTx(c, func(q *db.Queries) error {
		if moved && req.BeforeID == nil && req.AfterID == nil {
			var end float64
			var err error
			if next.Scope == viewScopeBoard {
				end, err = q.MaxSharedBoardViewPosition(c, next.BoardID)
			} else {
				end, err = q.MaxUserBoardViewPosition(c, db.MaxUserBoardViewPositionParams{BoardID: next.BoardID, UserID: next.UserID})
			}
			if err != nil {
				return err
			}
			next.Position = end + positionGap
		}
		if req.BeforeID != nil || req.AfterID != nil {
			prev, err := h.viewNeighbourPos(c, q, next, req.BeforeID)
			if err != nil {
				return err
			}
			after, err := h.viewNeighbourPos(c, q, next, req.AfterID)
			if err != nil {
				return err
			}
			next.Position = positionBetween(prev, after)
		}
		updated, err := q.UpdateBoardView(c, db.UpdateBoardViewParams{
			ID: next.ID, Name: next.Name, Config: next.Config, Scope: next.Scope,
			UserID: next.UserID, Position: next.Position, IsDefault: next.IsDefault,
		})
		if isUniqueViolation(err) {
			return errViewNameTaken
		}
		if err != nil {
			return err
		}
		if req.IsDefault != nil && *req.IsDefault != updated.IsDefault {
			updated, err = setBoardViewDefault(c, q, updated, *req.IsDefault)
			if err != nil {
				return err
			}
		}
		next = updated
		return nil
	})
	if errors.Is(err, errViewNameTaken) {
		c.JSON(http.StatusConflict, gin.H{"error": "view name taken"})
		return
	}
	if errors.Is(err, errViewNeighbour) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "neighbour is not in the same list"})
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	if wasShared || next.Scope == viewScopeBoard {
		h.broadcastAs(c, wsID, evBoardViews, gin.H{"board_id": next.BoardID})
	}
	creator := ""
	if next.CreatedBy != nil {
		creator = h.userName(c, *next.CreatedBy)
	}
	c.JSON(http.StatusOK, toBoardViewView(next, creator, true))
}

var errViewNeighbour = errors.New("neighbour is not in the same list")

// viewNeighbourPos reads the position of a move's neighbour, which must sit in
// the same list (same board, same scope, same owner) as the view being moved.
func (h *API) viewNeighbourPos(c *gin.Context, q *db.Queries, v db.BoardView, id *uuid.UUID) (*float64, error) {
	if id == nil {
		return nil, nil
	}
	n, err := q.GetBoardView(c, *id)
	if err != nil {
		return nil, errViewNeighbour
	}
	sameOwner := (n.UserID == nil && v.UserID == nil) ||
		(n.UserID != nil && v.UserID != nil && *n.UserID == *v.UserID)
	if n.BoardID != v.BoardID || n.Scope != v.Scope || !sameOwner || n.ID == v.ID {
		return nil, errViewNeighbour
	}
	return &n.Position, nil
}

// DeleteBoardView removes a view: a personal one by its owner, a shared one by
// a workspace owner/admin.
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
	wsID, ok := h.boardViewWorkspace(c, v.BoardID)
	if !ok {
		return
	}
	if !h.canEditBoardView(c, v, wsID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "not your view"})
		return
	}
	if err := h.q.DeleteBoardView(c, id); err != nil {
		fail(c, err)
		return
	}
	if v.Scope == viewScopeBoard {
		h.broadcastAs(c, wsID, evBoardViews, gin.H{"board_id": v.BoardID})
	}
	c.Status(http.StatusNoContent)
}
