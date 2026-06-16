package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/db"
	"tessera/middleware"
)

// taskDetail bundles a task with its tags, assignees and direct subtasks, plus
// GitLab provenance when the task is mirrored from a GitLab issue.
type taskDetail struct {
	db.Task
	Tags            []db.Tag                        `json:"tags"`
	Assignees       []db.ListTaskAssigneesRow       `json:"assignees"`
	GitlabAssignees []db.ListTaskGitlabAssigneesRow `json:"gitlab_assignees"`
	Subtasks        []db.ListSubtasksWithMetaRow    `json:"subtasks"`
	GitLab          *gitlabLinkView                 `json:"gitlab,omitempty"`
}

// CreateTask adds a task (or subtask) to a column on a board.
func (h *API) CreateTask(c *gin.Context) {
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
		ColumnID    uuid.UUID  `json:"column_id" binding:"required"`
		ParentID    *uuid.UUID `json:"parent_id"`
		Title       string     `json:"title" binding:"required"`
		Description string     `json:"description"`
		Priority    int32      `json:"priority"`
		DueDate     *time.Time `json:"due_date"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// The column must belong to this board.
	col, err := h.q.GetColumn(c, req.ColumnID)
	if err != nil || col.BoardID != boardID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "column does not belong to this board"})
		return
	}

	pos, err := h.nextTaskPosition(c, req.ColumnID, req.ParentID)
	if err != nil {
		fail(c)
		return
	}

	num, err := h.q.NextWorkspaceTaskNumber(c, wsID)
	if err != nil {
		fail(c)
		return
	}

	uid := middleware.CurrentUser(c)
	t, err := h.q.CreateTask(c, db.CreateTaskParams{
		BoardID:     boardID,
		ColumnID:    req.ColumnID,
		ParentID:    req.ParentID,
		Title:       req.Title,
		Description: req.Description,
		Priority:    req.Priority,
		DueDate:     req.DueDate,
		Position:    pos,
		CreatedBy:   &uid,
		Number:      &num,
	})
	if err != nil {
		fail(c)
		return
	}
	h.logEvent(c, t.ID, "created", nil)
	h.broadcast(wsID, "task.created", t)
	c.JSON(http.StatusCreated, t)
}

// ListBoardTasks returns the top-level tasks of a board.
func (h *API) ListBoardTasks(c *gin.Context) {
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
	tasks, err := h.q.ListBoardTasksWithMeta(c, boardID)
	if err != nil {
		fail(c)
		return
	}
	c.JSON(http.StatusOK, tasks)
}

// ListBoardSubtasks returns every subtask on a board (with meta) so the kanban
// can render them under their parent cards.
func (h *API) ListBoardSubtasks(c *gin.Context) {
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
	subs, err := h.q.ListBoardSubtasksWithMeta(c, boardID)
	if err != nil {
		fail(c)
		return
	}
	c.JSON(http.StatusOK, subs)
}

// SetTaskParent attaches a task to a parent (becoming its subtask, inheriting
// the parent's board/column) or detaches it (parent_id null → back on the board
// as a top-level task).
func (h *API) SetTaskParent(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	t, wsID, ok := h.loadTask(c, id)
	if !ok {
		return
	}
	var req struct {
		ParentID *uuid.UUID `json:"parent_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	boardID, columnID := t.BoardID, t.ColumnID
	if req.ParentID != nil {
		if *req.ParentID == t.ID {
			c.JSON(http.StatusBadRequest, gin.H{"error": "a task cannot be its own parent"})
			return
		}
		parent, err := h.q.GetTask(c, *req.ParentID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid parent"})
			return
		}
		// The parent must not itself be a child of this task (1-level cycle).
		if parent.ParentID != nil && *parent.ParentID == t.ID {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cyclic parent"})
			return
		}
		boardID, columnID = parent.BoardID, parent.ColumnID
	}
	updated, err := h.q.SetTaskParent(c, db.SetTaskParentParams{
		ID: id, ParentID: req.ParentID, BoardID: boardID, ColumnID: columnID,
	})
	if err != nil {
		fail(c)
		return
	}
	h.broadcast(wsID, "task.moved", updated)
	c.JSON(http.StatusOK, updated)
}

