package handlers

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/google/uuid"

	"tessera/internal/db"
	"tessera/internal/recur"
)

// recurAdvance advances a recurring task when an event matching its trigger fires
// (completion or a column move). It computes the next occurrence from the current
// due date and hands off to applyRecur. Returns (task, true) when it acted; for a
// non-matching trigger, a one-off task, or no due date it is a no-op (t, false).
func (h *API) recurAdvance(ctx context.Context, t db.Task, wsID, actorID uuid.UUID, trigger string) (db.Task, bool) {
	rule, ok := recur.Parse(t.Recurrence)
	if !ok || rule.Trigger != trigger || t.DueDate == nil {
		return t, false
	}
	next, hasNext := rule.Next(*t.DueDate)
	return h.applyRecur(ctx, t, wsID, actorID, rule, next, hasNext)
}

// applyRecur carries out a recurrence: it either reschedules the task itself
// (advance due, clear completion, move to the target column, reopen subtasks) or —
// when the rule says create_new — leaves the original as a completed record and
// spawns a fresh duplicate. A one-off rule (once) is dropped from whichever task
// carries it forward. When the recurrence has ended (custom dates exhausted) the
// rule is cleared and the task is left as-is.
func (h *API) applyRecur(ctx context.Context, t db.Task, wsID, actorID uuid.UUID, rule recur.Rule, next time.Time, hasNext bool) (db.Task, bool) {
	if !hasNext {
		if cleared, err := h.setTaskRecurrence(ctx, t, nil); err == nil {
			return cleared, true
		}
		return t, false
	}

	target := h.recurTargetColumn(ctx, t, rule)
	// The rule the continuing instance keeps — dropped entirely for a one-shot.
	carry := t.Recurrence
	if rule.Once {
		carry = nil
	}
	nextStart := shiftStart(t.StartDate, t.DueDate, next)

	if rule.CreateNew {
		// Strip the rule from the original (it's now a historical instance) and
		// spawn a duplicate that carries the recurrence forward.
		original, err := h.setTaskRecurrence(ctx, t, nil)
		if err != nil {
			original = t
		}
		if _, cerr := h.cloneRecurringTask(ctx, t, target, next, nextStart, carry, wsID, actorID); cerr == nil {
			h.logEventActor(ctx, t.ID, actorID, "recurred", map[string]any{"due": next, "cloned": true})
		}
		return original, true
	}

	// Reschedule the same task.
	updated, err := h.q.UpdateTask(ctx, db.UpdateTaskParams{
		ID: t.ID, Title: t.Title, Description: t.Description, Priority: t.Priority,
		DueDate: &next, CompletedAt: nil, Recurrence: carry, StartDate: nextStart, Estimate: t.Estimate,
	})
	if err != nil {
		return t, false
	}
	if target != updated.ColumnID {
		if endPos, merr := h.q.MaxTaskPositionInColumn(ctx, target); merr == nil {
			if moved, mverr := h.q.MoveTask(ctx, db.MoveTaskParams{
				ID: updated.ID, ColumnID: target, Position: positionBetween(&endPos, nil),
			}); mverr == nil {
				updated = moved
			}
		}
	}
	soft(ctx, "ReopenSubtasks", h.q.ReopenSubtasks(ctx, &t.ID))
	h.liftSubtasksFromDone(ctx, t, target)
	// The checklist travels with the parent: a subtask is a task in its own right
	// (its own card, its own bar, its own overdue badge), so leaving its dates on
	// the previous occurrence makes every item of the second run be born late.
	if t.DueDate != nil {
		soft(ctx, "ShiftSubtaskWindow", h.q.ShiftSubtaskWindow(ctx, db.ShiftSubtaskWindowParams{
			ShiftSeconds: next.Sub(*t.DueDate).Seconds(), ParentID: t.ID,
		}))
	}
	h.logEventActor(ctx, t.ID, actorID, "recurred", map[string]any{"due": next})
	return updated, true
}

// liftSubtasksFromDone takes the reopened checklist out of the board's done column
// and lands it where the parent just landed.
//
// A subtask is a task in its own right — its own card, its own column — and it is
// closed either by its checkbox or by being dragged into the done column, where
// MoveTask's auto-toggle sets completed_at. Those two facts are kept in step
// everywhere else, so ReopenSubtasks clearing only completed_at left an item the
// board still draws under «Готово» with an empty checkbox, and dragging it out
// again then found nothing to reopen. The parent already moves (recurTargetColumn
// is resolved right above), and the create_new branch builds the duplicate's
// checklist in the target column from the start; this is the reschedule branch
// saying the same thing.
//
// Only items sitting in the done column move: one parked in «В процессе» is where
// its owner left it, and a recurrence is not a reason to tidy that away.
func (h *API) liftSubtasksFromDone(ctx context.Context, t db.Task, target uuid.UUID) {
	board, err := h.q.GetBoard(ctx, t.BoardID)
	if err != nil {
		return
	}
	done := doneColumnID(board)
	// No done column configured, or the rule deliberately recurs *into* it — then
	// there is nothing to lift and nowhere else to put it.
	if done == nil || *done == target {
		return
	}
	soft(ctx, "LiftSubtasksFromColumn", h.q.LiftSubtasksFromColumn(ctx, db.LiftSubtasksFromColumnParams{
		TargetColumn: target, ParentID: t.ID, FromColumn: *done,
	}))
}

