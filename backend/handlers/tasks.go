package handlers

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/db"
	"tessera/internal/gitlab"
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
		fail(c, err)
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

	// binding:"required" only rejects the empty string — a title of pure
	// whitespace normalizes to nothing and must be refused here.
	req.Title = normalizeTitle(req.Title)
	if req.Title == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "заголовок не может быть пустым"})
		return
	}

	// The column must belong to this board.
	col, err := h.q.GetColumn(c, req.ColumnID)
	if err != nil || col.BoardID != boardID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "column does not belong to this board"})
		return
	}

	// …and so must the parent, if one is named: parent_id is a free-form task id
	// from the body, so without this a member of one workspace could hang a task
	// off a task in another and have it show up among that task's subtasks.
	if req.ParentID != nil {
		parent, err := h.q.GetTask(c, *req.ParentID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid parent"})
			return
		}
		if err := h.sameWorkspaceTask(c, parent, wsID); err != nil {
			respondOpError(c, err)
			return
		}
		// The parent must stand on THIS board, the one the URL names — a subtask
		// living anywhere else is drawn nowhere. The two board queries split the
		// work by the parent's archived state, not by its board:
		// ListBoardTasksWithMeta drops the child (it has a parent the same view
		// would draw, so it is not "top-level for this view") while
		// ListBoardSubtasksWithMeta serves it (its board_id matches and the parent
		// is active) — and the board files a subtask under a parent card that is
		// not in the payload, so nothing ever renders it. On the parent's board it
		// is not fetched at all. The other doors onto this pair already hold the
		// invariant: applyParent adopts the parent's board and carries the subtree
		// with it, /transfer clears parent_id and moves the subtree, and only the
		// column check above — not this one — used to stand here.
		if parent.BoardID != boardID {
			c.JSON(http.StatusBadRequest, gin.H{"error": "parent task belongs to another board"})
			return
		}
	}

	pos, err := h.nextTaskPosition(c, req.ColumnID, req.ParentID)
	if err != nil {
		fail(c, err)
		return
	}

	num, err := h.q.NextWorkspaceTaskNumber(c, wsID)
	if err != nil {
		fail(c, err)
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
		fail(c, err)
		return
	}
	h.logEvent(c, t.ID, "created", nil)
	// A subtask of a grouped, linked parent is mirrored into GitLab as a child work
	// item (#2592). Queued, never inline: GitLab must not be able to slow down — or
	// fail — task creation.
	if req.ParentID != nil {
		h.enqueueChildAttach(c, t.ID, *req.ParentID, uid)
	}
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
		fail(c, err)
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
		fail(c, err)
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
		fail(c, err)
		return
	}
	if !h.requireMember(c, wsID) {
		return
	}
	subs, err := h.q.ListBoardSubtasksWithMeta(c, boardID)
	if err != nil {
		fail(c, err)
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
		fail(c, err)
		return
	}
	assignees, err := h.q.ListTaskAssignees(c, id)
	if err != nil {
		fail(c, err)
		return
	}
	subtasks, err := h.q.ListSubtasksWithMeta(c, &id)
	if err != nil {
		fail(c, err)
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
		fail(c, err)
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
		Title       optJSON[string]          `json:"title"`
		Description optJSON[string]          `json:"description"`
		Priority    optJSON[int32]           `json:"priority"`
		DueDate     optJSON[time.Time]       `json:"due_date"`
		StartDate   optJSON[time.Time]       `json:"start_date"`
		Estimate    optJSON[float64]         `json:"estimate"`
		Completed   optJSON[bool]            `json:"completed"`
		Recurrence  optJSON[json.RawMessage] `json:"recurrence"`
		// UpdatedAt opts this edit into the optimistic lock: send back the
		// updated_at of the task version the edit was made against and the write
		// is refused with 409 if anyone landed one in between. Omit it and the
		// endpoint behaves exactly as before (last write wins).
		UpdatedAt *time.Time `json:"updated_at"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Tri-state PATCH: a field the body does not carry is left alone. It used to
	// be full-replace, and the dangerous field was `completed bool` — a client
	// that omitted it un-completed the task, which also clears completed_at,
	// advances recurrence and reopens the linked GitLab issue. Both clients send
	// whole objects today, so this is backwards compatible; what it buys is that
	// a *partial* edit (an inline board rename, a future quick action) can no
	// longer blank a field it never mentioned.
	//
	// Title keeps its non-empty check in applyTaskPatch rather than a `required`
	// binding tag: absent is now legal, empty is still not.
	updated, err := h.applyTaskPatch(c, t, wsID, taskPatch{
		Title:       req.Title.ptr(),
		Description: req.Description.ptr(),
		Priority:    req.Priority.ptr(),
		DueDate:     asTime(req.DueDate),
		StartDate:   asTime(req.StartDate),
		Estimate:    asFloat(req.Estimate),
		Completed:   req.Completed.ptr(),
		Recurrence:  req.Recurrence.ptr(), RecurrenceSet: req.Recurrence.set,
		ExpectedUpdatedAt: req.UpdatedAt,
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
// and returns the machine names of what changed (for the notification payload;
// fieldWords turns them into Russian for the legacy text, the client into the
// reader's language).
func (h *API) journalUpdate(c *gin.Context, before, after db.Task) []string {
	var changed []string
	if before.Title != after.Title {
		h.logEvent(c, after.ID, "renamed", map[string]any{"from": before.Title, "to": after.Title})
		changed = append(changed, "title")
	}
	if before.Description != after.Description {
		h.logEvent(c, after.ID, "description", nil)
		changed = append(changed, "description")
	}
	if before.Priority != after.Priority {
		h.logEvent(c, after.ID, "priority", map[string]any{"from": before.Priority, "to": after.Priority})
		changed = append(changed, "priority")
	}
	if !sameTime(before.DueDate, after.DueDate) {
		h.logEvent(c, after.ID, "due", map[string]any{"set": after.DueDate != nil})
		changed = append(changed, "due")
	}
	if !sameTime(before.StartDate, after.StartDate) {
		h.logEvent(c, after.ID, "start", map[string]any{"set": after.StartDate != nil})
		changed = append(changed, "start")
	}
	if !sameEstimate(before.Estimate, after.Estimate) {
		h.logEvent(c, after.ID, "estimate", map[string]any{"set": after.Estimate != nil})
		changed = append(changed, "estimate")
	}
	switch {
	case before.CompletedAt == nil && after.CompletedAt != nil:
		h.logEvent(c, after.ID, "completed", nil)
		changed = append(changed, "completed")
	case before.CompletedAt != nil && after.CompletedAt == nil:
		h.logEvent(c, after.ID, "reopened", nil)
		changed = append(changed, "reopened")
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

// normalizeTitle collapses every whitespace run in a title — newlines, tabs,
// doubled spaces — into one space and trims the ends. A title is single-line by
// contract: the web card renders it as HTML (a newline shows as a space) while
// the modal keeps it in an <input>, which strips newlines out of value, so a
// stored "\n" makes the same task read differently in the two places (#2813).
// Every write path goes through here, so no client — web, Android, MCP or
// curl — can put a newline back in.
func normalizeTitle(s string) string {
	return strings.Join(strings.Fields(s), " ")
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
	if rejectSameNeighbour(c, req.BeforeID, req.AfterID) {
		return
	}
	prev, next, ok := h.neighborTaskPositions(c, t, req.ColumnID, req.BeforeID, req.AfterID)
	if !ok {
		return
	}
	pos, ok := h.positionForDrop(c, t, req.ColumnID, req.BeforeID, req.AfterID, prev, next)
	if !ok {
		return
	}
	updated, err := h.applyMove(c, t, wsID, req.ColumnID, pos)
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
	t, wsID, ok := h.loadTask(c, id)
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
			fail(c, err)
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
		fail(c, err)
		return
	}
	// A transfer changes the column, so the board's done-column rule applies to it
	// exactly as it applies to a move (applyMove): what leaves a done column for a
	// column that does not complete is reopened. Without it a finished card carried
	// to another board stood struck-through in that board's first column, still
	// counted as done and with no gesture left to undo it — the reopen rule fires
	// only for a task standing in a done column, and this one no longer is.
	// Resolved before the transaction: it reads the two boards' done-column
	// pointers, and neither the transfer nor anything it calls writes those.
	leavingDone := h.doneColumnLeftBy(c, t, req.BoardID, columnID)
	// Transactional: the task and its subtasks must land on the new board
	// together — otherwise a crash strands subtasks on the old board.
	var updated db.Task
	var reopened []uuid.UUID
	var lostMilestones []db.SubtreeMilestonesOffProjectRow
	if err := h.inTx(c, func(q *db.Queries) error {
		// The target board may stand in another project, in which case the transfer
		// drops the subtree's milestones on its way past — read what it takes while
		// it is still readable (see milestonesLostToBoard). A transfer within the
		// same board is a column change and crosses nothing.
		if req.BoardID != t.BoardID {
			var err error
			if lostMilestones, err = h.milestonesLostToBoard(c, q, id, req.BoardID); err != nil {
				return err
			}
		}
		// Before the two moves below: the reopen condition is the column the task
		// is leaving, which they are about to overwrite.
		if leavingDone != nil {
			var err error
			if reopened, err = q.ReopenSubtreeLeavingDone(c, db.ReopenSubtreeLeavingDoneParams{
				RootID: id, DoneColumnID: *leavingDone,
			}); err != nil {
				return err
			}
		}
		var err error
		updated, err = q.TransferTask(c, db.TransferTaskParams{
			ID: id, BoardID: req.BoardID, ColumnID: columnID, Position: positionBetween(&maxPos, nil),
		})
		if err != nil {
			return err
		}
		return q.MoveSubtreeToBoard(c, db.MoveSubtreeToBoardParams{
			ParentID: &id, BoardID: req.BoardID, ColumnID: columnID,
		})
	}); err != nil {
		fail(c, err)
		return
	}
	h.journalLostMilestones(c, lostMilestones)
	// Same trail an in-board reopen leaves: a journal entry per task, and the
	// completion mirrored to GitLab (the outbox drops the ones with no issue).
	actor := middleware.CurrentUser(c)
	for _, rid := range reopened {
		h.logEvent(c, rid, "reopened", nil)
		h.enqueueWriteback(c, rid, actor, gitlab.TrigCompletion, map[string]any{"completed": false})
	}
	// The TransferTask statement clears parent_id — a task cannot keep a parent it
	// no longer stands beside — so a transfer IS a detach, and GitLab's hierarchy
	// has to hear it exactly as it hears the one made through /parent (#2932).
	// Left unsent, the work item stays under its parent's and the next pull takes
	// the re-parent branch, whose SetTaskParent writes parent_id, board_id and
	// column_id together: the card is dragged back under the old parent on the old
	// board, undoing the transfer through a sync the user never ran.
	if t.ParentID != nil {
		h.enqueueChildDetach(c, id, *t.ParentID, actor)
	}
	h.broadcastAs(c, wsID, "task.moved", updated)
	c.JSON(http.StatusOK, updated)
}

// doneColumnLeftBy names the done column a transfer takes the task out of, or nil
// when nothing is being left: the source board completes nothing, or the column
// the task lands in completes on the target board too (so the card stays done,
// exactly as dragging it between two done columns would).
//
// Only the SOURCE board's done column is the condition. A task completed while
// standing anywhere else keeps its flag — an in-board move leaves such a task
// alone as well, and a board with no done column does not tie completion to
// columns at all. A board that cannot be read is treated as completing nothing:
// the transfer then behaves as it always did.
func (h *API) doneColumnLeftBy(c *gin.Context, t db.Task, targetBoard, targetColumn uuid.UUID) *uuid.UUID {
	src, err := h.q.GetBoard(c, t.BoardID)
	if err != nil {
		return nil
	}
	done := doneColumnID(src)
	if done == nil {
		return nil
	}
	dst := src
	if targetBoard != t.BoardID {
		if dst, err = h.q.GetBoard(c, targetBoard); err != nil {
			return nil
		}
	}
	if d := doneColumnID(dst); d != nil && *d == targetColumn {
		return nil
	}
	return done
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
		fail(c, err)
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
		fail(c, err)
		return
	}
	if !h.requireMember(c, wsID) {
		return
	}
	rows, err := h.q.ListBoardArchivedWithMeta(c, boardID)
	if err != nil {
		fail(c, err)
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
		detached, err := h.q.DetachChildren(c, &id)
		if err != nil {
			fail(c, err)
			return
		}
		// Before the delete, while the parent's own link is still readable: a detach
		// resolves its integration from the CHILD's link, but the journal entry names
		// the parent, and the push is the same one /parent sends (see
		// pushDetachedChildren).
		h.pushDetachedChildren(c, detached, id)
	}
	gone, err := h.q.DeleteTask(c, id)
	if err != nil {
		fail(c, err)
		return
	}
	// Sweep the attachments of every task the delete actually took, subtasks
	// included — each has its own <uploadDir>/<id>/ (UploadAttachment) and its
	// rows have just gone with it, so nothing names those files any more. The
	// list comes from the delete itself rather than from a walk of the tree: a
	// ?subtasks=detach run re-parented the children a statement earlier, and
	// they keep both their rows and their files.
	h.purgeTaskUploads(gone)
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
		fail(c, err)
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
		fail(c, err)
		return db.Task{}, uuid.Nil, false
	}
	wsID, err := h.q.WorkspaceIDForBoard(c, t.BoardID)
	if err != nil {
		fail(c, err)
		return db.Task{}, uuid.Nil, false
	}
	if !h.requireMember(c, wsID) {
		return db.Task{}, uuid.Nil, false
	}
	return t, wsID, true
}

// nextTaskPosition appends to the end of a column (top-level) or a parent's
// subtask list. Takes a bare context rather than the request's: the recurrence
// engine spawns tasks off the same rule from a background trigger.
func (h *API) nextTaskPosition(ctx context.Context, columnID uuid.UUID, parentID *uuid.UUID) (float64, error) {
	if parentID != nil {
		subs, err := h.q.ListSubtasks(ctx, parentID)
		if err != nil {
			return 0, err
		}
		if len(subs) == 0 {
			return positionGap, nil
		}
		last := subs[len(subs)-1].Position
		return positionBetween(&last, nil), nil
	}
	maxPos, err := h.q.MaxTaskPositionInColumn(ctx, columnID)
	if err != nil {
		return 0, err
	}
	return positionBetween(&maxPos, nil), nil
}

// sameList reports whether [n] is a sibling of the card being dropped — i.e. it
// belongs to the list the drop lands in, so its position is on the same ladder as
// the one being computed. A subtask orders itself among its parent's children, a
// top-level card among the top-level cards of the destination column.
func sameList(n, moved db.Task, columnID uuid.UUID) bool {
	if moved.ParentID != nil {
		return n.ParentID != nil && *n.ParentID == *moved.ParentID
	}
	return n.ParentID == nil && n.ColumnID == columnID
}

// neighborTaskPositions resolves the positions of the before/after tasks. Both
// must be siblings of [moved] at the destination (see sameList); anything else is
// rejected with the same 400 an unknown id gets.
//
// Positions are per-list float8 ladders — a column's cards are numbered with no
// regard for any other column — so a neighbour from elsewhere hands the midpoint
// arithmetic a number that means nothing in the destination, and the card lands
// at an arbitrary slot with no error anywhere. The ids come from the client and
// really do go stale: both clients deliberately freeze the board for the duration
// of a drag (web `suppress()`, Android `dragging`), so a colleague moving the
// card you are aiming next to reaches you only after the drop. Deleted neighbours
// were always answered honestly (GetTask fails → 400 → the board reloads); a
// neighbour that merely moved away was the silent case.
//
// It also stops a bare id from reporting whether a task exists in a workspace the
// caller has no membership in — GetTask is scoped by nothing but the id (#2932).
func (h *API) neighborTaskPositions(c *gin.Context, moved db.Task, columnID uuid.UUID, beforeID, afterID *uuid.UUID) (prev, next *float64, ok bool) {
	if beforeID != nil {
		t, err := h.q.GetTask(c, *beforeID)
		if err != nil || !sameList(t, moved, columnID) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid before_id"})
			return nil, nil, false
		}
		prev = &t.Position
	}
	if afterID != nil {
		t, err := h.q.GetTask(c, *afterID)
		if err != nil || !sameList(t, moved, columnID) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid after_id"})
			return nil, nil, false
		}
		next = &t.Position
	}
	return prev, next, true
}

// positionForDrop is positionBetween with a way out of an exhausted slot: when
// the midpoint collapses onto a neighbour, the sibling list is respaced back
// onto the positionGap ladder and the midpoint recomputed against the fresh
// neighbour positions. Only the list the card is being dropped into is touched
// (the destination column, or the parent's subtasks for a child card) — which is
// what this needs over the generic positionForReorder that columns, projects and
// groups use: which list to respace depends on the card, not on the endpoint.
func (h *API) positionForDrop(c *gin.Context, t db.Task, columnID uuid.UUID, beforeID, afterID *uuid.UUID, prev, next *float64) (float64, bool) {
	if beforeID == nil && afterID == nil {
		return h.positionForUnanchoredMove(c, t, columnID)
	}
	pos := positionBetween(prev, next)
	if !positionExhausted(pos, prev, next) {
		return pos, true
	}
	var err error
	if t.ParentID != nil {
		err = h.q.RebalanceSubtaskPositions(c, db.RebalanceSubtaskPositionsParams{
			ParentID: *t.ParentID, Gap: positionGap,
		})
	} else {
		err = h.q.RebalanceColumnTaskPositions(c, db.RebalanceColumnTaskPositionsParams{
			ColumnID: columnID, Gap: positionGap,
		})
	}
	if err != nil {
		fail(c, err)
		return 0, false
	}
	prev, next, ok := h.neighborTaskPositions(c, t, columnID, beforeID, afterID)
	if !ok {
		return 0, false
	}
	return positionBetween(prev, next), true
}

// positionForUnanchoredMove answers "which slot" for a move that named no
// neighbours at all — the shape every non-drag caller sends, because outside a
// drag there is nothing to aim between: the web context menu's «Переместить →
// колонка» (the only mover the list / calendar / timeline / gantt layouts have)
// and the MCP client's MoveTask, whose own comment says the position is "left to
// the server (appended)".
//
// It was not appended. positionBetween(nil, nil) is the constant positionGap, and
// positionGap is precisely where a column's FIRST card sits (CreateTask appends
// onto a 65536, 131072, 196608 … ladder). So a card moved that way did not land at
// the end of the destination column — it landed on top of the first card, sharing
// its position exactly. That tie is the state the rest of this file goes out of its
// way to prevent (rejectSameNeighbour, positionExhausted, the rebalance above): two
// cards with one position have no order of their own, and both clients' drop
// arithmetic against that slot keeps computing the same number. None of those
// guards fire here, because with both neighbours nil there is no slot to find a
// collapse in.
//
// The two ladders answer differently, for the same reason sameList splits them:
//
//   - a top-level card is numbered among the destination COLUMN's top-level cards,
//     and its old number came from another column's ladder — meaningless there. It
//     appends, the same way TransferTask and CreateTask do, which is also what the
//     web task modal and Android already emulate by hand (columnTail / the
//     rightmost sibling) when they send an explicit before_id.
//   - a subtask is numbered among its PARENT's children, and changing column does
//     not move it out of that list. Keeping its position leaves the parent's order
//     exactly as it was — which is what the same two clients ask for explicitly by
//     sending the sibling neighbours of the row where it already stands.
func (h *API) positionForUnanchoredMove(c *gin.Context, t db.Task, columnID uuid.UUID) (float64, bool) {
	pos, err := h.unanchoredPosition(c, t, columnID)
	if err != nil {
		fail(c, err)
		return 0, false
	}
	return pos, true
}

// unanchoredPosition is the rule above without the HTTP reply, for the callers
// that report their own errors — quick actions, whose `/move`, `/close` and
// `/reopen` are a column change with no neighbours either.
//
// They used to append through nextTaskPosition, which is CreateTask's rule: a
// NEW subtask belongs at the end of its parent's list. A subtask that merely
// changed column does not — it was already standing somewhere among its
// siblings, and `/close` on the first of three children dropped it to the bottom
// of the parent's list. Nothing in the gesture said "reorder my children", and
// the same move from the task modal (which sends the sibling neighbours of the
// row the subtask already stands on) leaves the order alone (#2932).
//
// The recurrence engine relocates a task the same way — to the rule's target
// column, with nothing to aim between — which is why this takes a bare context
// and not the request's.
func (h *API) unanchoredPosition(ctx context.Context, t db.Task, columnID uuid.UUID) (float64, error) {
	if t.ParentID != nil {
		return t.Position, nil
	}
	maxPos, err := h.q.MaxTaskPositionInColumn(ctx, columnID)
	if err != nil {
		return 0, err
	}
	return positionBetween(&maxPos, nil), nil
}

// recurrenceToStore validates a client-supplied recurrence rule and returns its
// canonical JSON form (or nil for an absent/invalid rule). For monthly/yearly it
// manages the day-of-month anchor: the anchor is re-derived from the due date
// when the user changes the due date or the frequency, and otherwise carried over
// from the previously stored rule — so unrelated edits don't reset an anchor that
// a short-month clamp has temporarily moved (e.g. the 30th showing as Feb 28).
//
// The rule's zone is carried over the same way when the request omits it (#2932).
// It is the field that decides which calendar day the whole series lands on (see
// recur.Rule.TZ), and a client that does not know it exists re-sends the rest of
// the rule as it always did: the Android editor has no tz field at all and its
// task update re-sends the stored rule by default, so a rename or a tick of the
// completed box there would reset a web-authored "every Monday" to UTC — which
// for anyone east of it turns the series into Tuesdays. Taking an explicit zone
// as an overwrite and silence as "unchanged" leaves the deliberate edit working
// and the incidental one harmless.
func recurrenceToStore(reqRaw *json.RawMessage, reqDue *time.Time, prevRaw *json.RawMessage, prevDue *time.Time) *json.RawMessage {
	rule, ok := recur.Parse(reqRaw)
	if !ok {
		return nil
	}
	prev, hadPrev := recur.Parse(prevRaw)
	if rule.TZ == "" && hadPrev {
		rule.TZ = prev.TZ
	}
	if rule.Freq == recur.FreqMonthly || rule.Freq == recur.FreqYearly {
		if hadPrev && prev.Freq == rule.Freq && sameTime(reqDue, prevDue) {
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
