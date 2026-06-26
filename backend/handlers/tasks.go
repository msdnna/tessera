package handlers

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/db"
	"tessera/internal/recur"
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
		StartDate   *time.Time `json:"start_date"`
		Estimate    *float64   `json:"estimate"`
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
		StartDate:   req.StartDate,
		Estimate:    normalizeEstimate(req.Estimate),
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

// GetTaskByNumber resolves a per-workspace task number (the #252 on cards) to
// its task — backs human-readable deep links (?task=<number>). Returns the task
// row; the client then opens it (which loads full detail by id).
func (h *API) GetTaskByNumber(c *gin.Context) {
	wsID, ok := parseID(c, "id")
	if !ok || !h.requireMember(c, wsID) {
		return
	}
	n, err := strconv.ParseInt(c.Param("number"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task number"})
		return
	}
	t, err := h.q.GetTaskByNumber(c, db.GetTaskByNumberParams{WorkspaceID: wsID, Number: &n})
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c)
		return
	}
	c.JSON(http.StatusOK, t)
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
		Title       string           `json:"title" binding:"required"`
		Description string           `json:"description"`
		Priority    int32            `json:"priority"`
		DueDate     *time.Time       `json:"due_date"`
		StartDate   *time.Time       `json:"start_date"`
		Estimate    *float64         `json:"estimate"`
		Completed   bool             `json:"completed"`
		Recurrence  *json.RawMessage `json:"recurrence"`
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

	// Normalise the recurrence rule and manage its anchor (or NULL if invalid).
	recurrence := recurrenceToStore(req.Recurrence, req.DueDate, t.Recurrence, t.DueDate)

	updated, err := h.q.UpdateTask(c, db.UpdateTaskParams{
		ID: id, Title: req.Title, Description: req.Description,
		Priority: req.Priority, DueDate: req.DueDate, CompletedAt: completedAt,
		Recurrence: recurrence, StartDate: req.StartDate, Estimate: normalizeEstimate(req.Estimate),
	})
	if err != nil {
		fail(c)
		return
	}

	// A recurring task whose trigger is "complete" advances when it enters the
	// completed state (rescheduled, or duplicated, per its rule).
	if t.CompletedAt == nil && updated.CompletedAt != nil {
		if advanced, ok := h.recurAdvance(c, updated, wsID, middleware.CurrentUser(c), recur.TriggerComplete); ok {
			updated = advanced
		}
	}
	// A manual due-date change on a GitLab-linked task wins over the sync.
	if !sameTime(t.DueDate, updated.DueDate) {
		_ = h.q.MarkGitlabDueOverridden(c, id)
	}
	// Likewise a manual start-date change wins over the sync.
	if !sameTime(t.StartDate, updated.StartDate) {
		_ = h.q.MarkGitlabStartOverridden(c, id)
	}
	// A manual estimate change wins over the GitLab timeEstimate pull.
	if !sameEstimate(t.Estimate, updated.Estimate) {
		_ = h.q.MarkGitlabEstimateOverridden(c, id)
	}
	changes := h.journalUpdate(c, t, updated)
	if len(changes) > 0 {
		h.notifyTaskParticipants(c, updated, wsID, "updated",
			fmt.Sprintf("%s изменил(а) задачу #%s: %s",
				h.actorName(c), taskRef(updated.Number), strings.Join(changes, ", ")))
	}
	// Mirror user-side changes back to a linked GitLab issue (opt-in per integration).
	actor := middleware.CurrentUser(c)
	if (t.CompletedAt == nil) != (updated.CompletedAt == nil) {
		h.enqueueWriteback(c, id, actor, "state", map[string]any{"state": issueState(updated.CompletedAt != nil)})
	}
	if t.Priority != updated.Priority {
		h.enqueueWriteback(c, id, actor, "priority", map[string]any{"priority": updated.Priority})
	}
	// Due-date push reads the latest task state at push time, so an empty payload
	// is fine (also lets a burst of edits coalesce to one pending row).
	if !sameTime(t.DueDate, updated.DueDate) {
		h.enqueueWriteback(c, id, actor, "due", map[string]any{})
	}
	if !sameEstimate(t.Estimate, updated.Estimate) {
		h.enqueueWriteback(c, id, actor, "estimate", map[string]any{})
	}
	h.broadcast(wsID, "task.updated", updated)
	c.JSON(http.StatusOK, updated)
}