// recurTargetColumn resolves where a recurred task lands: the rule's target column
// (when still on the board), else the board's first column, else the current one.
func (h *API) recurTargetColumn(ctx context.Context, t db.Task, rule recur.Rule) uuid.UUID {
	if rule.TargetColumn != "" {
		if id, err := uuid.Parse(rule.TargetColumn); err == nil {
			if col, err := h.q.GetColumn(ctx, id); err == nil && col.BoardID == t.BoardID {
				return id
			}
		}
	}
	if cols, err := h.q.ListColumns(ctx, t.BoardID); err == nil && len(cols) > 0 {
		return cols[0].ID
	}
	return t.ColumnID
}

// setTaskRecurrence rewrites only a task's recurrence rule, preserving its other
// fields (used to clear a one-off rule or strip it from a duplicated original).
func (h *API) setTaskRecurrence(ctx context.Context, t db.Task, rule *json.RawMessage) (db.Task, error) {
	return h.q.UpdateTask(ctx, db.UpdateTaskParams{
		ID: t.ID, Title: t.Title, Description: t.Description, Priority: t.Priority,
		DueDate: t.DueDate, CompletedAt: t.CompletedAt, Recurrence: rule, StartDate: t.StartDate, Estimate: t.Estimate,
	})
}

// shiftStart moves a start date by the same step the due date just took, so a
// recurrence carries the planned window forward whole instead of stretching it.
//
// The boards draw one bar per task from start_date to due_date (Gantt and
// timeline both), and the estimate's ghost bar anchors on the start. Advancing
// only the deadline pinned every occurrence's start to the first one, so a daily
// stand-up's bar grew a day longer each time it recurred — after a month a
// half-hour meeting reads as a month-long effort, and it drags the chart's whole
// axis with it.
//
// The step is measured as elapsed time, not calendar arithmetic, because the
// window's length is what has to survive: a monthly rule landing on a short
// February moves due by 28 days, and the start has to move 28 too or the window
// silently changes size. A task with no start (or no previous due to measure
// from) keeps none — a bar for a task nobody scheduled to begin would be the same
// invention in the other direction.
func shiftStart(start, prevDue *time.Time, next time.Time) *time.Time {
	if start == nil || prevDue == nil {
		return start
	}
	shifted := start.Add(next.Sub(*prevDue))
	return &shifted
}

// shiftBy moves an optional timestamp by step, keeping "no date" as no date.
func shiftBy(ts *time.Time, step time.Duration) *time.Time {
	if ts == nil || step == 0 {
		return ts
	}
	shifted := ts.Add(step)
	return &shifted
}

