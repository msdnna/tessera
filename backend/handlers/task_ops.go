package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/db"
	"tessera/internal/gitlab"
	"tessera/internal/recur"
	"tessera/middleware"
)

// This file holds the *operations* behind task mutations — the part that touches
// storage and then fans out journal entries, notifications, WebSocket events and
// GitLab writeback.
//
// They exist because a task can now be mutated from two directions: the REST
// handlers, and quick actions typed into a comment ("/close", "/assign @user").
// Both call the same op, so a quick action can never quietly skip a notification
// or a writeback trigger — the failure mode that would only surface months later.
//
// Ops take resolved arguments and return errors instead of writing responses;
// the HTTP layer keeps its own request parsing and status codes.

// opError is a user-facing failure: a bad argument, a missing target, something
// the caller can fix. The HTTP layer renders it as a 400 and quick actions put
// its text in the command summary. Any other error is an internal failure.
type opError struct{ msg string }

func (e opError) Error() string { return e.msg }

// userErr builds an opError.
func userErr(format string, a ...any) error { return opError{msg: fmt.Sprintf(format, a...)} }

// isUserErr reports whether err is a user-facing op failure.
func isUserErr(err error) bool {
	_, ok := err.(opError)
	return ok
}

// respondOpError writes the right status for an op failure: 400 for something
// the caller can fix, 500 otherwise.
func respondOpError(c *gin.Context, err error) {
	if isUserErr(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	fail(c, err)
}

// ── task field patch ───────────────────────────────────────────

// optTime is a tri-state date field: absent (leave alone), or set to a value or
// to null. A plain *time.Time cannot express "clear it" and "don't touch it" at
// the same time, which is exactly what /due and /remove_due need.
type optTime struct {
	set bool
	v   *time.Time
}

func (o optTime) resolve(cur *time.Time) *time.Time {
	if !o.set {
		return cur
	}
	return o.v
}

// setTime sets the field to a value.
func setTime(v *time.Time) optTime { return optTime{set: true, v: v} }

// clearTime sets the field to null.
func clearTime() optTime { return optTime{set: true} }

// optFloat is optTime for the estimate.
type optFloat struct {
	set bool
	v   *float64
}

func (o optFloat) resolve(cur *float64) *float64 {
	if !o.set {
		return cur
	}
	return o.v
}

func setFloat(v *float64) optFloat { return optFloat{set: true, v: v} }
func clearFloat() optFloat         { return optFloat{set: true} }

// taskPatch is a partial edit of a task's plain fields. Absent fields keep their
// current value, which is what a quick action needs — "/priority высокий" must
// not blank the description on its way through.
type taskPatch struct {
	Title       *string
	Description *string
	Priority    *int32
	DueDate     optTime
	StartDate   optTime
	Estimate    optFloat
	Completed   *bool
	// Recurrence is applied only when set; unset carries the stored rule over
	// (re-anchored if the due date moved).
	Recurrence    *json.RawMessage
	RecurrenceSet bool
}

// applyTaskPatch writes a partial task edit and fans out everything a task edit
// implies: completion bookkeeping, recurrence advance, GitLab override marks,
// the journal, participant notifications, writeback triggers and the WS event.
//
// It is the body of UpdateTask, minus request parsing — see the file header.
func (h *API) applyTaskPatch(c *gin.Context, t db.Task, wsID uuid.UUID, p taskPatch) (db.Task, error) {
	title := t.Title
	if p.Title != nil {
		if strings.TrimSpace(*p.Title) == "" {
			return t, userErr("заголовок не может быть пустым")
		}
		title = *p.Title
	}
	description := t.Description
	if p.Description != nil {
		description = *p.Description
	}
	priority := t.Priority
	if p.Priority != nil {
		priority = *p.Priority
	}
	dueDate := p.DueDate.resolve(t.DueDate)
	startDate := p.StartDate.resolve(t.StartDate)
	estimate := p.Estimate.resolve(t.Estimate)

	// Preserve the original completion timestamp; set/clear on toggle.
	completedAt := t.CompletedAt
	if p.Completed != nil {
		switch {
		case *p.Completed && completedAt == nil:
			now := time.Now()
			completedAt = &now
		case !*p.Completed:
			completedAt = nil
		}
	}

	// Normalise the recurrence rule and manage its anchor (or NULL if invalid).
	reqRecurrence := t.Recurrence
	if p.RecurrenceSet {
		reqRecurrence = p.Recurrence
	}
	recurrence := recurrenceToStore(reqRecurrence, dueDate, t.Recurrence, t.DueDate)

	updated, err := h.q.UpdateTask(c, db.UpdateTaskParams{
		ID: t.ID, Title: title, Description: description,
		Priority: priority, DueDate: dueDate, CompletedAt: completedAt,
		Recurrence: recurrence, StartDate: startDate, Estimate: normalizeEstimate(estimate),
	})
	if err != nil {
		return t, err
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
		soft(c, "MarkGitlabDueOverridden", h.q.MarkGitlabDueOverridden(c, t.ID))
	}
	// Likewise a manual start-date change wins over the sync.
	if !sameTime(t.StartDate, updated.StartDate) {
		soft(c, "MarkGitlabStartOverridden", h.q.MarkGitlabStartOverridden(c, t.ID))
	}
	// A manual estimate change wins over the GitLab timeEstimate pull.
	if !sameEstimate(t.Estimate, updated.Estimate) {
		soft(c, "MarkGitlabEstimateOverridden", h.q.MarkGitlabEstimateOverridden(c, t.ID))
	}
	changes := h.journalUpdate(c, t, updated)
	if len(changes) > 0 {
		h.notifyTaskParticipants(c, updated, wsID, "updated",
			fmt.Sprintf("%s изменил(а) задачу #%s: %s",
				h.actorName(c), taskRef(updated.Number), strings.Join(changes, ", ")))
	}
	// Mirror user-side changes back to a linked GitLab issue (opt-in per integration).
	actor := middleware.CurrentUser(c)
	// The explicit "Completed" flag is the legitimate source of a state push
	// (drives set_state → close/reopen). Column moves use a separate trigger.
	if (t.CompletedAt == nil) != (updated.CompletedAt == nil) {
		h.enqueueWriteback(c, t.ID, actor, gitlab.TrigCompletion, map[string]any{"completed": updated.CompletedAt != nil})
	}
	if t.Priority != updated.Priority {
		h.enqueueWriteback(c, t.ID, actor, gitlab.TrigPriority, map[string]any{"priority": updated.Priority})
	}
	// Due-date push reads the latest task state at push time, so the payload only
	// carries the date kind (also lets a burst of edits coalesce to one pending row).
	if !sameTime(t.DueDate, updated.DueDate) {
		h.enqueueWriteback(c, t.ID, actor, gitlab.TrigDue, map[string]any{"date_kind": "due"})
	}
	if !sameEstimate(t.Estimate, updated.Estimate) {
		h.enqueueWriteback(c, t.ID, actor, gitlab.TrigEstimate, map[string]any{})
	}
	// Title/description push reads the latest task state at push time (empty payload),
	// and is conflict-checked — GitLab issue bodies get edited richly, so a naive
	// overwrite is gated behind three-way detection.
	if t.Title != updated.Title || t.Description != updated.Description {
		h.enqueueWriteback(c, t.ID, actor, gitlab.TrigTitleDesc, map[string]any{})
	}
	h.broadcastAs(c, wsID, "task.updated", updated)
	return updated, nil
}

// ── column move ────────────────────────────────────────────────

// applyMove moves a task to a column at an explicit position, with the board's
// done-column auto-complete, column-triggered recurrence, journal, notification
// and GitLab triggers. The caller computes the position: the REST handler from
// the drag's neighbours, a quick action by appending to the target column.
func (h *API) applyMove(c *gin.Context, t db.Task, wsID, columnID uuid.UUID, position float64) (db.Task, error) {
	// Target column must be on the same board.
	col, err := h.q.GetColumn(c, columnID)
	if err != nil || col.BoardID != t.BoardID {
		return t, userErr("column does not belong to this board")
	}
	updated, err := h.q.MoveTask(c, db.MoveTaskParams{ID: t.ID, ColumnID: columnID, Position: position})
	if err != nil {
		return t, err
	}

	if t.ColumnID != columnID {
		h.logEvent(c, t.ID, "moved", map[string]any{"to": col.Name})
		h.notifyTaskParticipants(c, updated, wsID, "moved",
			fmt.Sprintf("%s переместил(а) задачу #%s → «%s»",
				h.actorName(c), taskRef(updated.Number), col.Name))
	}

	// Auto-toggle completion based on the board's configured "done" column:
	// moving in completes the task, moving out reopens it.
	if board, berr := h.q.GetBoard(c, t.BoardID); berr == nil {
		doneID := doneColumnID(board)
		targetIsDone := doneID != nil && *doneID == columnID
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
				h.logEvent(c, t.ID, "completed", nil)
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
				h.logEvent(c, t.ID, "reopened", nil)
			}
		}
	}

	// Column-triggered recurrence: moving the task into its configured column
	// advances it (independent of completion).
	if t.ColumnID != columnID {
		if rule, ok := recur.Parse(updated.Recurrence); ok &&
			rule.Trigger == recur.TriggerColumn && rule.TriggerColumn == columnID.String() {
			if advanced, acted := h.recurAdvance(c, updated, wsID, middleware.CurrentUser(c), recur.TriggerColumn); acted {
				updated = advanced
			}
		}
	}

	actor := middleware.CurrentUser(c)
	// Mirror the move to GitLab as a "column" trigger (any column change) — this drives
	// column→label bindings. It never pushes issue state, so a move alone can't close
	// an issue (the decoupling requirement).
	if t.ColumnID != columnID {
		h.enqueueWriteback(c, t.ID, actor, gitlab.TrigColumn,
			map[string]any{"column_id": columnID.String(), "column_name": col.Name})
	}
	// A completion change here comes only from the board's done-column auto-complete;
	// mirror it as a "completion" trigger (drives set_state) so a done column still
	// closes the issue by default. Teams that don't want that make "Готово" a
	// non-done column (no auto-complete → no completion trigger).
	if (t.CompletedAt == nil) != (updated.CompletedAt == nil) {
		h.enqueueWriteback(c, t.ID, actor, gitlab.TrigCompletion,
			map[string]any{"completed": updated.CompletedAt != nil})
	}
	h.broadcastAs(c, wsID, "task.moved", updated)
	return updated, nil
}