// GetTask returns a task with its tags, assignees and subtasks.
func (h *API) GetTask(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	t, wsID, ok := h.loadTask(c, id)
	if !ok {
		return
	}
	tags, err := h.q.ListTaskTags(c, id)
	if err != nil {
		fail(c)
		return
	}
	assignees, err := h.q.ListTaskAssignees(c, id)
	if err != nil {
		fail(c)
		return
	}
	subtasks, err := h.q.ListSubtasksWithMeta(c, &id)
	if err != nil {
		fail(c)
		return
	}
	_ = wsID
	if subtasks == nil {
		subtasks = []db.ListSubtasksWithMetaRow{}
	}
	glAssignees, _ := h.q.ListTaskGitlabAssignees(c, id)
	if glAssignees == nil {
		glAssignees = []db.ListTaskGitlabAssigneesRow{}
	}
	c.JSON(http.StatusOK, taskDetail{
		Task: t, Tags: orEmptyTags(tags), Assignees: assignees, GitlabAssignees: glAssignees,
		Subtasks: subtasks, GitLab: h.gitlabLinkForTask(c, id),
	})
}

// UpdateTask edits a task's fields. `completed` toggles completed_at.
func (h *API) UpdateTask(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	t, wsID, ok := h.loadTask(c, id)
	if !ok {
		return
	}
	var req struct {
		Title       string     `json:"title" binding:"required"`
		Description string     `json:"description"`
		Priority    int32      `json:"priority"`
		DueDate     *time.Time `json:"due_date"`
		Completed   bool       `json:"completed"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Preserve the original completion timestamp; set/clear on toggle.
	completedAt := t.CompletedAt
	switch {
	case req.Completed && completedAt == nil:
		now := time.Now()
		completedAt = &now
	case !req.Completed:
		completedAt = nil
	}

	updated, err := h.q.UpdateTask(c, db.UpdateTaskParams{
		ID: id, Title: req.Title, Description: req.Description,
		Priority: req.Priority, DueDate: req.DueDate, CompletedAt: completedAt,
	})
	if err != nil {
		fail(c)
		return
	}
	// A manual due-date change on a GitLab-linked task wins over the sync.
	if !sameTime(t.DueDate, updated.DueDate) {
		_ = h.q.MarkGitlabDueOverridden(c, id)
	}
	changes := h.journalUpdate(c, t, updated)
	if len(changes) > 0 {
		h.notifyTaskParticipants(c, updated, wsID, "updated",
			fmt.Sprintf("%s изменил(а) задачу #%s: %s",
				h.actorName(c), taskRef(updated.Number), strings.Join(changes, ", ")))
	}
	h.broadcast(wsID, "task.updated", updated)
	c.JSON(http.StatusOK, updated)
}

// journalUpdate records the field-level changes of a task edit into its journal
// and returns a short human list of what changed (for a notification summary).
func (h *API) journalUpdate(c *gin.Context, before, after db.Task) []string {
	var changed []string
	if before.Title != after.Title {
		h.logEvent(c, after.ID, "renamed", map[string]any{"from": before.Title, "to": after.Title})
		changed = append(changed, "название")
	}
	if before.Description != after.Description {
		h.logEvent(c, after.ID, "description", nil)
		changed = append(changed, "описание")
	}
	if before.Priority != after.Priority {
		h.logEvent(c, after.ID, "priority", map[string]any{"from": before.Priority, "to": after.Priority})
		changed = append(changed, "приоритет")
	}
	if !sameTime(before.DueDate, after.DueDate) {
		h.logEvent(c, after.ID, "due", map[string]any{"set": after.DueDate != nil})
		changed = append(changed, "срок")
	}
	switch {
	case before.CompletedAt == nil && after.CompletedAt != nil:
		h.logEvent(c, after.ID, "completed", nil)
		changed = append(changed, "выполнена")
	case before.CompletedAt != nil && after.CompletedAt == nil:
		h.logEvent(c, after.ID, "reopened", nil)
		changed = append(changed, "возвращена в работу")
	}
	return changed
}

func sameTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

// MoveTask moves a task to a column and repositions it between neighbours.
func (h *API) MoveTask(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	t, wsID, ok := h.loadTask(c, id)
	if !ok {
		return
	}
	var req struct {
		ColumnID uuid.UUID  `json:"column_id" binding:"required"`
		BeforeID *uuid.UUID `json:"before_id"`
		AfterID  *uuid.UUID `json:"after_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// Target column must be on the same board.
	col, err := h.q.GetColumn(c, req.ColumnID)
	if err != nil || col.BoardID != t.BoardID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "column does not belong to this board"})
		return
	}
	prev, next, ok := h.neighborTaskPositions(c, req.BeforeID, req.AfterID)
	if !ok {
		return
	}
	updated, err := h.q.MoveTask(c, db.MoveTaskParams{
		ID: id, ColumnID: req.ColumnID, Position: positionBetween(prev, next),
	})
	if err != nil {
		fail(c)
		return
	}

	if t.ColumnID != req.ColumnID {
		h.logEvent(c, id, "moved", map[string]any{"to": col.Name})
		h.notifyTaskParticipants(c, updated, wsID, "moved",
			fmt.Sprintf("%s переместил(а) задачу #%s → «%s»",
				h.actorName(c), taskRef(updated.Number), col.Name))
	}

	// Auto-toggle completion based on the board's configured "done" column:
	// moving in completes the task, moving out reopens it.
	if board, berr := h.q.GetBoard(c, t.BoardID); berr == nil {
		doneID := h.doneColumnID(c, board)
		targetIsDone := doneID != nil && *doneID == req.ColumnID
		sourceIsDone := doneID != nil && *doneID == t.ColumnID
		switch {
		case targetIsDone && updated.CompletedAt == nil:
			now := time.Now()
			if done, derr := h.q.UpdateTask(c, db.UpdateTaskParams{
				ID: updated.ID, Title: updated.Title, Description: updated.Description,
				Priority: updated.Priority, DueDate: updated.DueDate, CompletedAt: &now,
			}); derr == nil {
				updated = done
				h.logEvent(c, id, "completed", nil)
			}
		case sourceIsDone && !targetIsDone && updated.CompletedAt != nil:
			if reopened, derr := h.q.UpdateTask(c, db.UpdateTaskParams{
				ID: updated.ID, Title: updated.Title, Description: updated.Description,
				Priority: updated.Priority, DueDate: updated.DueDate, CompletedAt: nil,
			}); derr == nil {
				updated = reopened
				h.logEvent(c, id, "reopened", nil)
			}
		}
	}

	h.broadcast(wsID, "task.moved", updated)
	c.JSON(http.StatusOK, updated)
}

