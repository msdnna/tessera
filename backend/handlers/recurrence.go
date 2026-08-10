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

	if rule.CreateNew {
		// Strip the rule from the original (it's now a historical instance) and
		// spawn a duplicate that carries the recurrence forward.
		original, err := h.setTaskRecurrence(ctx, t, nil)
		if err != nil {
			original = t
		}
		if _, cerr := h.cloneRecurringTask(ctx, t, target, next, carry, wsID, actorID); cerr == nil {
			h.logEventActor(ctx, t.ID, actorID, "recurred", map[string]any{"due": next, "cloned": true})
		}
		return original, true
	}

	// Reschedule the same task.
	updated, err := h.q.UpdateTask(ctx, db.UpdateTaskParams{
		ID: t.ID, Title: t.Title, Description: t.Description, Priority: t.Priority,
		DueDate: &next, CompletedAt: nil, Recurrence: carry, StartDate: t.StartDate, Estimate: t.Estimate,
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
	h.logEventActor(ctx, t.ID, actorID, "recurred", map[string]any{"due": next})
	return updated, true
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

// cloneRecurringTask duplicates a task into the target column with a fresh due
// date and the carried recurrence rule: title/description/priority/tags/assignees
// are copied and direct subtasks are recreated uncompleted (a clean checklist).
func (h *API) cloneRecurringTask(ctx context.Context, src db.Task, columnID uuid.UUID, due time.Time, rule *json.RawMessage, wsID, actorID uuid.UUID) (db.Task, error) {
	endPos, err := h.q.MaxTaskPositionInColumn(ctx, columnID)
	if err != nil {
		return db.Task{}, err
	}
	num, err := h.q.NextWorkspaceTaskNumber(ctx, wsID)
	if err != nil {
		return db.Task{}, err
	}
	clone, err := h.q.CreateTask(ctx, db.CreateTaskParams{
		BoardID: src.BoardID, ColumnID: columnID, ParentID: nil,
		Title: src.Title, Description: src.Description, Priority: src.Priority,
		DueDate: &due, StartDate: src.StartDate, Estimate: src.Estimate, Position: positionBetween(&endPos, nil), CreatedBy: src.CreatedBy, Number: &num,
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
			soft(ctx, "AddTaskTag", h.q.AddTaskTag(ctx, db.AddTaskTagParams{TaskID: clone.ID, TagID: tg.ID}))
		}
	}
	if as, aerr := h.q.ListTaskAssignees(ctx, src.ID); aerr == nil {
		for _, a := range as {
			soft(ctx, "AddTaskAssignee", h.q.AddTaskAssignee(ctx, db.AddTaskAssigneeParams{TaskID: clone.ID, UserID: a.ID}))
		}
	}
	if subs, serr := h.q.ListSubtasks(ctx, &src.ID); serr == nil {
		pos := positionGap
		for _, s := range subs {
			snum, nerr := h.q.NextWorkspaceTaskNumber(ctx, wsID)
			if nerr != nil {
				continue
			}
			_, _ = h.q.CreateTask(ctx, db.CreateTaskParams{
				BoardID: src.BoardID, ColumnID: columnID, ParentID: &clone.ID,
				Title: s.Title, Description: s.Description, Priority: s.Priority,
				DueDate: s.DueDate, StartDate: s.StartDate, Position: pos, CreatedBy: src.CreatedBy, Number: &snum,
			})
			pos += positionGap
		}
	}
	h.logEventActor(ctx, clone.ID, actorID, "created", nil)
	h.broadcast(wsID, "task.created", clone)
	return clone, nil
}

// RunRecurrenceWorker drives schedule-triggered recurrences: tasks whose rule
// fires "on schedule" advance automatically once their due date passes, without
// anyone closing them. Idle until such a rule exists.
func (h *API) RunRecurrenceWorker(ctx context.Context) {
	const tick = time.Minute
	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	h.tick(jobRecurrence, "продвижение повторяющихся задач")
	h.withAdvisoryLock(ctx, "recurrence", func() { h.advanceScheduleDue(ctx) }) // catch up at startup
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.tick(jobRecurrence, "продвижение повторяющихся задач")
			h.withAdvisoryLock(ctx, "recurrence", func() { h.advanceScheduleDue(ctx) })
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
