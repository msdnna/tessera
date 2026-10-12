package handlers

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"tessera/internal/db"
	"tessera/middleware"
)

// evFavorites tells the caller's other tabs and devices that their favourite
// boards changed (#3115). The list is one person's business alone, so it goes to
// their sockets only (broadcastToUser), never to the workspace.
const evFavorites = "favorites.changed"

// boardFavoriteView is one row of «Избранное»: the board, what the sidebar row
// draws (icon, colour, the project's name) and where it sits in the list.
type boardFavoriteView struct {
	BoardID      uuid.UUID `json:"board_id"`
	Position     float64   `json:"position"`
	CreatedAt    time.Time `json:"created_at"`
	Name         string    `json:"name"`
	Slug         string    `json:"slug"`
	Icon         string    `json:"icon"`
	Color        string    `json:"color"`
	IconMode     string    `json:"icon_mode"`
	ProjectID    uuid.UUID `json:"project_id"`
	ProjectName  string    `json:"project_name"`
	ProjectSlug  string    `json:"project_slug"`
	ProjectColor string    `json:"project_color"`
	WorkspaceID  uuid.UUID `json:"workspace_id"`
}

func favoriteFromRow(r db.GetBoardFavoriteRowRow) boardFavoriteView {
	return boardFavoriteView(r)
}

// ListBoardFavorites returns the caller's favourite boards in a workspace, in
// their order. Deleted boards and boards of workspaces the caller has left are
// filtered by the query itself.
func (h *API) ListBoardFavorites(c *gin.Context) {
	wsID, ok := parseID(c, "id")
	if !ok {
		return
	}
	if !h.requireMember(c, wsID) {
		return
	}
	rows, err := h.q.ListBoardFavorites(c, db.ListBoardFavoritesParams{
		UserID: middleware.CurrentUser(c), WorkspaceID: wsID,
	})
	if err != nil {
		fail(c, err)
		return
	}
	out := make([]boardFavoriteView, 0, len(rows))
	for _, r := range rows {
		out = append(out, boardFavoriteView(r))
	}
	c.JSON(http.StatusOK, out)
}

// AddBoardFavorite stars a board for the caller; it goes to the end of their
// list. Starring a board that is already a favourite keeps its place and answers
// the same row — the call is idempotent, so a double click or two tabs racing
// cannot produce two entries.
func (h *API) AddBoardFavorite(c *gin.Context) {
	boardID, ok := parseID(c, "id")
	if !ok {
		return
	}
	wsID, ok := h.boardViewWorkspace(c, boardID)
	if !ok {
		return
	}
	uid := middleware.CurrentUser(c)
	err := h.inTx(c, func(q *db.Queries) error {
		end, err := q.MaxBoardFavoritePosition(c, uid)
		if err != nil {
			return err
		}
		_, err = q.AddBoardFavorite(c, db.AddBoardFavoriteParams{
			UserID: uid, BoardID: boardID, Position: positionBetween(&end, nil),
		})
		return err
	})
	if err != nil {
		fail(c, err)
		return
	}
	row, err := h.q.GetBoardFavoriteRow(c, db.GetBoardFavoriteRowParams{UserID: uid, BoardID: boardID})
	if err != nil {
		fail(c, err)
		return
	}
	h.broadcastToUser(wsID, uid, evFavorites, gin.H{"board_id": boardID, "favorite": true})
	c.JSON(http.StatusOK, favoriteFromRow(row))
}

// RemoveBoardFavorite takes the star off. Removing a board that is not a
// favourite is not an error (204 either way): the caller's intent — «not in my
// favourites» — holds. It works on a soft-deleted board too, so a star can be
// dropped from a board that is in the trash.
func (h *API) RemoveBoardFavorite(c *gin.Context) {
	boardID, ok := parseID(c, "id")
	if !ok {
		return
	}
	wsID, err := h.q.WorkspaceIDForBoard(c, boardID)
	if errors.Is(err, pgx.ErrNoRows) {
		wsID, err = h.q.WorkspaceIDForDeletedBoard(c, boardID)
	}
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
	uid := middleware.CurrentUser(c)
	n, err := h.q.DeleteBoardFavorite(c, db.DeleteBoardFavoriteParams{UserID: uid, BoardID: boardID})
	if err != nil {
		fail(c, err)
		return
	}
	if n > 0 {
		h.broadcastToUser(wsID, uid, evFavorites, gin.H{"board_id": boardID, "favorite": false})
	}
	c.Status(http.StatusNoContent)
}

// MoveBoardFavorite drops a favourite between two others of the same list —
// before_id / after_id are the neighbours' board ids (either may be omitted for
// the ends), the same contract as moving a column.
func (h *API) MoveBoardFavorite(c *gin.Context) {
	boardID, ok := parseID(c, "id")
	if !ok {
		return
	}
	wsID, ok := h.boardViewWorkspace(c, boardID)
	if !ok {
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
	if (req.BeforeID != nil && *req.BeforeID == boardID) || (req.AfterID != nil && *req.AfterID == boardID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "a favourite cannot be its own neighbour"})
		return
	}
	uid := middleware.CurrentUser(c)
	if _, err := h.q.GetBoardFavoriteRow(c, db.GetBoardFavoriteRowParams{UserID: uid, BoardID: boardID}); err != nil {
		if notFoundMsg(c, err, "board is not a favourite") {
			return
		}
		fail(c, err)
		return
	}
	neighbors := func() (*float64, *float64, bool) {
		return h.neighborFavoritePositions(c, uid, wsID, req.BeforeID, req.AfterID)
	}
	prev, next, ok := neighbors()
	if !ok {
		return
	}
	pos, ok := h.positionForReorder(c, prev, next,
		func() error {
			return h.q.RebalanceBoardFavoritePositions(c, db.RebalanceBoardFavoritePositionsParams{
				UserID: uid, Gap: positionGap,
			})
		},
		neighbors)
	if !ok {
		return
	}
	if err := h.q.SetBoardFavoritePosition(c, db.SetBoardFavoritePositionParams{
		UserID: uid, BoardID: boardID, Position: pos,
	}); err != nil {
		fail(c, err)
		return
	}
	row, err := h.q.GetBoardFavoriteRow(c, db.GetBoardFavoriteRowParams{UserID: uid, BoardID: boardID})
	if err != nil {
		fail(c, err)
		return
	}
	h.broadcastToUser(wsID, uid, evFavorites, gin.H{"board_id": boardID, "favorite": true})
	c.JSON(http.StatusOK, favoriteFromRow(row))
}

// neighborFavoritePositions reads the positions of the two favourites a moved
// one lands between. Each must be one of the caller's own favourites in the same
// workspace — anything else is a 400, not a silent append.
func (h *API) neighborFavoritePositions(c *gin.Context, uid, wsID uuid.UUID, beforeID, afterID *uuid.UUID) (prev, next *float64, ok bool) {
	read := func(id *uuid.UUID, field string) (*float64, bool) {
		if id == nil {
			return nil, true
		}
		r, err := h.q.GetBoardFavoriteRow(c, db.GetBoardFavoriteRowParams{UserID: uid, BoardID: *id})
		if err != nil || r.WorkspaceID != wsID {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid " + field})
			return nil, false
		}
		return &r.Position, true
	}
	if prev, ok = read(beforeID, "before_id"); !ok {
		return nil, nil, false
	}
	if next, ok = read(afterID, "after_id"); !ok {
		return nil, nil, false
	}
	return prev, next, true
}
