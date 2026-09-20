package handlers

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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
	nextStart := shiftStart(rule, t.StartDate, t.DueDate, next)

	if rule.CreateNew {
		// Strip the rule from the original (it's now a historical instance) and
		// spawn a duplicate that carries the recurrence forward.
		original, err := h.setTaskRecurrence(ctx, t, nil)
		if err != nil {
			original = t
		}
		if _, cerr := h.cloneRecurringTask(ctx, t, target, next, nextStart, rule, carry, wsID, actorID); cerr == nil {
			h.logEventActor(ctx, t.ID, actorID, "recurred", map[string]any{"due": next, "cloned": true})
		}
		return original, true
	}

	// Reschedule the same task.
	updated, err := h.q.UpdateTask(ctx, db.UpdateTaskParams{
		ID: t.ID, Title: t.Title, Description: untickDescription(t.Description), Priority: t.Priority,
		DueDate: &next, CompletedAt: nil, Recurrence: carry, StartDate: nextStart, Estimate: t.Estimate,
	})
	if err != nil {
		return t, false
	}
	// The relocation names no neighbours — there is nothing to aim between — so
	// it takes the same slot rule every other unanchored move takes: a card
	// appends to the target column, a checklist item keeps the number it already
	// had. Off the column's tail instead, a recurring *item* was renumbered onto
	// the wrong ladder entirely (a card's, per column, not its parent's) and so
	// left the middle of the checklist for the bottom of it — every occurrence,
	// on a list nobody asked to reorder (#2932).
	if target != updated.ColumnID {
		if pos, perr := h.unanchoredPosition(ctx, updated, target); perr == nil {
			if moved, mverr := h.q.MoveTask(ctx, db.MoveTaskParams{
				ID: updated.ID, ColumnID: target, Position: pos,
			}); mverr == nil {
				updated = moved
			}
		}
	}
	h.carryChecklist(ctx, t, rule, target, next)
	h.logEventActor(ctx, t.ID, actorID, "recurred", map[string]any{"due": next})
	return updated, true
}

// carryChecklist hands the next occurrence the parent's whole checklist: unticked,
// out of the board's done column, and re-dated by the step the parent's due date
// just took.
//
// The checklist travels with the parent because a subtask is a task in its own
// right — its own card, its own column, its own bar, its own overdue badge. It is
// closed either by its checkbox or by being dragged into the done column, where
// MoveTask's auto-toggle sets completed_at, and those two facts are kept in step
// everywhere else: clearing completed_at alone left an item the board still draws
// under «Готово» with an empty checkbox, and dragging it out again then found
// nothing to reopen. Leaving its dates on the previous occurrence makes every item
// of the second run be born late.
//
// All of that is dispatched by carryChecklistFrom below, which is where the depth
// lives.
// The step handed down is the one the rule's calendar took (WallStep), not the
// elapsed one: on the day a zone changes its offset the two differ by an hour,
// and only the calendar one keeps each item on the clock its parent kept.
func (h *API) carryChecklist(ctx context.Context, t db.Task, rule recur.Rule, target uuid.UUID, next time.Time) {
	var shift *float64
	if t.DueDate != nil {
		s := rule.WallStep(*t.DueDate, next).Seconds()
		shift = &s
	}
	h.carryChecklistFrom(ctx, t.ID, target, h.doneColumnToLeave(ctx, t, target), rule.ZoneName(), shift,
		map[uuid.UUID]bool{t.ID: true})
}

