package handlers

import (
	"encoding/json"
	"math"
	"net/http"
	"strconv"
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
	h.broadcastAs(c, wsID, "task.created", t)
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
	// Optional milestone scope (sprint navigation): ?milestone=<slug|uuid> shows
	// one sprint, ?milestone=backlog shows tasks with no milestone, absent shows
	// all. UUIDs stay accepted so links shared before slugs existed keep working.
	// ?archived=1 returns the board's archived tasks (read-only archive view).
	params := db.ListBoardTasksWithMetaParams{BoardID: boardID, Archived: c.Query("archived") == "1"}
	switch m := c.Query("milestone"); {
	case m == backlogScope:
		params.Backlog = true
	case m != "":
		params.MilestoneID = h.resolveMilestoneScope(c, boardID, m)
	}
	tasks, err := h.q.ListBoardTasksWithMeta(c, params)
	if err != nil {
		fail(c)
		return
	}
	c.JSON(http.StatusOK, slimBoardTasks(tasks))
}

// backlogScope is the reserved ?milestone= value for "tasks with no milestone".
const backlogScope = "backlog"

// resolveMilestoneScope turns a ?milestone= value into a milestone id: a UUID is
// used as-is, anything else is looked up as a slug within the board's project.
// An unresolvable value yields uuid.Nil — a broken or stale link then shows an
// empty scoped board, exactly as a deleted milestone's UUID does today, instead
// of silently falling back to the unfiltered board.
func (h *API) resolveMilestoneScope(c *gin.Context, boardID uuid.UUID, scope string) *uuid.UUID {
	if mid, err := uuid.Parse(scope); err == nil {
		return &mid
	}
	unmatched := uuid.Nil
	projectID, err := h.q.ProjectIDForBoard(c, boardID)
	if err != nil {
		return &unmatched
	}
	m, err := h.q.GetMilestoneInProjectBySlug(c, db.GetMilestoneInProjectBySlugParams{ProjectID: projectID, Slug: scope})
	if err != nil {
		return &unmatched
	}
	return &m.ID
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
	c.JSON(http.StatusOK, slimBoardSubtasks(subs))
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
	updated, err := h.applyParent(c, t, wsID, req.ParentID)
	if err != nil {
		respondOpError(c, err)
		return
	}
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

// GetTaskDescription returns just a task's description markdown. The board lists
// omit descriptions to stay small (see task_list_dto.go); the card fetches this
// lazily when the user hovers the description affordance. One membership check +
// one row read — far lighter than GetTask's full tags/assignees/subtasks fan-out.
func (h *API) GetTaskDescription(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	t, _, ok := h.loadTask(c, id)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": t.ID, "description": t.Description})
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
		Description *string          `json:"description"`
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

	// Full-replace for the fields present in the body — a quick action sets only
	// what it touches. Description is the one exception: it's tri-state. Board
	// cards / timeline / gantt no longer receive descriptions (they're stripped
	// from the list payloads to keep boards small), so those inline edits OMIT
	// description and it must be preserved, not blanked. Omitted (nil) → keep the
	// stored text; present (incl. "") → replace, so the modal can still clear it.
	updated, err := h.applyTaskPatch(c, t, wsID, taskPatch{
		Title:       &req.Title,
		Description: req.Description,
		Priority:    &req.Priority,
		DueDate:     setTime(req.DueDate),
		StartDate:   setTime(req.StartDate),
		Estimate:    setFloat(req.Estimate),
		Completed:   &req.Completed,
		Recurrence:  req.Recurrence, RecurrenceSet: true,
	})
	if err != nil {
		respondOpError(c, err)
		return
	}
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
	prev, next, ok := h.neighborTaskPositions(c, req.BeforeID, req.AfterID)
	if !ok {
		return
	}
	updated, err := h.applyMove(c, t, wsID, req.ColumnID, positionBetween(prev, next))
	if err != nil {
		respondOpError(c, err)
		return
	}
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
	// Transactional: the task and its subtasks must land on the new board
	// together — otherwise a crash strands subtasks on the old board.
	var updated db.Task
	if err := h.inTx(c, func(q *db.Queries) error {
		var err error
		updated, err = q.TransferTask(c, db.TransferTaskParams{
			ID: id, BoardID: req.BoardID, ColumnID: columnID, Position: positionBetween(&maxPos, nil),
		})
		if err != nil {
			return err
		}
		return q.MoveSubtasksToBoard(c, db.MoveSubtasksToBoardParams{
			ParentID: &id, BoardID: req.BoardID, ColumnID: columnID,
		})
	}); err != nil {
		fail(c)
		return
	}
	h.broadcastAs(c, wsID, "task.moved", updated)
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
	if err := h.applyArchive(c, t, wsID, c.Query("subtasks") == "detach"); err != nil {
		respondOpError(c, err)
		return
	}
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
	c.JSON(http.StatusOK, slimBoardArchivedTasks(rows))
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
	if err := h.applyTag(c, id, wsID, req.TagID, true); err != nil {
		respondOpError(c, err)
		return
	}
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
	if err := h.applyTag(c, id, wsID, tagID, false); err != nil {
		respondOpError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// AddTaskAssignee assigns a user to a task.
func (h *API) AddTaskAssignee(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	t, wsID, ok := h.loadTask(c, id)
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
	if err := h.applyAssignee(c, t, wsID, req.UserID, true); err != nil {
		respondOpError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// RemoveTaskAssignee unassigns a user from a task.
func (h *API) RemoveTaskAssignee(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	t, wsID, ok := h.loadTask(c, id)
	if !ok {
		return
	}
	userID, ok := parseID(c, "userId")
	if !ok {
		return
	}
	if err := h.applyAssignee(c, t, wsID, userID, false); err != nil {
		respondOpError(c, err)
		return
	}
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