// ── assignees ──────────────────────────────────────────────────

// applyAssignee adds or removes a task assignee, with the journal entry, the
// "assigned to you" notification, the WS event and the GitLab assignee trigger.
func (h *API) applyAssignee(c *gin.Context, t db.Task, wsID, userID uuid.UUID, add bool) error {
	if add {
		if err := h.q.AddTaskAssignee(c, db.AddTaskAssigneeParams{TaskID: t.ID, UserID: userID}); err != nil {
			return err
		}
		h.logEvent(c, t.ID, "assigned", map[string]any{"user_id": userID})
		h.notify(c, userID, wsID, &t.ID, "assigned",
			fmt.Sprintf("%s назначил вам задачу #%s%s", h.actorName(c), taskRef(t.Number), shortCtx(t.Title)))
		h.broadcast(wsID, "task.assigned", gin.H{"task_id": t.ID, "user_id": userID})
	} else {
		if err := h.q.RemoveTaskAssignee(c, db.RemoveTaskAssigneeParams{TaskID: t.ID, UserID: userID}); err != nil {
			return err
		}
		h.logEvent(c, t.ID, "unassigned", map[string]any{"user_id": userID})
		h.broadcast(wsID, "task.unassigned", gin.H{"task_id": t.ID, "user_id": userID})
	}
	h.enqueueWriteback(c, t.ID, middleware.CurrentUser(c), gitlab.TrigAssignees, map[string]any{})
	return nil
}

