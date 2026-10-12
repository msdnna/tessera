package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/db"
)

// The task trash (migration 0086, #3004 / #3120).
//
// DELETE /tasks/:id is a soft delete: the task and its subtasks get one
// deleted_at stamp and drop out of every live read (GetTask, the board lists,
// search, «Мои задачи», the scanners — the ratchet in trash_ratchet_test.go
// keeps new queries honest). From the trash a task comes back with
// POST /tasks/:id/restore (the undo toast, the board's «Корзина»), or goes for
// good with DELETE /tasks/:id/purge («Удалить навсегда») or after
// taskTrashRetention by trash_purge.
//
// The archive (PATCH /tasks/:id/archive and its own PATCH …/restore) is a
// separate thing: an archived task is finished work kept for reference, a task
// in the trash is a mistake waiting to be undone or forgotten.

type slimBoardTrashed struct {
	db.ListBoardTrashWithMetaRow
	Description    string `json:"description,omitempty"`
	HasDescription bool   `json:"has_description"`
}

// ListBoardTrash returns the board's trash (GET /boards/:id/trash): the tasks
// that went there as entries of their own, newest first.
func (h *API) ListBoardTrash(c *gin.Context) {
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
	rows, err := h.q.ListBoardTrashWithMeta(c, boardID)
	if err != nil {
		fail(c, err)
		return
	}
	out := make([]slimBoardTrashed, len(rows))
	for i, r := range rows {
		out[i] = slimBoardTrashed{ListBoardTrashWithMetaRow: r, HasDescription: r.Description != ""}
	}
	c.JSON(http.StatusOK, out)
}

// loadTrashedTask resolves a task that is in the trash and authorizes the
// caller against its workspace. A live task, or one on a deleted board (the
// board's own restore brings it back), is a 404 here.
func (h *API) loadTrashedTask(c *gin.Context, id uuid.UUID) (db.Task, uuid.UUID, bool) {
	t, err := h.q.GetTaskAny(c, id)
	if notFound(c, err) {
		return db.Task{}, uuid.Nil, false
	}
	if err != nil {
		fail(c, err)
		return db.Task{}, uuid.Nil, false
	}
	wsID, err := h.q.WorkspaceIDForBoard(c, t.BoardID)
	if notFound(c, err) {
		return db.Task{}, uuid.Nil, false
	}
	if err != nil {
		fail(c, err)
		return db.Task{}, uuid.Nil, false
	}
	if !h.requireMember(c, wsID) {
		return db.Task{}, uuid.Nil, false
	}
	if t.DeletedAt == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "задача не в корзине"})
		return db.Task{}, uuid.Nil, false
	}
	return t, wsID, true
}

// RestoreDeletedTask takes a task out of the trash with the subtasks that went
// in with it (POST /tasks/:id/restore — not to be confused with PATCH
// /tasks/:id/restore, the archive's). A subtask whose parent is still in the
// trash comes back as a card at the end of its column: under a parent nobody
// can see it would be drawn nowhere.
func (h *API) RestoreDeletedTask(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	t, wsID, ok := h.loadTrashedTask(c, id)
	if !ok {
		return
	}
	if !h.requireProjectWritable(c, t.BoardID) {
		return
	}
	detach := false
	position := t.Position
	if t.ParentID != nil {
		p, perr := h.q.GetTaskAny(c, *t.ParentID)
		if perr != nil && !errIsNoRows(perr) {
			fail(c, perr)
			return
		}
		if perr != nil || p.DeletedAt != nil {
			detach = true
			maxPos, merr := h.q.MaxTaskPositionInColumn(c, t.ColumnID)
			if merr != nil {
				fail(c, merr)
				return
			}
			position = maxPos + positionGap
		}
	}
	if _, err := h.q.RestoreTaskSubtree(c, db.RestoreTaskSubtreeParams{
		ID: id, Detach: detach, Position: position,
	}); err != nil {
		fail(c, err)
		return
	}
	restored, err := h.q.GetTask(c, id)
	if err != nil {
		fail(c, err)
		return
	}
	h.logEvent(c, id, "untrashed", nil)
	// Clients re-read the board on a task event they cannot patch in; the trash
	// view reloads its list on the same event.
	h.broadcastAs(c, wsID, "task.restored", gin.H{
		"id": id, "number": restored.Number, "title": restored.Title,
		"board_id": restored.BoardID, "from": "trash",
	})
	c.JSON(http.StatusOK, restored)
}

// PurgeDeletedTask removes a task from the trash for good
// (DELETE /tasks/:id/purge, «Удалить навсегда»). Only a task already in the
// trash can be purged: a live one has to go through the reversible delete
// first, so no single gesture loses work.
func (h *API) PurgeDeletedTask(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	t, wsID, ok := h.loadTrashedTask(c, id)
	if !ok {
		return
	}
	if !h.requireProjectWritable(c, t.BoardID) {
		return
	}
	gone, err := h.q.DeleteTask(c, id)
	if err != nil {
		fail(c, err)
		return
	}
	// Sweep the attachments of every task the delete actually took, subtasks
	// included — each has its own <uploadDir>/<id>/ (UploadAttachment), and its
	// rows have just gone with it, so nothing names those files any more.
	h.purgeTaskUploads(gone)
	h.broadcastAs(c, wsID, "task.purged", gin.H{"id": id, "board_id": t.BoardID})
	c.Status(http.StatusNoContent)
}