// TransferTask moves a task (and its subtasks) to another board/column within
// the same workspace; it becomes a top-level card on the target board.
func (h *API) TransferTask(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	_, wsID, ok := h.loadTask(c, id)
	if !ok {
		return
	}
	var req struct {
		BoardID  uuid.UUID  `json:"board_id" binding:"required"`
		ColumnID *uuid.UUID `json:"column_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	targetWs, err := h.q.WorkspaceIDForBoard(c, req.BoardID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid target board"})
		return
	}
	if targetWs != wsID {
		c.JSON(http.StatusForbidden, gin.H{"error": "target board is in another workspace"})
		return
	}
	var columnID uuid.UUID
	if req.ColumnID != nil {
		col, err := h.q.GetColumn(c, *req.ColumnID)
		if err != nil || col.BoardID != req.BoardID {
			c.JSON(http.StatusBadRequest, gin.H{"error": "column does not belong to the target board"})
			return
		}
		columnID = *req.ColumnID
	} else {
		cols, err := h.q.ListColumns(c, req.BoardID)
		if err != nil {
			fail(c)
			return
		}
		if len(cols) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "target board has no columns"})
			return
		}
		columnID = cols[0].ID
	}
	max, err := h.q.MaxTaskPositionInColumn(c, columnID)
	if err != nil {
		fail(c)
		return
	}
	updated, err := h.q.TransferTask(c, db.TransferTaskParams{
		ID: id, BoardID: req.BoardID, ColumnID: columnID, Position: positionBetween(&max, nil),
	})
	if err != nil {
		fail(c)
		return
	}
	if err := h.q.MoveSubtasksToBoard(c, db.MoveSubtasksToBoardParams{
		ParentID: &id, BoardID: req.BoardID, ColumnID: columnID,
	}); err != nil {
		fail(c)
		return
	}
	h.broadcast(wsID, "task.moved", updated)
	c.JSON(http.StatusOK, updated)
}

// ArchiveTask soft-deletes a task. ?subtasks=detach keeps the subtasks on the
// board (re-parented to null); otherwise they are archived with the parent.
func (h *API) ArchiveTask(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	t, wsID, ok := h.loadTask(c, id)
	if !ok {
		return
	}
	if c.Query("subtasks") == "detach" {
		if err := h.q.DetachChildren(c, &id); err != nil {
			fail(c)
			return
		}
		if err := h.q.ArchiveTask(c, id); err != nil {
			fail(c)
			return
		}
	} else if err := h.q.ArchiveTaskCascade(c, id); err != nil {
		fail(c)
		return
	}
	h.logEvent(c, id, "archived", nil)
	h.notifyTaskParticipants(c, t, wsID, "archived",
		fmt.Sprintf("%s архивировал(а) задачу #%s%s", h.actorName(c), taskRef(t.Number), shortCtx(t.Title)))
	h.broadcast(wsID, "task.archived", gin.H{"id": id})
	c.Status(http.StatusNoContent)
}

// RestoreTask un-archives a task (and any subtasks archived with it).
func (h *API) RestoreTask(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	_, wsID, ok := h.loadTask(c, id)
	if !ok {
		return
	}
	if err := h.q.RestoreTask(c, id); err != nil {
		fail(c)
		return
	}
	h.logEvent(c, id, "restored", nil)
	h.broadcast(wsID, "task.restored", gin.H{"id": id})
	c.Status(http.StatusNoContent)
}

// ListBoardArchived returns a board's archived (soft-deleted) top-level tasks.
func (h *API) ListBoardArchived(c *gin.Context) {
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
	rows, err := h.q.ListBoardArchivedWithMeta(c, boardID)
	if err != nil {
		fail(c)
		return
	}
	c.JSON(http.StatusOK, rows)
}

// DeleteTask removes a task and its subtasks.
func (h *API) DeleteTask(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	_, wsID, ok := h.loadTask(c, id)
	if !ok {
		return
	}
	// ?subtasks=detach keeps subtasks (re-parented to null); default cascades.
	if c.Query("subtasks") == "detach" {
		if err := h.q.DetachChildren(c, &id); err != nil {
			fail(c)
			return
		}
	}
	if err := h.q.DeleteTask(c, id); err != nil {
		fail(c)
		return
	}
	h.broadcast(wsID, "task.deleted", gin.H{"id": id})
	c.Status(http.StatusNoContent)
}

// ── Task tags / assignees ──────────────────────────────────

// AddTaskTag attaches a tag to a task.
func (h *API) AddTaskTag(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	_, wsID, ok := h.loadTask(c, id)
	if !ok {
		return
	}
	var req struct {
		TagID uuid.UUID `json:"tag_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.q.AddTaskTag(c, db.AddTaskTagParams{TaskID: id, TagID: req.TagID}); err != nil {
		fail(c)
		return
	}
	h.broadcast(wsID, "task.tagged", gin.H{"task_id": id, "tag_id": req.TagID})
	c.Status(http.StatusNoContent)
}

