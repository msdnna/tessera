package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/db"
	"tessera/internal/i18n"
	"tessera/middleware"
)

// ── Boards ─────────────────────────────────────────────────

// boardScopeView is a board plus the workspace it lives in. Deep-link resolution
// needs the scope (the client filters realtime events and loads members by it),
// and both resolvers already hold the id for their access check — so returning
// it costs no extra query. Embedding keeps the JSON shape flat and additive.
type boardScopeView struct {
	db.Board
	WorkspaceID uuid.UUID `json:"workspace_id"`
}

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
	if p.ArchivedAt != nil {
		c.JSON(http.StatusConflict, gin.H{"error": errProjectArchived, "archived": true})
		return
	}
	var req struct {
		Name string `json:"name" binding:"required"`
		// The view a new board opens in (#3017) — the inline row's Tab switches
		// Канбан / Список. Absent = kanban, as before.
		DefaultLayout string `json:"default_layout"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	layout, ok := boardLayout(req.DefaultLayout)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown board layout"})
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
			Position: positionBetween(&maxPos, nil), DefaultLayout: layout,
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
				BoardID: b.ID, Name: dc.name, NameKey: ptr(dc.nameKey),
				Color: dc.color, Position: pos,
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

	h.broadcastAs(c, p.WorkspaceID, "board.created", b)
	c.JSON(http.StatusCreated, b)
}

// boardLayout validates a board view name (#3017): the layouts the web client
// draws. "" means the default — kanban.
func boardLayout(s string) (string, bool) {
	switch s {
	case "":
		return "board", true
	case "board", "list", "timeline", "gantt", "calendar", "matrix":
		return s, true
	}
	return "", false
}

// doneColumnName is the default name for the task-completing column (used only
// when seeding a new board's columns; the live "done" column is tracked
// per-board via boards.done_column_id).
const doneColumnName = "Готово"

// defaultColumns are created for every new board. Each carries a nameKey next to
// its Russian name: the client captions seeded columns from its own catalogue, so
// they follow the reader's language, while name stays as the stored fallback (old
// clients, GitLab rules, DB lookups). Renaming a column clears its key — see
// UpdateColumn.
var defaultColumns = []struct{ name, nameKey, color string }{
	// v2 lane palette (#2951): teal · blue · orange · green.
	{"К работе", "todo", "#0eb0a9"},
	{"В процессе", "in_progress", "#2f80ed"},
	{"На рассмотрении", "review", "#f0a020"},
	{doneColumnName, "done", "#18a058"},
}

// defaultColumnKey maps a seeded column's stored (Russian) name to its key, or ""
// for a name that is not one of the four defaults. It lets code that only knows a
// column by name — GitLab rules, for one — reach the same column by key.
func defaultColumnKey(name string) string {
	for _, dc := range defaultColumns {
		if dc.name == name {
			return dc.nameKey
		}
	}
	return ""
}

// seededColumnCaptions is what each client writes on a seeded column, per
// language: the twin of frontend/src/locales/<lang>/board.json
// (column.defaultName) and android/…/res/values*/strings.xml (default_column_*).
// Adding a language to i18n.Supported means adding its arm here —
// TestSeededColumnCaptionsCoverSupportedLanguages iterates that list.
//
// This is not the server localising its own output (i18n's package doc is clear
// that clients do that): it is the reverse direction. A user typing `/move Done`
// is echoing the caption their own screen shows them, and only the server can
// resolve a quick action, so the server has to recognise the words the client
// put on the card. The Russian arm duplicates defaultColumns on purpose — the
// two are edited in different places, and the test above pins them together.
var seededColumnCaptions = map[string]map[string]string{
	"ru": {"todo": "К работе", "in_progress": "В процессе", "review": "На рассмотрении", "done": doneColumnName},
	"en": {"todo": "To do", "in_progress": "In progress", "review": "In review", "done": "Done"},
}

// seededColumnKey maps a caption a client may have shown the user back to the
// seeded column's key, in any supported language, or "" for a caption that is
// not one of the four. Case- and space-insensitive: it matches text a human
// retyped, not a stored value.
func seededColumnKey(caption string) string {
	caption = strings.TrimSpace(caption)
	if caption == "" {
		return ""
	}
	for _, lang := range i18n.Supported {
		for key, want := range seededColumnCaptions[lang] {
			if strings.EqualFold(want, caption) {
				return key
			}
		}
	}
	return ""
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
	c.JSON(http.StatusOK, boardScopeView{Board: b, WorkspaceID: proj.WorkspaceID})
}

// resolveBoardParam resolves a board route's :id segment — a UUID or a
// human-readable slug (/board/<slug> deep links) — and authorizes the caller.
// Writes the error response itself; ok=false means the caller must return.
//
// The slug branch searches only workspaces the caller belongs to: board slugs
// are unique per project, not globally (0025), so a bare slug matches several
// boards at once and an unscoped lookup regularly answered with a stranger's
// board — a 403 on the caller's own link, or the wrong board of a workspace they
// happen to share. `?workspace=` is the client's hint about which one it means.
func (h *API) resolveBoardParam(c *gin.Context) (db.Board, uuid.UUID, bool) {
	param := c.Param("id")
	var b db.Board
	var err error
	if id, perr := uuid.Parse(param); perr == nil {
		b, err = h.q.GetBoard(c, id)
	} else {
		var preferred *uuid.UUID
		if hint, perr := uuid.Parse(c.Query("workspace")); perr == nil {
			preferred = &hint
		}
		b, err = h.q.GetBoardBySlugForUser(c, db.GetBoardBySlugForUserParams{
			UserID: middleware.CurrentUser(c), Slug: param, PreferredWorkspace: preferred,
		})
	}
	if notFound(c, err) {
		return db.Board{}, uuid.Nil, false
	}
	if err != nil {
		fail(c, err)
		return db.Board{}, uuid.Nil, false
	}
	wsID, err := h.q.WorkspaceIDForBoard(c, b.ID)
	if err != nil {
		fail(c, err)
		return db.Board{}, uuid.Nil, false
	}
	// Redundant for the slug branch (the query joins the membership) but not for
	// a UUID, and it is the one place that turns a lost membership into a 403.
	if !h.requireMember(c, wsID) {
		return db.Board{}, uuid.Nil, false
	}
	return b, wsID, true
}

// GetBoard returns a single board by UUID or human-readable slug.
func (h *API) GetBoard(c *gin.Context) {
	b, wsID, ok := h.resolveBoardParam(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, boardScopeView{Board: b, WorkspaceID: wsID})
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
	if !h.requireMember(c, wsID) || !h.requireProjectWritable(c, id) {
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
		// Absent keeps the board's view for newcomers (#3017).
		DefaultLayout *string `json:"default_layout"`
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
	layout := b.DefaultLayout
	if req.DefaultLayout != nil {
		l, ok := boardLayout(*req.DefaultLayout)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unknown board layout"})
			return
		}
		layout = l
	}
	updated, err := h.q.UpdateBoard(c, db.UpdateBoardParams{
		ID: id, Name: req.Name, Position: b.Position, Icon: icon, Color: color, IconMode: iconMode,
		DefaultLayout: layout,
	})
	if err != nil {
		fail(c, err)
		return
	}
	h.broadcast(wsID, "board.updated", updated)
	c.JSON(http.StatusOK, updated)
}

// DeleteBoard soft-deletes a board (0078): it disappears for every reader at
// once, and RestoreBoard can bring it back until the trash worker purges it
// (trashRetention). The v2 client offers that undo in a toast. Nothing else is
// touched here — columns and cards stay as they are, so a restore is exact.
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
	if !h.requireMember(c, wsID) || !h.requireProjectWritable(c, id) {
		return
	}
	if _, err := h.q.SoftDeleteBoard(c, id); notFound(c, err) {
		return
	} else if err != nil {
		fail(c, err)
		return
	}
	h.broadcast(wsID, "board.deleted", gin.H{"id": id})
	c.Status(http.StatusNoContent)
}

// RestoreBoard undoes DeleteBoard while the board is still in the trash. The
// board comes back as it was; clients learn about it through board.created, the
// event they already handle for a new board (the Android client included).
func (h *API) RestoreBoard(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	wsID, err := h.q.WorkspaceIDForDeletedBoard(c, id)
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
	b, err := h.q.RestoreBoard(c, id)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	h.broadcastAs(c, wsID, "board.created", b)
	c.JSON(http.StatusOK, b)
}

// SetDoneColumn configures which column auto-completes tasks (or, when column_id
// is null, leaves the board without one at all — see doneColumnID, #2588).
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
	if !h.requireMember(c, wsID) || !h.requireProjectWritable(c, id) {
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
	if !h.requireMember(c, wsID) || !h.requireProjectWritable(c, boardID) {
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
	if !h.requireMember(c, wsID) || !h.requireColumnWritable(c, id) {
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
	if !h.requireMember(c, wsID) || !h.requireColumnWritable(c, id) {
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
	if rejectSameNeighbour(c, req.BeforeID, req.AfterID) {
		return
	}
	prev, next, ok := h.neighborColumnPositions(c, col.BoardID, req.BeforeID, req.AfterID)
	if !ok {
		return
	}
	pos, ok := h.positionForReorder(c, prev, next,
		func() error {
			return h.q.RebalanceColumnPositions(c, db.RebalanceColumnPositionsParams{
				BoardID: col.BoardID, Gap: positionGap,
			})
		},
		func() (*float64, *float64, bool) {
			return h.neighborColumnPositions(c, col.BoardID, req.BeforeID, req.AfterID)
		})
	if !ok {
		return
	}
	updated, err := h.q.UpdateColumn(c, db.UpdateColumnParams{
		ID: id, Name: col.Name, Color: col.Color, Position: pos,
	})
	if err != nil {
		fail(c, err)
		return
	}
	h.broadcast(wsID, "column.moved", updated)
	c.JSON(http.StatusOK, updated)
}

// columnMoveGap spaces the cards a column delete appends to its target — the
// same ladder step a fresh column's cards get.
const columnMoveGap = 1024.0

// DeleteColumn soft-deletes a column (0078). Its cards no longer go with it:
// they move to another column of the board, keeping their order, and
// RestoreColumn can put everything back while the column is in the trash.
//
// The target is the board's first remaining column — except when the deleted
// column was the completing one. Then the board's done flag moves to the
// rightmost remaining column (an unannounced NULL would stop the board closing
// anything, see #2588 and the repair in migration 0046), and the cards follow
// it there: they are finished work, and dropping a hundred completed cards into
// «К работе» would undo them in bulk. completed_at is left as it was either way,
// and nothing is pushed to GitLab — this is housekeeping of the board, not a
// move anyone made on a card. The last column of a board can't be deleted:
// there would be nowhere to put its cards.
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
	if !h.requireMember(c, wsID) || !h.requireColumnWritable(c, id) {
		return
	}
	col, err := h.q.GetColumn(c, id)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	var (
		target    db.BoardColumn
		moved     int64
		repointed *db.Board
	)
	errLastColumn := errors.New("the last column of a board can't be deleted")
	if err := h.inTx(c, func(q *db.Queries) error {
		board, err := q.GetBoard(c, col.BoardID)
		if err != nil {
			return err
		}
		cols, err := q.ListColumns(c, col.BoardID)
		if err != nil {
			return err
		}
		rest := make([]db.BoardColumn, 0, len(cols))
		for _, cc := range cols {
			if cc.ID != id {
				rest = append(rest, cc)
			}
		}
		if len(rest) == 0 {
			return errLastColumn
		}
		wasDone := board.DoneColumnID != nil && *board.DoneColumnID == id
		target = rest[0]
		if wasDone {
			target = rest[len(rest)-1]
		}
		base, err := q.MaxTaskPositionInColumn(c, target.ID)
		if err != nil {
			return err
		}
		if _, err = q.RecordColumnMoves(c, db.RecordColumnMovesParams{MovedTo: target.ID, ColumnID: id}); err != nil {
			return err
		}
		if moved, err = q.MoveColumnTasks(c, db.MoveColumnTasksParams{
			Target: target.ID, Base: base, Gap: columnMoveGap, Source: id,
		}); err != nil {
			return err
		}
		subs, err := q.MoveColumnSubtasks(c, db.MoveColumnSubtasksParams{Target: target.ID, Source: id})
		if err != nil {
			return err
		}
		moved += subs
		if _, err = q.SoftDeleteColumn(c, db.SoftDeleteColumnParams{WasDone: wasDone, ID: id}); err != nil {
			return err
		}
		if !wasDone {
			return nil
		}
		updated, err := q.SetBoardDoneColumn(c, db.SetBoardDoneColumnParams{ID: col.BoardID, DoneColumnID: &target.ID})
		if err != nil {
			return err
		}
		repointed = &updated
		return nil
	}); err != nil {
		if errors.Is(err, errLastColumn) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		if notFound(c, err) {
			return
		}
		fail(c, err)
		return
	}
	h.broadcast(wsID, "column.deleted", gin.H{
		"id": id, "board_id": col.BoardID, "moved_to": target.ID, "moved": moved,
	})
	if repointed != nil {
		h.broadcast(wsID, "board.updated", *repointed)
	}
	c.Status(http.StatusNoContent)
}

// RestoreColumn undoes DeleteColumn: the column returns to its place, the cards
// the delete moved go back to where they stood — those still in the column the
// delete put them in; a card someone has moved since stays where they put it —
// and a column deleted as the completing one gets the done flag back.
func (h *API) RestoreColumn(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	wsID, err := h.q.WorkspaceIDForDeletedColumn(c, id)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	if !h.requireMember(c, wsID) || !h.requireColumnWritable(c, id) {
		return
	}
	var (
		col       db.BoardColumn
		repointed *db.Board
	)
	if err := h.inTx(c, func(q *db.Queries) error {
		before, err := q.GetColumnAny(c, id)
		if err != nil {
			return err
		}
		if col, err = q.RestoreColumn(c, id); err != nil {
			return err
		}
		if _, err = q.RestoreColumnTasks(c, id); err != nil {
			return err
		}
		if err = q.ClearColumnMoves(c, id); err != nil {
			return err
		}
		if !before.DeletedWasDone {
			return nil
		}
		updated, err := q.SetBoardDoneColumn(c, db.SetBoardDoneColumnParams{ID: col.BoardID, DoneColumnID: &col.ID})
		if err != nil {
			return err
		}
		repointed = &updated
		return nil
	}); err != nil {
		if notFound(c, err) {
			return
		}
		fail(c, err)
		return
	}
	h.broadcast(wsID, "column.restored", gin.H{"id": col.ID, "board_id": col.BoardID, "column": col})
	if repointed != nil {
		h.broadcast(wsID, "board.updated", *repointed)
	}
	c.JSON(http.StatusOK, col)
}

// neighborColumnPositions resolves the positions of the before/after columns.
// Returns (nil, nil) bounds when an id is absent; writes 400 and returns ok=false
// if a referenced column can't be loaded or doesn't stand on [boardID].
//
// The board check is the point, not a formality: every board numbers its columns
// on its OWN float8 ladder, so a neighbour from another board contributes a
// position that means nothing here — the moved column would land at an arbitrary
// end of its own row. A neighbour id is client-supplied and can go stale (a drag
// deliberately freezes the board, so a colleague's edits arrive only after the
// drop), and `GetColumn` takes a bare id: without this, the id also answers
// "does this column exist?" for workspaces the caller isn't a member of. Same
// 400 either way, so the reply tells the two cases apart no better than the
// honest one did (#2932).
func (h *API) neighborColumnPositions(c *gin.Context, boardID uuid.UUID, beforeID, afterID *uuid.UUID) (prev, next *float64, ok bool) {
	if beforeID != nil {
		col, err := h.q.GetColumn(c, *beforeID)
		if err != nil || col.BoardID != boardID {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid before_id"})
			return nil, nil, false
		}
		prev = &col.Position
	}
	if afterID != nil {
		col, err := h.q.GetColumn(c, *afterID)
		if err != nil || col.BoardID != boardID {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid after_id"})
			return nil, nil, false
		}
		next = &col.Position
	}
	return prev, next, true
}