// carryChecklistFrom applies the carry to one node's direct children and then to
// theirs — the whole subtree, not its first level.
//
// Nesting has no depth limit (CreateTask hangs a subtask off a subtask without
// complaint, and the board draws and counts every level), so a repeating task
// decomposed two levels deep is ordinary. All three statements name parent_id, so
// on their own they reach the DIRECT children only, and everything below the first
// level stayed exactly where the previous occurrence ended it: still ticked, still
// parked under «Готово», still dated for a deadline already past. The second run
// of the routine was born half-done and overdue, and it got worse every cycle.
// cloneChecklist already walks the whole subtree for the create_new branch; this
// is the reschedule branch saying the same thing about the same checklist.
//
// Only items sitting in the done column move: one parked in «В процессе» is where
// its owner left it, and a recurrence is not a reason to tidy that away.
//
// An archived item is skipped — by all three statements (they filter on
// archived_at) and by the walk below, which does not descend into one, the same
// way cloneChecklist skips an archived row and everything under it. Leaving it
// archived was not enough: the row is an entry of the board's archive in its own
// right, and un-ticking it, moving it out of «Готово» and stepping its dates every
// occurrence is what the archive then gave back on restore — an item that went in
// finished came out open, in the rule's target column, with a deadline a week in
// the future (#2932).
//
// `seen` guards the walk itself rather than leaning on the re-parent guard, for
// the reason cloneChecklist gives: a ring must not spin here.
func (h *API) carryChecklistFrom(
	ctx context.Context, parentID, target uuid.UUID, done *uuid.UUID,
	zone string, shift *float64, seen map[uuid.UUID]bool,
) {
	soft(ctx, "ReopenSubtasks", h.q.ReopenSubtasks(ctx, &parentID))
	if done != nil {
		soft(ctx, "LiftSubtasksFromColumn", h.q.LiftSubtasksFromColumn(ctx, db.LiftSubtasksFromColumnParams{
			TargetColumn: target, ParentID: parentID, FromColumn: *done,
		}))
	}
	if shift != nil {
		soft(ctx, "ShiftSubtaskWindow", h.q.ShiftSubtaskWindow(ctx, db.ShiftSubtaskWindowParams{
			Zone: zone, ShiftSeconds: *shift, ParentID: parentID,
		}))
	}
	subs, err := h.q.ListSubtasks(ctx, &parentID)
	if err != nil {
		return
	}
	for _, s := range subs {
		if s.ArchivedAt != nil || seen[s.ID] {
			continue
		}
		seen[s.ID] = true
		// The item's own description gets the same reset the card's does — an
		// item is a task in its own right and holds a description of its own.
		// One statement per item that actually has a ticked box: the three bulk
		// ones above cannot do it (a fence has to be read line by line), and a
		// routine's checklist is a handful of rows.
		if fresh := untickDescription(s.Description); fresh != s.Description {
			soft(ctx, "SetTaskDescription", h.q.SetTaskDescription(ctx, db.SetTaskDescriptionParams{
				ID: s.ID, Description: fresh,
			}))
		}
		h.carryChecklistFrom(ctx, s.ID, target, done, zone, shift, seen)
	}
}