// issueState maps a completion flag to the GitLab issue state string.
func issueState(completed bool) string {
	if completed {
		return "closed"
	}
	return "opened"
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
	if !sameTime(before.StartDate, after.StartDate) {
		h.logEvent(c, after.ID, "start", map[string]any{"set": after.StartDate != nil})
		changed = append(changed, "начало")
	}
	if !sameEstimate(before.Estimate, after.Estimate) {
		h.logEvent(c, after.ID, "estimate", map[string]any{"set": after.Estimate != nil})
		changed = append(changed, "оценка")
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

func sameEstimate(a, b *float64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// normalizeEstimate rejects non-positive / non-finite estimates, mapping them to
// NULL (unestimated) so a stray 0 or negative never persists as a real value.
func normalizeEstimate(v *float64) *float64 {
	if v == nil || *v <= 0 || math.IsNaN(*v) || math.IsInf(*v, 0) {
		return nil
	}
	return v
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
				Recurrence: updated.Recurrence, StartDate: updated.StartDate, Estimate: updated.Estimate,
			}); derr == nil {
				updated = done
				h.logEvent(c, id, "completed", nil)
				// A "complete"-triggered recurring task bounces straight back out of
				// done with its due date advanced — overriding the move just made.
				if advanced, ok := h.recurAdvance(c, updated, wsID, middleware.CurrentUser(c), recur.TriggerComplete); ok {
					updated = advanced
				}
			}
		case sourceIsDone && !targetIsDone && updated.CompletedAt != nil:
			if reopened, derr := h.q.UpdateTask(c, db.UpdateTaskParams{
				ID: updated.ID, Title: updated.Title, Description: updated.Description,
				Priority: updated.Priority, DueDate: updated.DueDate, CompletedAt: nil,
				Recurrence: updated.Recurrence, StartDate: updated.StartDate, Estimate: updated.Estimate,
			}); derr == nil {
				updated = reopened
				h.logEvent(c, id, "reopened", nil)
			}
		}
	}

	// Column-triggered recurrence: moving the task into its configured column
	// advances it (independent of completion).
	if t.ColumnID != req.ColumnID {
		if rule, ok := recur.Parse(updated.Recurrence); ok &&
			rule.Trigger == recur.TriggerColumn && rule.TriggerColumn == req.ColumnID.String() {
			if advanced, acted := h.recurAdvance(c, updated, wsID, middleware.CurrentUser(c), recur.TriggerColumn); acted {
				updated = advanced
			}
		}
	}

	// Mirror a completion change (crossing the board's done boundary) back to GitLab.
	if (t.CompletedAt == nil) != (updated.CompletedAt == nil) {
		h.enqueueWriteback(c, id, middleware.CurrentUser(c), "state",
			map[string]any{"state": issueState(updated.CompletedAt != nil)})
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
	maxPos, err := h.q.MaxTaskPositionInColumn(c, columnID)
	if err != nil {
		fail(c)
		return
	}
	updated, err := h.q.TransferTask(c, db.TransferTaskParams{
		ID: id, BoardID: req.BoardID, ColumnID: columnID, Position: positionBetween(&maxPos, nil),
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
	// Reconcile labels back to the linked issue (the worker reads the task's
	// current tags, so the empty payload coalesces a burst into one row).
	h.enqueueWriteback(c, id, middleware.CurrentUser(c), "labels", map[string]any{})
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
	h.enqueueWriteback(c, id, middleware.CurrentUser(c), "labels", map[string]any{})
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
	h.enqueueWriteback(c, id, middleware.CurrentUser(c), "assignees", map[string]any{})
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
	h.enqueueWriteback(c, id, middleware.CurrentUser(c), "assignees", map[string]any{})
	c.Status(http.StatusNoContent)
}

// SetTaskEisenhower pins a task to an Eisenhower-matrix quadrant (0-3), or clears
// the override (quadrant null → the matrix derives it from priority + due-date).
// Driven by the matrix view's drag-between-quadrants and «сбросить на авто».
func (h *API) SetTaskEisenhower(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	_, wsID, ok := h.loadTask(c, id)
	if !ok {
		return
	}
	var req struct {
		Quadrant *int16 `json:"quadrant"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Quadrant != nil && (*req.Quadrant < 0 || *req.Quadrant > 3) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "quadrant must be 0-3 or null"})
		return
	}
	t, err := h.q.SetTaskEisenhower(c, db.SetTaskEisenhowerParams{ID: id, EisenhowerQuadrant: req.Quadrant})
	if err != nil {
		fail(c)
		return
	}
	h.broadcast(wsID, "task.updated", t)
	c.JSON(http.StatusOK, t)
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
	maxPos, err := h.q.MaxTaskPositionInColumn(c, columnID)
	if err != nil {
		return 0, err
	}
	return positionBetween(&maxPos, nil), nil
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

// recurrenceToStore validates a client-supplied recurrence rule and returns its
// canonical JSON form (or nil for an absent/invalid rule). For monthly/yearly it
// manages the day-of-month anchor: the anchor is re-derived from the due date
// when the user changes the due date or the frequency, and otherwise carried over
// from the previously stored rule — so unrelated edits don't reset an anchor that
// a short-month clamp has temporarily moved (e.g. the 30th showing as Feb 28).
func recurrenceToStore(reqRaw *json.RawMessage, reqDue *time.Time, prevRaw *json.RawMessage, prevDue *time.Time) *json.RawMessage {
	rule, ok := recur.Parse(reqRaw)
	if !ok {
		return nil
	}
	if rule.Freq == recur.FreqMonthly || rule.Freq == recur.FreqYearly {
		if prev, hadPrev := recur.Parse(prevRaw); hadPrev && prev.Freq == rule.Freq && sameTime(reqDue, prevDue) {
			rule.Day, rule.Month = prev.Day, prev.Month
		} else if reqDue != nil {
			rule = rule.WithAnchor(*reqDue)
		}
	}
	canon, _ := rule.Marshal()
	return canon
}

func orEmptyTags(tags []db.Tag) []db.Tag {
	if tags == nil {
		return []db.Tag{}
	}
	return tags
}
