package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Project archive (migration 0086, #3005).
//
// An archived project is not deleted and has no expiry: it drops out of the
// sidebar tree (GET /workspaces/:id/projects), search, the palette, «Мои
// задачи» and the notification scanners, and pauses its schedule-driven
// recurrences — but it still opens, read-only, from the «Архив проектов» list
// (GET /workspaces/:id/projects?archived=1). Writes on its boards and tasks are
// refused with 409 {archived: true} (requireProjectWritable); «Восстановить»
// (POST /projects/:id/restore) brings everything back as it was.

// errProjectArchived is the refusal every write on an archived project gets.
const errProjectArchived = "проект в архиве — только просмотр"

// requireProjectWritable refuses a write on a board whose project is in the
// archive. A lookup failure lets the write through: the caller has already
// resolved the board, and its own query answers for a missing one.
func (h *API) requireProjectWritable(c *gin.Context, boardID uuid.UUID) bool {
	archived, err := h.q.ProjectArchivedForBoard(c, boardID)
	if err != nil || !archived {
		return true
	}
	c.JSON(http.StatusConflict, gin.H{"error": errProjectArchived, "archived": true})
	return false
}

// requireColumnWritable is requireProjectWritable for the column routes, which
// name a column (live or, for the restore, deleted) rather than its board.
func (h *API) requireColumnWritable(c *gin.Context, columnID uuid.UUID) bool {
	col, err := h.q.GetColumnAny(c, columnID)
	if err != nil {
		return true
	}
	return h.requireProjectWritable(c, col.BoardID)
}

// isWrite reports whether the request changes something. loadTask uses it to
// keep an archived project's tasks readable but not writable.
func isWrite(c *gin.Context) bool {
	switch c.Request.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}
	return true
}

// ArchiveProject puts a project in the archive (POST /projects/:id/archive).
// Owner/admin only, like the slug and the transfer: the project vanishes from
// everybody's tree.
func (h *API) ArchiveProject(c *gin.Context) {
	p, ok := h.loadProject(c)
	if !ok || !h.requireManager(c, p.WorkspaceID) {
		return
	}
	archived, err := h.q.ArchiveProject(c, p.ID)
	if notFoundMsg(c, err, "проект уже в архиве") {
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	h.broadcastAs(c, p.WorkspaceID, "project.archived", archived)
	c.JSON(http.StatusOK, archived)
}

// UnarchiveProject is «Восстановить» (POST /projects/:id/restore).
func (h *API) UnarchiveProject(c *gin.Context) {
	p, ok := h.loadProject(c)
	if !ok || !h.requireManager(c, p.WorkspaceID) {
		return
	}
	restored, err := h.q.UnarchiveProject(c, p.ID)
	if notFoundMsg(c, err, "проект не в архиве") {
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	h.broadcastAs(c, p.WorkspaceID, "project.restored", restored)
	c.JSON(http.StatusOK, restored)
}