// ── tags ───────────────────────────────────────────────────────

// applyTag attaches or detaches a project tag, with the WS event and the GitLab
// label reconciliation trigger.
func (h *API) applyTag(c *gin.Context, taskID, wsID, tagID uuid.UUID, add bool) error {
	if add {
		if err := h.q.AddTaskTag(c, db.AddTaskTagParams{TaskID: taskID, TagID: tagID}); err != nil {
			return err
		}
		h.broadcast(wsID, "task.tagged", gin.H{"task_id": taskID, "tag_id": tagID})
	} else {
		if err := h.q.RemoveTaskTag(c, db.RemoveTaskTagParams{TaskID: taskID, TagID: tagID}); err != nil {
			return err
		}
		h.broadcast(wsID, "task.untagged", gin.H{"task_id": taskID, "tag_id": tagID})
	}
	// Reconcile labels back to the linked issue (the worker reads the task's
	// current tags, so the empty payload coalesces a burst into one row).
	h.enqueueWriteback(c, taskID, middleware.CurrentUser(c), gitlab.TrigLabels, map[string]any{})
	return nil
}

// ── milestone ──────────────────────────────────────────────────

// applyMilestone pins or clears a task's milestone (nil clears), marking the
// change as a local override of the GitLab sync.
func (h *API) applyMilestone(c *gin.Context, taskID, wsID uuid.UUID, milestoneID *uuid.UUID) error {
	if err := h.q.SetTaskMilestone(c, db.SetTaskMilestoneParams{ID: taskID, MilestoneID: milestoneID}); err != nil {
		return err
	}
	// A manual milestone change on a GitLab-linked task wins over the sync.
	soft(c, "MarkGitlabMilestoneOverridden", h.q.MarkGitlabMilestoneOverridden(c, taskID))
	h.enqueueWriteback(c, taskID, middleware.CurrentUser(c), gitlab.TrigMilestone, map[string]any{})
	if t, err := h.q.GetTask(c, taskID); err == nil {
		h.broadcast(wsID, "task.updated", t)
	}
	return nil
}