// cloneRecurringTask duplicates a task into the target column with a fresh due
// date and the carried recurrence rule: title/description/priority/tags/assignees
// are copied and direct subtasks are recreated uncompleted (a clean checklist).
// start is the duplicate's own start date (already stepped by shiftStart) — the
// clone is the next occurrence, not a snapshot of the first.
//
// The duplicate hangs off the same parent as the original, which for a recurring
// *checklist item* is the whole point: the rule is written in the same DueEditor
// whether the modal was opened on a card or on one of its subtasks, and a
// create_new item left the checklist for good the first time it was ticked. The
// original stays under the parent as the spent occurrence, so a root clone made
// the item read as deleted — the parent showed only the completed copy, while the
// next occurrence stood on the board as a card of its own, outside the subtask
// counts and out of reach of liftSubtasksFromDone. The reschedule branch never had
// this: it edits the task in place and UpdateTask does not carry parent_id.
func (h *API) cloneRecurringTask(ctx context.Context, src db.Task, columnID uuid.UUID, due time.Time, start *time.Time, rule *json.RawMessage, wsID, actorID uuid.UUID) (db.Task, error) {
	endPos, err := h.q.MaxTaskPositionInColumn(ctx, columnID)
	if err != nil {
		return db.Task{}, err
	}
	num, err := h.q.NextWorkspaceTaskNumber(ctx, wsID)
	if err != nil {
		return db.Task{}, err
	}
	clone, err := h.q.CreateTask(ctx, db.CreateTaskParams{
		BoardID: src.BoardID, ColumnID: columnID, ParentID: src.ParentID,
		Title: src.Title, Description: src.Description, Priority: src.Priority,
		DueDate: &due, StartDate: start, Estimate: src.Estimate, Position: positionBetween(&endPos, nil), CreatedBy: src.CreatedBy, Number: &num,
	})
	if err != nil {
		return db.Task{}, err
	}
	// CreateTask doesn't take a recurrence rule — set it on the clone afterwards.
	if rule != nil {
		if c2, uerr := h.setTaskRecurrence(ctx, clone, rule); uerr == nil {
			clone = c2
		}
	}
	if tags, terr := h.q.ListTaskTags(ctx, src.ID); terr == nil {
		for _, tg := range tags {
			_, addErr := h.q.AddTaskTag(ctx, db.AddTaskTagParams{TaskID: clone.ID, TagID: tg.ID})
			soft(ctx, "AddTaskTag", addErr)
		}
	}
	if as, aerr := h.q.ListTaskAssignees(ctx, src.ID); aerr == nil {
		for _, a := range as {
			soft(ctx, "AddTaskAssignee", h.q.AddTaskAssignee(ctx, db.AddTaskAssigneeParams{TaskID: clone.ID, UserID: a.ID}))
		}
	}
	if subs, serr := h.q.ListSubtasks(ctx, &src.ID); serr == nil {
		// The step the parent's due date took — each item's own window rides along
		// so the duplicate's checklist isn't born pinned to the previous occurrence.
		var step time.Duration
		if src.DueDate != nil {
			step = due.Sub(*src.DueDate)
		}
		pos := positionGap
		for _, s := range subs {
			snum, nerr := h.q.NextWorkspaceTaskNumber(ctx, wsID)
			if nerr != nil {
				continue
			}
			sub, cerr := h.q.CreateTask(ctx, db.CreateTaskParams{
				BoardID: src.BoardID, ColumnID: columnID, ParentID: &clone.ID,
				Title: s.Title, Description: s.Description, Priority: s.Priority,
				DueDate: shiftBy(s.DueDate, step), StartDate: shiftBy(s.StartDate, step),
				Estimate: s.Estimate, Position: pos, CreatedBy: src.CreatedBy, Number: &snum,
			})
			pos += positionGap
			if cerr != nil {
				continue
			}
			// The reschedule branch keeps each item's estimate, tags and assignees
			// (it never touches them); the duplicate must hand back the same
			// checklist, not a barer one.
			if tags, terr := h.q.ListTaskTags(ctx, s.ID); terr == nil {
				for _, tg := range tags {
					_, addErr := h.q.AddTaskTag(ctx, db.AddTaskTagParams{TaskID: sub.ID, TagID: tg.ID})
					soft(ctx, "AddTaskTag", addErr)
				}
			}
			if as, aerr := h.q.ListTaskAssignees(ctx, s.ID); aerr == nil {
				for _, a := range as {
					soft(ctx, "AddTaskAssignee", h.q.AddTaskAssignee(ctx, db.AddTaskAssigneeParams{TaskID: sub.ID, UserID: a.ID}))
				}
			}
		}
	}
	h.logEventActor(ctx, clone.ID, actorID, "created", nil)
	// The journal already knows who caused this (logEventActor above) — the live
	// event has to say it too. Closing a "create a duplicate" recurring task is a
	// person's action, and the clone's created_by is the author of the FIRST
	// occurrence, so an actor-less event left clients no honest way to attribute it
	// (#2932). The schedule worker passes uuid.Nil and stays anonymous.
	h.broadcastByActor(wsID, "task.created", actorID, clone)
	return clone, nil
}

// RunRecurrenceWorker drives schedule-triggered recurrences: tasks whose rule
// fires "on schedule" advance automatically once their due date passes, without
// anyone closing them. Idle until such a rule exists.
func (h *API) RunRecurrenceWorker(ctx context.Context) {
	const tick = time.Minute
	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	h.tick(jobRecurrence, opRecurrence)
	h.withAdvisoryLock(ctx, recurrenceLock, func() { h.advanceScheduleDue(ctx) }) // catch up at startup
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.tick(jobRecurrence, opRecurrence)
			h.withAdvisoryLock(ctx, recurrenceLock, func() { h.advanceScheduleDue(ctx) })
		}
	}
}

// advanceScheduleDue advances every schedule-triggered task whose due date has
// passed, skipping occurrences missed during downtime (NextAfter) so the worker
// doesn't fire once per missed period.
func (h *API) advanceScheduleDue(ctx context.Context) {
	tasks, err := h.q.ListScheduleRecurDue(ctx)
	if err != nil {
		return
	}
	now := time.Now()
	for _, t := range tasks {
		rule, ok := recur.Parse(t.Recurrence)
		if !ok || t.DueDate == nil {
			continue
		}
		wsID, werr := h.q.WorkspaceIDForBoard(ctx, t.BoardID)
		if werr != nil {
			continue
		}
		next, hasNext := rule.NextAfter(*t.DueDate, now)
		updated, acted := h.applyRecur(ctx, t, wsID, uuid.Nil, rule, next, hasNext)
		if acted {
			h.broadcast(wsID, "task.moved", updated)
			log.Printf("recurrence(schedule) advanced task=%s ws=%s", t.ID, wsID)
		}
	}
}