// doneColumnToLeave resolves the done column a recurrence has to take the
// checklist out of: nil when the board has none configured, or when the rule
// deliberately recurs *into* it — then there is nothing to lift and nowhere else
// to put it.
func (h *API) doneColumnToLeave(ctx context.Context, t db.Task, target uuid.UUID) *uuid.UUID {
	board, err := h.q.GetBoard(ctx, t.BoardID)
	if err != nil {
		return nil
	}
	done := doneColumnID(board)
	if done == nil || *done == target {
		return nil
	}
	return done
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
// The step is the whole distance the due date travelled, not calendar
// arithmetic of its own, because the window's length is what has to survive: a
// monthly rule landing on a short February moves due by 28 days, and the start
// has to move 28 too or the window silently changes size. A task with no start
// (or no previous due to measure from) keeps none — a bar for a task nobody
// scheduled to begin would be the same invention in the other direction.
//
// That distance is measured on the rule's calendar (WallStep) and applied to the
// start's clock, because the two readings part company on the day a zone changes
// its offset: a daily task due 09:00 is due 09:00 again the next day, 23 elapsed
// hours later across a spring-forward, and a start at 08:00 that same morning has
// to read 08:00 too. Adding the 23 hours to the instant read 07:00 instead — the
// window grew an hour, and it stayed grown until the clocks went back (#2932).
func shiftStart(rule recur.Rule, start, prevDue *time.Time, next time.Time) *time.Time {
	if start == nil || prevDue == nil {
		return start
	}
	shifted := rule.ShiftWall(*start, rule.WallStep(*prevDue, next))
	return &shifted
}

// shiftBy moves an optional timestamp by step on the rule's calendar, keeping
// "no date" as no date. The step comes from WallStep, so it advances the clock
// reading — see shiftStart for why the instant is the wrong thing to move.
func shiftBy(rule recur.Rule, ts *time.Time, step time.Duration) *time.Time {
	if ts == nil || step == 0 {
		return ts
	}
	shifted := rule.ShiftWall(*ts, step)
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
func (h *API) cloneRecurringTask(ctx context.Context, src db.Task, columnID uuid.UUID, due time.Time, start *time.Time, rule recur.Rule, carry *json.RawMessage, wsID, actorID uuid.UUID) (db.Task, error) {
	// The duplicate is a new task in whichever list it is born into, so it takes
	// CreateTask's own rule — the end of the parent's checklist for an item, the
	// end of the column for a card. The column's tail alone was a card's number
	// handed to a child: on the ordinary layout (the parent being its column's
	// only card) "last card + one gap" is exactly the second item's position, and
	// two children with one number have no order of their own (#2932).
	clonePos, err := h.nextTaskPosition(ctx, columnID, src.ParentID)
	if err != nil {
		return db.Task{}, err
	}
	num, err := h.q.NextWorkspaceTaskNumber(ctx, wsID)
	if err != nil {
		return db.Task{}, err
	}
	clone, err := h.q.CreateTask(ctx, db.CreateTaskParams{
		BoardID: src.BoardID, ColumnID: columnID, ParentID: src.ParentID,
		Title: src.Title, Description: untickDescription(src.Description), Priority: src.Priority,
		DueDate: &due, StartDate: start, Estimate: src.Estimate, Position: clonePos, CreatedBy: src.CreatedBy, Number: &num,
	})
	if err != nil {
		return db.Task{}, err
	}
	// CreateTask doesn't take a recurrence rule — set it on the clone afterwards.
	if carry != nil {
		if c2, uerr := h.setTaskRecurrence(ctx, clone, carry); uerr == nil {
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
	h.carryDueNotify(ctx, src, clone.ID)
	h.carryTaskPins(ctx, src, clone.ID)
	h.moveDocumentLinks(ctx, src.ID, clone.ID)
	clone = h.carryAttachments(ctx, src, clone)
	// The step the parent's due date took — each item's own window rides along
	// so the duplicate's checklist isn't born pinned to the previous occurrence.
	// Measured on the rule's calendar, for the reason shiftStart gives.
	var step time.Duration
	if src.DueDate != nil {
		step = rule.WallStep(*src.DueDate, due)
	}
	// Who became whom, this task included: the relations are carried after the
	// whole checklist is rebuilt so a link BETWEEN two items of the same plan can
	// land on the new pair instead of the spent one.
	mapped := map[uuid.UUID]uuid.UUID{src.ID: clone.ID}
	h.cloneChecklist(ctx, src, clone, columnID, rule, step, wsID, map[uuid.UUID]bool{src.ID: true}, mapped)
	h.carryRelations(ctx, wsID, mapped)
	h.logEventActor(ctx, clone.ID, actorID, "created", nil)
	// The journal already knows who caused this (logEventActor above) — the live
	// event has to say it too. Closing a "create a duplicate" recurring task is a
	// person's action, and the clone's created_by is the author of the FIRST
	// occurrence, so an actor-less event left clients no honest way to attribute it
	// (#2932). The schedule worker passes uuid.Nil and stays anonymous.
	h.broadcastByActor(wsID, "task.created", actorID, clone)
	return clone, nil
}

// cloneChecklist copies src's checklist under dst — every level of it, not just
// the first.
//
// Nesting has no depth limit (CreateTask hangs a subtask off a subtask without
// complaint, and the board draws and counts every level), so a repeating task
// planned two levels deep is ordinary. This used to be one flat loop over
// ListSubtasks(src), which returns the DIRECT children only: the duplicate
// collapsed such a plan into its first level, and the steps below it were not
// archived, not moved and not reported — they simply were not in the new
// occurrence, nor in any after it. The reschedule branch edits the task in place
// and carries the whole subtree, and the duplicate has to match it.
//
// `seen` carries the source ids already walked. The re-parent guard keeps the
// tree acyclic, but this walk must not be the thing that depends on it: a ring
// would otherwise spin creating tasks until something gave (the same reason
// documentSubtreeIDs dedups its own walk).
func (h *API) cloneChecklist(
	ctx context.Context, src, dst db.Task, columnID uuid.UUID,
	rule recur.Rule, step time.Duration, wsID uuid.UUID, seen map[uuid.UUID]bool,
	mapped map[uuid.UUID]uuid.UUID,
) {
	subs, err := h.q.ListSubtasks(ctx, &src.ID)
	if err != nil {
		return
	}
	pos := positionGap
	for _, s := range subs {
		// An item in the archive is not part of the routine any more, and
		// archiving one while keeping the rest is a deliberate gesture with its
		// own undo (the row stands in the board's archive as an entry of its
		// own, its parent being active). ListSubtasks does not filter on
		// archived_at — it is also what nextTaskPosition reads, where an
		// archived sibling must still hold its place — so the skip belongs
		// here. Without it CreateTask handed the item back alive, tags and
		// assignees included, in this occurrence and in every one after: one
		// archiving, endless copies, while the original stayed in the archive.
		// The reschedule branch edits the task in place and never resurrected
		// anything; the duplicate has to match it. Skipping the row skips what
		// hangs below it too: an archived item takes its own steps with it.
		if s.ArchivedAt != nil || seen[s.ID] {
			continue
		}
		seen[s.ID] = true
		snum, nerr := h.q.NextWorkspaceTaskNumber(ctx, wsID)
		if nerr != nil {
			continue
		}
		sub, cerr := h.q.CreateTask(ctx, db.CreateTaskParams{
			BoardID: src.BoardID, ColumnID: columnID, ParentID: &dst.ID,
			Title: s.Title, Description: untickDescription(s.Description), Priority: s.Priority,
			DueDate: shiftBy(rule, s.DueDate, step), StartDate: shiftBy(rule, s.StartDate, step),
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
		h.carryDueNotify(ctx, s, sub.ID)
		h.carryTaskPins(ctx, s, sub.ID)
		h.moveDocumentLinks(ctx, s.ID, sub.ID)
		sub = h.carryAttachments(ctx, s, sub)
		sub = h.carryItemRecurrence(ctx, s, sub)
		mapped[s.ID] = sub.ID
		h.cloneChecklist(ctx, s, sub, columnID, rule, step, wsID, seen, mapped)
	}
}

// carryDueNotify copies a task's own due-notification settings — the lead, the
// repeat and the on/off switch — onto the occurrence cloned from it.
//
// The three live as columns on `tasks` (NULL = inherit the user's default) and the
// card's due popover is what sets them. The reschedule branch keeps them for free
// (it edits the task in place, and UpdateTask names none of the three); the
// duplicate is born from CreateTask with all three NULL, so without this a muted
// daily chore started notifying again the next day and a report set to warn three
// days ahead warned an hour ahead — while the popover of the spent occurrence
// still showed what had been asked for (#2932).
//
// Nothing is written when the source set nothing: pinning the user's current
// default into the clone's row would make the next change to that default stop
// reaching a task nobody ever customised.
func (h *API) carryDueNotify(ctx context.Context, src db.Task, dstID uuid.UUID) {
	if src.DueLeadMinutes == nil && src.DueRepeatMinutes == nil && src.DueNotifyEnabled == nil {
		return
	}
	_, err := h.q.SetTaskDueNotify(ctx, db.SetTaskDueNotifyParams{
		ID: dstID, DueLeadMinutes: src.DueLeadMinutes,
		DueRepeatMinutes: src.DueRepeatMinutes, DueNotifyEnabled: src.DueNotifyEnabled,
	})
	soft(ctx, "SetTaskDueNotify", err)
}

// carryTaskPins copies where a task was pinned — its stage and its place on the
// Eisenhower matrix — onto the occurrence cloned from it.
//
// Both are plain columns on `tasks` that CreateTask does not take, so the
// duplicate is born with milestone_id and eisenhower_quadrant NULL, while the
// reschedule branch keeps them for free (UpdateTask names neither). Same shape as
// carryDueNotify above, and the same conclusion: what a person pinned has to
// survive the repeat.
//
// The stage is the one that hurts. ListWorkspaceMilestones rolls a milestone up
// over `tasks t ON t.milestone_id = m.id` — task_count, done_count, Σ estimate —
// and the board can be scoped to one stage, so a repeating task pinned to a
// sprint dropped out of both the moment it was first ticked: the progress bar
// lost a task and the scoped board stopped listing it, while the card stood on
// the unscoped board showing «Не задан» where the stage had been. The matrix
// placement went the same way — a card deliberately dragged into "important, not
// urgent" fell back to the derived quadrant, every occurrence (#2932).
//
// Neither is written when the source has none: NULL means "no stage" and "derive
// the quadrant from priority + due date", and both are the right answer to copy.
//
// The milestone needs no workspace check here (applyMilestone's concern): the
// clone is born on the source's own board, so a stage the source could hold is
// one the clone can.
func (h *API) carryTaskPins(ctx context.Context, src db.Task, dstID uuid.UUID) {
	if src.MilestoneID != nil {
		soft(ctx, "SetTaskMilestone", h.q.SetTaskMilestone(ctx, db.SetTaskMilestoneParams{
			ID: dstID, MilestoneID: src.MilestoneID,
		}))
	}
	if src.EisenhowerQuadrant != nil {
		_, err := h.q.SetTaskEisenhower(ctx, db.SetTaskEisenhowerParams{
			ID: dstID, EisenhowerQuadrant: src.EisenhowerQuadrant,
		})
		soft(ctx, "SetTaskEisenhower", err)
	}
}

// taskMarkerRe matches a GFM task-list marker that is ticked: the line's list
// bullet, the bracket, and the x inside it. Mirrors the frontend's
// TASK_MARKER_RE (utils/markdown.js) character for character, including the
// blockquote prefix — «> - [x] …» is drawn with a real box — and the space the
// bracket must be followed by, without which GFM renders literal text.
var taskMarkerRe = regexp.MustCompile("^([ \t>]*(?:[-*+]|[0-9]+[.)])[ \t]+\\[)[xX](\\]([ \t]|$))")

// fenceRe matches the opening or closing line of a fenced code block.
var fenceRe = regexp.MustCompile("^[ \t]{0,3}(```|~~~)")

// untickDescription hands a description to the next occurrence with its
// checkboxes empty again.
//
// A description is the other place a routine's steps get written. RichContent
// draws a real <input> for every GFM task item and a click rewrites the stored
// markdown (toggleTaskMarker), so «- [ ] полить цветы» in the description of a
// daily chore is the same checklist the subtasks are, written the cheap way.
// Everything else about being half-done is undone on purpose when a task
// recurs — completed_at cleared, ReopenSubtasks unticking every item at every
// depth, LiftSubtasksFromColumn taking them out of «Готово» — and the
// description was the one place it was not: from the second occurrence on the
// list came back fully ticked, with no gesture to reset it short of editing the
// markdown by hand every time (#2932). The spent occurrence keeps its ticks;
// it is the record of what was done.
//
// A fenced block is skipped. «- [x] …» inside one is a code SAMPLE — drawn as
// text, with no box — and a description that documents a checklist is exactly
// where such a sample lives; toggleTaskMarker guards the same line from the
// other side. Nothing else about the markdown is touched, and a description
// with no boxes comes back byte for byte.
func untickDescription(desc string) string {
	if !strings.Contains(desc, "[") {
		return desc
	}
	lines := strings.Split(desc, "\n")
	inFence := false
	changed := false
	for i, line := range lines {
		if fenceRe.MatchString(line) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if fresh := taskMarkerRe.ReplaceAllString(line, "$1 $2"); fresh != line {
			lines[i] = fresh
			changed = true
		}
	}
	if !changed {
		return desc
	}
	return strings.Join(lines, "\n")
}

// carryRelations hands a task's links — «связана с», «блокирует», «дублирует» —
// to the occurrence cloned from it. `mapped` is src id → clone id for the whole
// plan: the card and every item of its checklist.
//
// Relations live in a table of their own, so CreateTask knows nothing about them
// and the duplicate was born unlinked, while the reschedule branch keeps them for
// free — the relation rows point at the row it edits in place. The half that
// rots is the one the user did not make: a relation is ONE directed row that BOTH
// ends display (ListTaskRelations reads either direction), so the link did not
// vanish with the tick — it stayed on the spent occurrence. «Выкатить релиз» went
// on saying it was blocked by last week's finished report, the Gantt kept drawing
// its arrow to a card in «Готово», and the occurrence actually standing on the
// board was linked to nothing (#2932).
//
// Direction is preserved by construction: ListTaskRelations already flips an
// incoming row to the reader's point of view (the report `blocks` the release),
// which is exactly the row to write from the clone.
//
// Two ends are treated differently. A link to a task outside the plan is copied
// as is; a link BETWEEN two members of the plan — an item blocking its neighbour,
// a step linked to its own card — is rewired to the new pair through `mapped`,
// which is why this runs after the checklist is rebuilt. Left alone it would pin
// the fresh occurrence to the spent one.
//
// Only user-made links travel. The GitLab projection writes edges of its own
// (gitlab_relations.go, source='gitlab') and reprojects them from the issue
// graph; a copy of one would land as a user row the sync can neither recognise
// nor remove, and would outlive the issue link it was made from.
//
// Each link that really lands is announced like any other (`task.relation`): the
// Gantt keeps its edge list in a fetch of its own and re-reads it on that event
// alone — `task.created` refreshes the bars, not the arrows — so a chart left
// open over the tick would otherwise draw the dependency to the spent occurrence
// until the page was reloaded. Nothing is journalled: the journal records what a
// person did to a task, and the clone's history opens with «created».
// moveDocumentLinks hands a task's document links to the copy of it in the next
// occurrence — the task modal's «Документы» tab on one side, the document's
// «Задачи» panel on the other.
//
// This is the one carry that MOVES instead of copying, and the difference is the
// second end. A relation joins two tasks, and the spent occurrence's «Связи» tab
// is the record of what that week depended on; a document link joins a task to a
// living artefact, and the document's panel lists every link it has with no
// filter on completed_at (DocLinks.vue renders the rows it is handed). A copy
// would therefore leave the регламент naming one more finished task per
// repetition — a daily chore buries the panel inside a month — while the
// reschedule branch, editing the same row, shows exactly one link that always
// names the live occurrence (#2932).
//
// The document is told, because its links panel is fed by its own room
// (docroom.TypeLinks) and hears nothing about tasks: a document open over the
// tick would otherwise go on listing the spent copy until it was reopened.
func (h *API) moveDocumentLinks(ctx context.Context, srcID, dstID uuid.UUID) {
	docs, err := h.q.MoveDocumentTaskLinks(ctx, db.MoveDocumentTaskLinksParams{
		FromTaskID: srcID, ToTaskID: dstID,
	})
	if err != nil {
		soft(ctx, "MoveDocumentTaskLinks", err)
		return
	}
	seen := map[uuid.UUID]bool{}
	for _, docID := range docs {
		if seen[docID] {
			continue // one link per block, one nudge per document
		}
		seen[docID] = true
		h.notifyDocLinks(docID)
	}
}

func (h *API) carryRelations(ctx context.Context, wsID uuid.UUID, mapped map[uuid.UUID]uuid.UUID) {
	for srcID, dstID := range mapped {
		rels, err := h.q.ListTaskRelations(ctx, srcID)
		if err != nil {
			soft(ctx, "ListTaskRelations", err)
			continue
		}
		var dst db.Task
		for _, r := range rels {
			if r.Source != "user" {
				continue
			}
			other := r.RelatedTaskID
			if to, ok := mapped[other]; ok {
				other = to
			}
			if other == dstID {
				continue
			}
			_, addErr := h.q.AddTaskRelation(ctx, db.AddTaskRelationParams{
				TaskID: dstID, RelatedTaskID: other, Kind: r.Kind,
			})
			if errIsNoRows(addErr) {
				continue // the pair is already on record this way
			}
			if addErr != nil {
				soft(ctx, "AddTaskRelation", addErr)
				continue
			}
			if dst.ID == uuid.Nil {
				loaded, gerr := h.q.GetTask(ctx, dstID)
				if gerr != nil {
					soft(ctx, "GetTask", gerr)
					continue
				}
				dst = loaded
			}
			target, gerr := h.q.GetTask(ctx, other)
			if gerr != nil {
				soft(ctx, "GetTask", gerr)
				continue
			}
			h.broadcastRelation(wsID, dst, target, r.Kind)
		}
	}
}

// carryItemRecurrence copies a checklist item's OWN repeat rule onto the copy of
// it in the next occurrence, and returns that copy (the rule lives on `tasks`, so
// writing it produces a newer row).
//
// A subtask may carry a rule of its own: the DueEditor is the same popover
// whether the modal was opened on a card or on one of its items, which is the
// very reason cloneRecurringTask rebuilds a recurring item under the same parent
// instead of as a card. So a weekly report whose step «свериться с бухгалтерией»
// repeats monthly is an ordinary plan.
//
// The parent's rule is put back on the duplicate by hand (setTaskRecurrence, via
// `carry`) because CreateTask takes no recurrence — each item's rule needs the
// same treatment and was not getting it, while the reschedule branch keeps every
// item's rule for free (it edits them in place, and none of ReopenSubtasks /
// LiftSubtasksFromColumn / ShiftSubtaskWindow names the column). From the second
// occurrence on the step simply stopped repeating, with nothing said: the new
// card's popover reads «Не повторять», and the only copy still holding the rule
// is the spent occurrence standing in «Готово» (#2932).
//
// Copied verbatim, one-shots included — a `once` item is one the reschedule
// branch also hands forward until the item itself is ticked. Nothing is written
// when the source has no rule: an item that never repeated must not start.
func (h *API) carryItemRecurrence(ctx context.Context, src, dst db.Task) db.Task {
	if src.Recurrence == nil {
		return dst
	}
	updated, err := h.setTaskRecurrence(ctx, dst, src.Recurrence)
	if err != nil {
		soft(ctx, "carryItemRecurrence", err)
		return dst
	}
	return updated
}

// carryAttachments hands a task's files to the copy of it in the next
// occurrence, description links included.
//
// The template is what makes this a carry rather than a tidiness: a weekly
// report is a rule with the blank form pinned to it, and under «создавать
// копию» the form stayed on the spent copy in «Готово» while the live
// occurrence showed an empty «Вложения». The reschedule branch keeps the files
// for free — it edits the same row, and the attachment rows go on pointing at it.
//
// The FILE is copied, not just the row. storage_path is written under
// <uploadDir>/<task id>/ and purgeTaskUploads wipes that directory per task id,
// on the stated grounds that no row is ever copied to another task — two rows
// sharing one path would make deleting the spent occurrence, the ordinary way to
// clear a finished month, delete the live one's bytes.
//
// The description is rewritten for the same reason. MarkdownEditor writes an
// uploaded file into the text as `[имя](/api/attachments/<id>/download)`, and the
// text travels verbatim, so the duplicate was born naming rows belonging to the
// previous occurrence — which the next delete turns into a dead link, in this
// occurrence and in every one after it (#2932).
//
// Copied rather than moved, unlike moveDocumentLinks: the spent occurrence's
// «Вложения» is the record of what that week produced, and an archive stripped of
// the filled-in report would be worse than the empty one this fixes. That the
// files pile up occurrence by occurrence is the reschedule branch's behaviour
// too — after N repetitions the live task holds the same N files either way,
// which is the standard every other carry here is measured against.
//
// A file that cannot be copied is skipped rather than recorded: a row pointing at
// bytes that are not there is the one state the download route cannot tell from a
// deleted file. Nothing is journalled — the clone's history opens with «created».
func (h *API) carryAttachments(ctx context.Context, src, dst db.Task) db.Task {
	atts, err := h.q.ListTaskAttachments(ctx, src.ID)
	if err != nil {
		soft(ctx, "ListTaskAttachments", err)
		return dst
	}
	if len(atts) == 0 {
		return dst
	}
	dir := h.taskUploadDir(dst.ID)
	if mkErr := os.MkdirAll(dir, 0o750); mkErr != nil {
		soft(ctx, "carryAttachments mkdir", mkErr)
		return dst
	}
	rewrite := map[string]string{}
	for _, a := range atts {
		path := filepath.Join(dir, uuid.NewString()+filepath.Ext(a.Filename))
		if cpErr := copyStoredFile(a.StoragePath, path); cpErr != nil {
			soft(ctx, "carryAttachments copy", cpErr)
			continue
		}
		copied, cErr := h.q.CreateAttachment(ctx, db.CreateAttachmentParams{
			TaskID: dst.ID, UploaderID: a.UploaderID, Filename: a.Filename,
			ContentType: a.ContentType, Size: a.Size, StoragePath: path,
		})
		if cErr != nil {
			_ = os.Remove(path)
			soft(ctx, "CreateAttachment", cErr)
			continue
		}
		rewrite[a.ID.String()] = copied.ID.String()
	}
	return h.relinkAttachments(ctx, dst, rewrite)
}

// relinkAttachments points the copy's description at the copy's own rows.
//
// Only the ids this occurrence really copied are substituted: a link to a file
// that failed to travel is left naming the row that still exists, which at least
// resolves while the spent occurrence does.
func (h *API) relinkAttachments(ctx context.Context, dst db.Task, rewrite map[string]string) db.Task {
	if dst.Description == "" || len(rewrite) == 0 {
		return dst
	}
	text := dst.Description
	for oldID, newID := range rewrite {
		text = strings.ReplaceAll(text, "/attachments/"+oldID+"/", "/attachments/"+newID+"/")
	}
	if text == dst.Description {
		return dst
	}
	if err := h.q.SetTaskDescription(ctx, db.SetTaskDescriptionParams{ID: dst.ID, Description: text}); err != nil {
		soft(ctx, "SetTaskDescription", err)
		return dst
	}
	dst.Description = text
	return dst
}

// copyStoredFile duplicates an attachment's bytes into the next occurrence's
// directory.
func copyStoredFile(from, to string) error {
	// #nosec G304 -- `from` is a storage_path this server wrote (uploadDir/<task
	// id>/<uuid><ext>) and `to` is built the same way here; neither carries
	// anything a client chose.
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	// #nosec G304 -- see above.
	out, err := os.Create(to)
	if err != nil {
		return err
	}
	if _, err = io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// RunRecurrenceWorker drives schedule-triggered recurrences: tasks whose rule
// fires "on schedule" advance automatically once their due date passes, without
// anyone closing them. Idle until such a rule exists.
func (h *API) RunRecurrenceWorker(ctx context.Context) {
	const tick = time.Minute
	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	h.tick(jobRecurrence, opRecurrence)
	h.AdvanceRecurrencesOnce(ctx) // catch up at startup
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.tick(jobRecurrence, opRecurrence)
			h.AdvanceRecurrencesOnce(ctx)
		}
	}
}

// AdvanceRecurrencesOnce runs a single recurrence tick under the worker lock, the
// way every other background sweep here runs. Reports whether the tick ran; false
// means another instance holds the lock and is doing the work.
func (h *API) AdvanceRecurrencesOnce(ctx context.Context) bool {
	return h.withAdvisoryLock(ctx, recurrenceLock, func() { h.advanceScheduleDue(ctx) })
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