// RemoveTaskTag detaches a tag from a task.
func (h *API) RemoveTaskTag(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	_, wsID, ok := h.loadTask(c, id)
	if !ok {
		return
	}
	tagID, ok := parseID(c, "tagId")
	if !ok {
		return
	}
	if err := h.q.RemoveTaskTag(c, db.RemoveTaskTagParams{TaskID: id, TagID: tagID}); err != nil {
		fail(c)
		return
	}
	h.broadcast(wsID, "task.untagged", gin.H{"task_id": id, "tag_id": tagID})
	c.Status(http.StatusNoContent)
}

// AddTaskAssignee assigns a user to a task.
func (h *API) AddTaskAssignee(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	_, wsID, ok := h.loadTask(c, id)
	if !ok {
		return
	}
	var req struct {
		UserID uuid.UUID `json:"user_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.q.AddTaskAssignee(c, db.AddTaskAssigneeParams{TaskID: id, UserID: req.UserID}); err != nil {
		fail(c)
		return
	}
	t, _, _ := h.loadTask(c, id)
	h.logEvent(c, id, "assigned", map[string]any{"user_id": req.UserID})
	h.notify(c, req.UserID, wsID, &id, "assigned",
		fmt.Sprintf("%s назначил вам задачу #%s%s", h.actorName(c), taskRef(t.Number), shortCtx(t.Title)))
	h.broadcast(wsID, "task.assigned", gin.H{"task_id": id, "user_id": req.UserID})
	c.Status(http.StatusNoContent)
}

// RemoveTaskAssignee unassigns a user from a task.
func (h *API) RemoveTaskAssignee(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	_, wsID, ok := h.loadTask(c, id)
	if !ok {
		return
	}
	userID, ok := parseID(c, "userId")
	if !ok {
		return
	}
	if err := h.q.RemoveTaskAssignee(c, db.RemoveTaskAssigneeParams{TaskID: id, UserID: userID}); err != nil {
		fail(c)
		return
	}
	h.logEvent(c, id, "unassigned", map[string]any{"user_id": userID})
	h.broadcast(wsID, "task.unassigned", gin.H{"task_id": id, "user_id": userID})
	c.Status(http.StatusNoContent)
}

// ── helpers ────────────────────────────────────────────────

// loadTask fetches a task and authorizes the caller against its workspace.
func (h *API) loadTask(c *gin.Context, id uuid.UUID) (db.Task, uuid.UUID, bool) {
	t, err := h.q.GetTask(c, id)
	if notFound(c, err) {
		return db.Task{}, uuid.Nil, false
	}
	if err != nil {
		fail(c)
		return db.Task{}, uuid.Nil, false
	}
	wsID, err := h.q.WorkspaceIDForBoard(c, t.BoardID)
	if err != nil {
		fail(c)
		return db.Task{}, uuid.Nil, false
	}
	if !h.requireMember(c, wsID) {
		return db.Task{}, uuid.Nil, false
	}
	return t, wsID, true
}

// nextTaskPosition appends to the end of a column (top-level) or a parent's
// subtask list.
func (h *API) nextTaskPosition(c *gin.Context, columnID uuid.UUID, parentID *uuid.UUID) (float64, error) {
	if parentID != nil {
		subs, err := h.q.ListSubtasks(c, parentID)
		if err != nil {
			return 0, err
		}
		if len(subs) == 0 {
			return positionGap, nil
		}
		last := subs[len(subs)-1].Position
		return positionBetween(&last, nil), nil
	}
	max, err := h.q.MaxTaskPositionInColumn(c, columnID)
	if err != nil {
		return 0, err
	}
	return positionBetween(&max, nil), nil
}

// neighborTaskPositions resolves the positions of the before/after tasks.
func (h *API) neighborTaskPositions(c *gin.Context, beforeID, afterID *uuid.UUID) (prev, next *float64, ok bool) {
	if beforeID != nil {
		t, err := h.q.GetTask(c, *beforeID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid before_id"})
			return nil, nil, false
		}
		prev = &t.Position
	}
	if afterID != nil {
		t, err := h.q.GetTask(c, *afterID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid after_id"})
			return nil, nil, false
		}
		next = &t.Position
	}
	return prev, next, true
}

func orEmptyTags(tags []db.Tag) []db.Tag {
	if tags == nil {
		return []db.Tag{}
	}
	return tags
}