// ── relations ──────────────────────────────────────────────────

// applyRelation links two tasks, journalling both sides so the link shows in the
// referenced task's history too (relations are stored one-way).
func (h *API) applyRelation(c *gin.Context, t db.Task, wsID uuid.UUID, target db.Task, kind string) error {
	if target.ID == t.ID {
		return userErr("нельзя связать задачу с собой")
	}
	if _, err := h.q.AddTaskRelation(c, db.AddTaskRelationParams{
		TaskID: t.ID, RelatedTaskID: target.ID, Kind: kind,
	}); err != nil && !errIsNoRows(err) {
		return err
	}
	h.logEvent(c, t.ID, "relation", map[string]any{"related": target.Number, "kind": kind})
	// Record the link on the referenced task too, so it shows in #target's
	// history ("добавил связь с #<source>") — relations are otherwise one-way.
	if t.Number != nil {
		h.logEvent(c, target.ID, "relation", map[string]any{"related": *t.Number, "kind": inverseRelationKind(kind)})
		h.broadcast(wsID, "task.updated", gin.H{"id": target.ID})
	}
	h.broadcast(wsID, "task.updated", gin.H{"id": t.ID})
	return nil
}

// applyUnlink drops every relation between two tasks, in either direction, and
// reports how many rows went away (0 means there was nothing to unlink).
//
// Relations are stored one-way, so "/unlink #N" has to look from both ends: the
// row may live on either task depending on who linked whom.
func (h *API) applyUnlink(c *gin.Context, t db.Task, wsID uuid.UUID, target db.Task) (int, error) {
	var n int
	for _, side := range [2][2]uuid.UUID{{t.ID, target.ID}, {target.ID, t.ID}} {
		rels, err := h.q.ListTaskRelations(c, side[0])
		if err != nil {
			return n, err
		}
		for _, r := range rels {
			if r.RelatedTaskID != side[1] {
				continue
			}
			if err := h.q.DeleteTaskRelation(c, r.ID); err != nil {
				return n, err
			}
			n++
		}
	}
	if n > 0 {
		h.logEvent(c, t.ID, "relation_removed", map[string]any{"related": target.Number})
		h.broadcast(wsID, "task.updated", gin.H{"id": t.ID})
		h.broadcast(wsID, "task.updated", gin.H{"id": target.ID})
	}
	return n, nil
}

// ── hierarchy / archive ────────────────────────────────────────

// applyParent attaches a task to a parent (inheriting its board and column) or
// detaches it (nil → back on the board as a top-level card).
func (h *API) applyParent(c *gin.Context, t db.Task, wsID uuid.UUID, parentID *uuid.UUID) (db.Task, error) {
	boardID, columnID := t.BoardID, t.ColumnID
	if parentID != nil {
		if *parentID == t.ID {
			return t, userErr("a task cannot be its own parent")
		}
		parent, err := h.q.GetTask(c, *parentID)
		if err != nil {
			return t, userErr("invalid parent")
		}
		// The parent must not itself be a child of this task (1-level cycle).
		if parent.ParentID != nil && *parent.ParentID == t.ID {
			return t, userErr("cyclic parent")
		}
		boardID, columnID = parent.BoardID, parent.ColumnID
	}
	updated, err := h.q.SetTaskParent(c, db.SetTaskParentParams{
		ID: t.ID, ParentID: parentID, BoardID: boardID, ColumnID: columnID,
	})
	if err != nil {
		return t, err
	}
	h.broadcastAs(c, wsID, "task.moved", updated)
	return updated, nil
}

// applyArchive soft-deletes a task. detach keeps its subtasks on the board
// (re-parented to null); otherwise they are archived with the parent.
func (h *API) applyArchive(c *gin.Context, t db.Task, wsID uuid.UUID, detach bool) error {
	// Transactional: detach-then-archive must be atomic, else a crash leaves the
	// parent archived with its subtasks still active on the board (or vice versa).
	if err := h.inTx(c, func(q *db.Queries) error {
		if detach {
			if err := q.DetachChildren(c, &t.ID); err != nil {
				return err
			}
			return q.ArchiveTask(c, t.ID)
		}
		return q.ArchiveTaskCascade(c, t.ID)
	}); err != nil {
		return err
	}
	h.logEvent(c, t.ID, "archived", nil)
	h.notifyTaskParticipants(c, t, wsID, "archived",
		fmt.Sprintf("%s архивировал(а) задачу #%s%s", h.actorName(c), taskRef(t.Number), shortCtx(t.Title)))
	h.broadcast(wsID, "task.archived", gin.H{"id": t.ID})
	return nil
}

// applyCreateSubtask appends a subtask under a parent, in the parent's column.
func (h *API) applyCreateSubtask(c *gin.Context, parent db.Task, wsID uuid.UUID, title string) (db.Task, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return db.Task{}, userErr("не указан заголовок подзадачи")
	}
	pos, err := h.nextTaskPosition(c, parent.ColumnID, &parent.ID)
	if err != nil {
		return db.Task{}, err
	}
	num, err := h.q.NextWorkspaceTaskNumber(c, wsID)
	if err != nil {
		return db.Task{}, err
	}
	uid := middleware.CurrentUser(c)
	sub, err := h.q.CreateTask(c, db.CreateTaskParams{
		BoardID: parent.BoardID, ColumnID: parent.ColumnID, ParentID: &parent.ID,
		Title: title, Position: pos, CreatedBy: &uid, Number: &num,
	})
	if err != nil {
		return db.Task{}, err
	}
	h.logEvent(c, sub.ID, "created", nil)
	h.broadcastAs(c, wsID, "task.created", sub)
	return sub, nil
}
