package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

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

// forbiddenErr is a cross-tenant reference: the caller may touch the task the
// route names, but the request *body* names a second task in a workspace they
// are not a member of. Kept apart from opError because it is not a bad argument
// the caller can fix — it renders as a 403, matching TransferTask's refusal of
// a target board in another workspace.
type forbiddenErr struct{ msg string }

func (e forbiddenErr) Error() string { return e.msg }

// staleErr is a lost-update failure: the caller sent the updated_at it had read
// and the stored row has moved on since, so writing would silently drop whoever
// wrote first. It carries the current row — the client needs it to show what it
// is up against and offer a reload or an overwrite.
type staleErr struct{ current db.Task }

func (e staleErr) Error() string { return "задача изменена другим клиентом" }

// errTaskGone is the other way an optimistic-lock write can miss: the row is
// not stale, it is gone — deleted between the caller's read and its write.
var errTaskGone = errors.New("задача удалена")

// respondOpError writes the right status for an op failure: 409 for a lost
// update, 404 for a target that vanished, 400 for something the caller can fix,
// 500 otherwise.
func respondOpError(c *gin.Context, err error) {
	var stale staleErr
	if errors.As(err, &stale) {
		// "stale" rather than an error code: the project has no error-code
		// convention, and 409 + this flag is enough to branch a client on.
		c.JSON(http.StatusConflict, gin.H{"error": stale.Error(), "stale": true, "task": stale.current})
		return
	}
	if errors.Is(err, errTaskGone) {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	var forbidden forbiddenErr
	if errors.As(err, &forbidden) {
		c.JSON(http.StatusForbidden, gin.H{"error": forbidden.Error()})
		return
	}
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

// optJSON is the wire-side counterpart of optTime/optFloat: a request field that
// can be absent, null, or a value. The three are only distinguishable at decode
// time — encoding/json calls UnmarshalJSON for the keys the body actually
// carries, and for those alone — so a plain *T cannot express it, and a client
// that omits a field would silently reset it to the zero value.
type optJSON[T any] struct {
	set bool
	v   *T
}

func (o *optJSON[T]) UnmarshalJSON(b []byte) error {
	o.set = true
	if string(b) == "null" {
		return nil
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	o.v = &v
	return nil
}

// ptr returns the decoded value, or nil when the field was absent or null —
// which is exactly what taskPatch's plain pointer fields mean.
func (o optJSON[T]) ptr() *T { return o.v }

// asTime/asFloat carry the absent/null distinction over into the patch's
// tri-state fields, where a plain pointer would lose it.
func asTime(o optJSON[time.Time]) optTime {
	if !o.set {
		return optTime{}
	}
	return optTime{set: true, v: o.v}
}

func asFloat(o optJSON[float64]) optFloat {
	if !o.set {
		return optFloat{}
	}
	return optFloat{set: true, v: o.v}
}

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
	// ExpectedUpdatedAt is the optimistic-lock token: the updated_at the caller
	// read. Nil means no check — quick actions carry no version, and neither do
	// internal callers, so they keep last-write-wins.
	ExpectedUpdatedAt *time.Time
}

// applyTaskPatch writes a partial task edit and fans out everything a task edit
// implies: completion bookkeeping, recurrence advance, GitLab override marks,
// the journal, participant notifications, writeback triggers and the WS event.
//
// It is the body of UpdateTask, minus request parsing — see the file header.
func (h *API) applyTaskPatch(c *gin.Context, t db.Task, wsID uuid.UUID, p taskPatch) (db.Task, error) {
	title := t.Title
	if p.Title != nil {
		title = normalizeTitle(*p.Title)
		if title == "" {
			return t, userErr("заголовок не может быть пустым")
		}
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

	// Closing a repeating task is a state transition, not a field write: the
	// advance below rewrites the very row this statement writes — due date,
	// completion, start, the whole checklist's window, and under create_new a
	// duplicate on the board. Whether to take it is decided by comparing the row
	// read at the start of the request against the one just written, and nothing
	// between the two is atomic, so two requests that both read the task open
	// both advance it: one tick of the box, two next occurrences.
	//
	// That is not a theoretical interleaving. The board offers the same gesture
	// from several places at once — the card's checkbox and the modal's, two
	// people on one board, a second tap on a phone whose first request is still
	// in flight — and the advance is slow (it rebuilds the checklist, copies
	// every attachment's bytes and rewrites the description), so "still in
	// flight" is hundreds of milliseconds, not the few a person waits before
	// tapping again. Under create_new the extra card then repeats on a series of
	// its own, forever; under reschedule the parent lands on the right date
	// while every item's window is stepped twice (#2932).
	//
	// The lock is the one already here, taken from the inside: no client sends a
	// version (ExpectedUpdatedAt is nil on every real request — the web app
	// doesn't send it and neither does Android), so the check-and-write is armed
	// against the row we read. It is armed for this transition only; an ordinary
	// edit keeps last-write-wins, which is what every caller here expects.
	lock := p.ExpectedUpdatedAt
	mine := false
	if lock == nil && p.Completed != nil && *p.Completed && t.CompletedAt == nil && t.Recurrence != nil {
		lock, mine = &t.UpdatedAt, true
	}
	updated, err := h.q.UpdateTask(c, db.UpdateTaskParams{
		ID: t.ID, Title: title, Description: description,
		Priority: priority, DueDate: dueDate, CompletedAt: completedAt,
		Recurrence: recurrence, StartDate: startDate, Estimate: normalizeEstimate(estimate),
		ExpectedUpdatedAt: lock,
	})
	if err != nil {
		// Under an optimistic lock, "no rows" is not a storage failure: the
		// WHERE clause refused the write. Re-read to tell the two reasons apart
		// — the row moved on (409, hand back the current one) or it is gone
		// (404). Everything below this point is a side effect (journal,
		// notifications, writeback, WS), so a refused write leaves none of it.
		if lock != nil && errors.Is(err, pgx.ErrNoRows) {
			current, gerr := h.q.GetTask(c, t.ID)
			if errors.Is(gerr, pgx.ErrNoRows) {
				return t, errTaskGone
			}
			if gerr != nil {
				return t, gerr
			}
			// A lock the caller did not ask for must not answer 409, and must
			// not swallow a close either. The row moved — by whose hand decides
			// which:
			//
			// Closed already: the request said "close this", the row is closed,
			// and the occurrence that opened is the one the winner opened.
			// Handing back the current task is the honest answer to a gesture
			// made twice, with every side effect skipped — the point of the
			// whole guard.
			//
			// Still open: this was not lost to another close but to something
			// else writing the row in the same breath (the schedule worker, a
			// GitLab sync), and dropping the tick there would be a checkbox that
			// answers 200 and stays empty. Make the close once more against what
			// is on the row now, under the caller's own lock so the retry cannot
			// become the race it is guarding.
			if mine {
				if current.CompletedAt != nil {
					return current, nil
				}
				p.ExpectedUpdatedAt = &current.UpdatedAt
				return h.applyTaskPatch(c, current, wsID, p)
			}
			return t, staleErr{current: current}
		}
		return t, err
	}

	// The row the request itself wrote — what the person asked for, before any
	// automatic step taken on top of it. Everything that records *authorship* (the
	// journal, the notification it feeds, the GitLab "the user overrode this"
	// flags) diffs against this and not against `updated`.
	//
	// A recurring task whose trigger is "complete" advances right below, and the
	// advance rewrites the same row: completion is cleared again and the due date
	// (with the start, and the whole checklist's window) steps forward. Diffed
	// against that, closing the occurrence left no `completed` entry at all — both
	// ends of the diff are open — and handed the reader the recurrence's own step
	// as «изменил срок», signed by whoever ticked the box, with the matching
	// notification to every participant. Dragging the same card into «Готово»
	// journals it correctly (applyMove logs `completed` before advancing), so one
	// card had two histories depending on how it was closed. The step itself is
	// not lost: applyRecur writes its own `recurred` entry carrying the new date,
	// which is what the drag path shows too (#2932).
	edited := updated
	// A recurring task whose trigger is "complete" advances when it enters the
	// completed state (rescheduled, or duplicated, per its rule).
	if t.CompletedAt == nil && updated.CompletedAt != nil {
		if advanced, ok := h.recurAdvance(c, updated, wsID, middleware.CurrentUser(c), recur.TriggerComplete); ok {
			updated = advanced
		}
	}
	// A manual due-date change on a GitLab-linked task wins over the sync. The
	// recurrence's step is not one: taken for `updated`, it froze the linked
	// issue's date sync for good on the first close of a repeating task.
	if !sameTime(t.DueDate, edited.DueDate) {
		soft(c, "MarkGitlabDueOverridden", h.q.MarkGitlabDueOverridden(c, t.ID))
	}
	// Likewise a manual start-date change wins over the sync.
	if !sameTime(t.StartDate, edited.StartDate) {
		soft(c, "MarkGitlabStartOverridden", h.q.MarkGitlabStartOverridden(c, t.ID))
	}
	// A manual estimate change wins over the GitLab timeEstimate pull.
	if !sameEstimate(t.Estimate, edited.Estimate) {
		soft(c, "MarkGitlabEstimateOverridden", h.q.MarkGitlabEstimateOverridden(c, t.ID))
	}
	changes := h.journalUpdate(c, t, edited)
	if len(changes) > 0 {
		h.notifyTaskParticipants(c, updated, wsID, "updated",
			msgUpdated(h.actorName(c), updated.Number, changes))
	}
	// Mirror user-side changes back to a linked GitLab issue (opt-in per integration).
	// These read `updated` deliberately: a writeback mirrors the state the task is
	// actually left in, so a recurrence that moved the deadline is a deadline the
	// linked issue has to learn about, and a close that the recurrence immediately
	// undid is not a close to push — the issue would stay shut under an open task.
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
		// The journal and the notification both name the target column, and both
		// carry its name_key beside the stored name: a seeded column is captioned
		// by the reader's client (#2800), a column the user renamed has no key and
		// stays as written. Entries from before this carry no key and fall back to
		// the stored string, which is what they used to show anyway.
		moved := map[string]any{"to": col.Name}
		if col.NameKey != nil && *col.NameKey != "" {
			moved["to_key"] = *col.NameKey
		}
		h.logEvent(c, t.ID, "moved", moved)
		h.notifyTaskParticipants(c, updated, wsID, "moved",
			msgMoved(h.actorName(c), updated.Number, col.Name, col.NameKey))
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
			// The auto-complete is the other way into the advance, and it races
			// the same way — two drags of one card into «Готово» closed it twice
			// and advanced it twice. Here the row we lock against is the one this
			// request's own move just wrote, so the guard costs a losing drag
			// nothing but the completion it did not win (applyTaskPatch above
			// says the rest).
			if done, derr := h.q.UpdateTask(c, db.UpdateTaskParams{
				ID: updated.ID, Title: updated.Title, Description: updated.Description,
				Priority: updated.Priority, DueDate: updated.DueDate, CompletedAt: &now,
				Recurrence: updated.Recurrence, StartDate: updated.StartDate, Estimate: updated.Estimate,
				ExpectedUpdatedAt: &updated.UpdatedAt,
			}); derr == nil {
				updated = done
				h.logEvent(c, t.ID, "completed", nil)
				// A "complete"-triggered recurring task bounces straight back out of
				// done with its due date advanced — overriding the move just made.
				if advanced, ok := h.recurAdvance(c, updated, wsID, middleware.CurrentUser(c), recur.TriggerComplete); ok {
					updated = advanced
				}
			} else if errors.Is(derr, pgx.ErrNoRows) {
				// The refusal means the row moved on between the move and the
				// completion — the other drag got there first. Answer with what
				// it left, not with the half-state this request happens to hold.
				if current, gerr := h.q.GetTask(c, updated.ID); gerr == nil {
					updated = current
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
		// Same rule as applyTag: the user_id arrives in the body, so loadTask
		// does not vouch for it. Assigning a stranger reads their address back
		// through ListTaskAssignees (it selects u.email) and pushes them an
		// "assigned to you" notification carrying a title the caller writes,
		// from a workspace they cannot open. The sibling notification paths
		// (notifyMentions, notifyThread) already skip non-members.
		//
		// The remove arm stays unguarded on purpose — see
		// TestRevokedMemberCanStillBeUnassigned: RemoveMember leaves existing
		// assignments standing, and a check here would make them unremovable.
		if _, err := h.q.GetMembership(c, db.GetMembershipParams{WorkspaceID: wsID, UserID: userID}); err != nil {
			if errIsNoRows(err) {
				return forbiddenErr{msg: "assignee is not a member of this workspace"}
			}
			return err
		}
		if err := h.q.AddTaskAssignee(c, db.AddTaskAssigneeParams{TaskID: t.ID, UserID: userID}); err != nil {
			return err
		}
		h.logEvent(c, t.ID, "assigned", map[string]any{"user_id": userID})
		h.notify(c, userID, wsID, &t.ID, "assigned",
			msgAssigned(h.actorName(c), t.Number, t.Title))
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
	// The tag must live in the caller's own workspace. loadTask vouches for the
	// task in the route; tag_id arrives separately in the body (or the URL of
	// the delete) and needs its own check — otherwise a stranger's tag attaches
	// to the caller's task and the task detail reads its name and colour back,
	// a cross-workspace read of data the caller was never shown. Tags stay
	// free to cross *projects* inside one workspace, which the board views rely on.
	tag, err := h.q.GetTag(c, tagID)
	if err != nil {
		return userErr("invalid tag")
	}
	if tag.WorkspaceID != wsID {
		return forbiddenErr{msg: "tag is in another workspace"}
	}
	// The journal records the tag by NAME, not by id: an entry has to stay
	// readable after the tag is renamed away or deleted outright (the M:N row
	// cascades, the history line must not turn into a dangling uuid). Same shape
	// as `moved`, which stores the column's name beside its key. A tag has no
	// name_key — tag names are user content everywhere — so there is nothing to
	// caption from the client side.
	if add {
		rows, err := h.q.AddTaskTag(c, db.AddTaskTagParams{TaskID: taskID, TagID: tagID})
		if err != nil {
			return err
		}
		if rows > 0 {
			h.logEvent(c, taskID, "tagged", map[string]any{"tag": tag.Name})
		}
		h.broadcast(wsID, "task.tagged", gin.H{"task_id": taskID, "tag_id": tagID})
	} else {
		rows, err := h.q.RemoveTaskTag(c, db.RemoveTaskTagParams{TaskID: taskID, TagID: tagID})
		if err != nil {
			return err
		}
		if rows > 0 {
			h.logEvent(c, taskID, "untagged", map[string]any{"tag": tag.Name})
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
	// Same rule as applyTag: a milestone named in the body must belong to the
	// caller's workspace. The «Этапы» rollup (ListWorkspaceMilestones) counts
	// every task carrying the id without a workspace filter of its own, so an
	// unchecked milestone_id lets an outsider inflate a stranger's milestone
	// progress with a task the victim can neither see nor unpin.
	var title string
	if milestoneID != nil {
		m, err := h.sameWorkspaceMilestone(c, *milestoneID, wsID)
		if err != nil {
			return err
		}
		title = m.Title
	}
	// Read before writing: the journal entry below is the difference between the
	// two states, and after the UPDATE the previous milestone is unrecoverable.
	before, err := h.q.GetTask(c, taskID)
	if err != nil {
		return err
	}
	if err := h.q.SetTaskMilestone(c, db.SetTaskMilestoneParams{ID: taskID, MilestoneID: milestoneID}); err != nil {
		return err
	}
	h.journalMilestone(c, taskID, before.MilestoneID, milestoneID, title)
	// A manual milestone change on a GitLab-linked task wins over the sync.
	soft(c, "MarkGitlabMilestoneOverridden", h.q.MarkGitlabMilestoneOverridden(c, taskID))
	h.enqueueWriteback(c, taskID, middleware.CurrentUser(c), gitlab.TrigMilestone, map[string]any{})
	if t, err := h.q.GetTask(c, taskID); err == nil {
		h.broadcast(wsID, "task.updated", t)
	}
	return nil
}

// journalMilestone records a milestone change as the pair milestone /
// milestone_removed — the shape every other field of a task has had all along
// (tagged/untagged, assigned/unassigned, attachment/attachment_removed), and the
// one the milestone was missing: pinning a task to a stage, dragging it to
// another one or unpinning it left no trace at all, while the «Этапы» screen
// recomputed task_count / done_count / Σestimate of both stages from it. A
// roadmap that moved and a history that says nothing is exactly the pair a
// reader cannot reconcile, and the /milestone quick action made it worse: the
// history then showed "ran the commands" and not what the command did.
//
// The entry carries the milestone's TITLE rather than its id, for the reason the
// tag does: a milestone may be renamed or deleted (SetTaskMilestone's column is
// ON DELETE SET NULL), and history must not decay into a dangling uuid. A
// re-pin of the milestone the task already carries writes nothing — the same
// no-op guard AddTaskTag gets from its rows count.
func (h *API) journalMilestone(c *gin.Context, taskID uuid.UUID, before, after *uuid.UUID, title string) {
	switch {
	case before == nil && after == nil:
		return
	case before != nil && after != nil && *before == *after:
		return
	case after != nil:
		h.logEvent(c, taskID, "milestone", map[string]any{"title": title})
	default:
		// Cleared: the title of what was there is what the reader needs, and it is
		// only reachable from the row the task pointed at a moment ago. A stage
		// deleted in between leaves the bare phrase rather than a hole.
		var was string
		if m, err := h.q.GetMilestone(c, *before); err == nil {
			was = m.Title
		}
		h.logEvent(c, taskID, "milestone_removed", map[string]any{"title": was})
	}
}

// milestonesLostToBoard names the tasks of a subtree whose milestone the move
// onto targetBoard is about to drop, and the title each one loses. Call it from
// inside the move's transaction, with its tx-bound Queries, BEFORE the update:
// afterwards the task no longer holds the id the title is reachable through.
//
// It reads, it does not unpin. A milestone hangs off a PROJECT while a task moves
// between boards by WORKSPACE, so the three statements a cross-board move runs —
// TransferTask, SetTaskParent, MoveSubtreeToBoard — each clear milestone_id
// themselves when the target board stands in another project (#2932, the split
// that let «Этапы» keep counting a task its own board no longer holds).
//
// What they do not do is say so. The journal pair milestone / milestone_removed
// came later, and a move is the one way a stage comes off a task without anybody
// asking: the reader drags a card to another project's board and its «Этап»
// silently empties, with the History — the thing that pair exists for — showing
// only that it moved. Everything else that clears a field on the way past leaves
// the same entry a deliberate change would (reopened on a transfer out of a done
// column, untagged, attachment_removed), and this is the gap in that row.
//
// The GitLab sync is deliberately left alone: no override mark, no write-back. A
// board move inside Tessera is not a statement about the issue, and pushing
// "milestone removed" to a live GitLab issue because a card changed boards would
// be the larger surprise — nor does this change what the move already did.
func (h *API) milestonesLostToBoard(c *gin.Context, q *db.Queries, rootID, targetBoard uuid.UUID) ([]db.SubtreeMilestonesOffProjectRow, error) {
	project, err := q.ProjectIDForBoard(c, targetBoard)
	if err != nil {
		return nil, err
	}
	return q.SubtreeMilestonesOffProject(c, db.SubtreeMilestonesOffProjectParams{
		RootID: rootID, ProjectID: project,
	})
}

// journalLostMilestones records a milestone_removed entry per row
// milestonesLostToBoard returned. Outside the transaction, like every other
// journal write a move makes (TransferTask's reopened list does the same).
func (h *API) journalLostMilestones(c *gin.Context, rows []db.SubtreeMilestonesOffProjectRow) {
	for _, r := range rows {
		h.logEvent(c, r.ID, "milestone_removed", map[string]any{"title": r.Title})
	}
}

// ── relations ──────────────────────────────────────────────────

// applyRelation links two tasks, journalling both sides so the link shows in the
// referenced task's history too (relations are stored one-way).
//
// Linking a pair that is already linked this way is a no-op, and says so: it
// writes no journal entry and puts nothing on the socket. Pressing «Связать»
// twice, or repeating "/blocks #N", cannot add a second row (AddTaskRelation is
// ON CONFLICT DO NOTHING) — announcing it anyway left both histories collecting
// arrivals for a link that had been there all along, and woke every open Gantt
// to re-read an unchanged graph. applyUnlink already draws exactly this line at
// the other end: it journals only when rows actually went away.
//
// "Already linked this way" is asked of both directions, not just this row's:
// both tasks show the same link whichever end made it (ListTaskRelations reads
// either direction and collapses the pair), and the ✕ takes both halves — so
// #20 "blocked_by" #10 adds nothing to #10 "blocks" #20 but a redundant row.
func (h *API) applyRelation(c *gin.Context, t db.Task, wsID uuid.UUID, target db.Task, kind string) error {
	if target.ID == t.ID {
		return userErr("нельзя связать задачу с собой")
	}
	if !relationKinds[kind] {
		return userErr("неизвестный вид связи: %s", kind)
	}
	existed, err := h.q.RelationExistsBothWays(c, db.RelationExistsBothWaysParams{
		TaskID: t.ID, RelatedTaskID: target.ID,
		Kind: kind, InverseKind: inverseRelationKind(kind),
	})
	if err != nil {
		return err
	}
	if existed {
		return nil
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
	h.broadcastRelation(wsID, t, target, kind)
	return nil
}

// broadcastRelation announces that the link between two tasks changed.
//
// The two `task.updated` events above say the tasks changed, not that the edges
// between them did — and the Gantt view keeps its blocking-edge list in a fetch
// of its own (/boards/:id/dependencies), separate from the board's task lists.
// Without a relation-specific event an open chart would either miss every link
// made elsewhere (the task modal, a second browser, a quick action) or have to
// re-read the whole graph after each unrelated task patch.
//
// Both endpoint boards travel with it: an arrow is drawn only when both ends sit
// on the same board (ListBoardDependencies), so a board can tell from the event
// alone whether the change is any of its business.
func (h *API) broadcastRelation(wsID uuid.UUID, t, target db.Task, kind string) {
	h.broadcast(wsID, "task.relation", gin.H{
		"task_id":          t.ID,
		"related_task_id":  target.ID,
		"kind":             kind,
		"board_id":         t.BoardID,
		"related_board_id": target.BoardID,
	})
}

// logRelationRemoved journals an unlink on both tasks, mirroring what
// applyRelation writes when the link is made.
//
// Both sides, for the same reason the link is recorded on both: a relation is one
// directed row, but both tasks show it and both journals announced its arrival
// ("добавил(а) связь с #N"). Recording only the acting side left the other task's
// history stating a link that is no longer there, with nothing after it — and the
// ✕ in the «Связи» tab (DeleteRelation, the ordinary way either end drops a link)
// wrote nothing at all, on either task. `relation_removed` is not a new event
// kind: both clients have translated it since "/unlink" shipped — it was simply
// unreachable from the button.
//
// Best-effort like every other journal write, and the payload matches the link
// entry's (`related` = the *other* task's number), so the two lines read as a
// pair. The guards are the same too: a task without a number cannot be named in
// the other's history, which is why that side is skipped rather than logged with
// a blank.
func (h *API) logRelationRemoved(c *gin.Context, t, target db.Task) {
	h.logEvent(c, t.ID, "relation_removed", map[string]any{"related": target.Number})
	if t.Number != nil {
		h.logEvent(c, target.ID, "relation_removed", map[string]any{"related": *t.Number})
	}
}

// applyUnlink drops every relation between two tasks, in either direction, and
// reports how many rows went away (0 means there was nothing to unlink).
//
// Relations are stored one-way, so "/unlink #N" has to clear both ends: the row
// may live on either task depending on who linked whom, and a pair can be on
// record twice (the GitLab projection mirrors the reverse edge).
func (h *API) applyUnlink(c *gin.Context, t db.Task, wsID uuid.UUID, target db.Task) (int, error) {
	rows, err := h.q.DeleteRelationsBetween(c, db.DeleteRelationsBetweenParams{
		TaskID: t.ID, RelatedTaskID: target.ID,
	})
	if err != nil {
		return 0, err
	}
	n := int(rows)
	if n > 0 {
		h.logRelationRemoved(c, t, target)
		h.broadcast(wsID, "task.updated", gin.H{"id": t.ID})
		h.broadcast(wsID, "task.updated", gin.H{"id": target.ID})
		// Kind is empty: "/unlink" drops every relation between the pair at once,
		// so no single one names the change. Consumers re-read the edge list.
		h.broadcastRelation(wsID, t, target, "")
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
		// The parent must live in the caller's own workspace. Without this the
		// lines below would adopt a stranger's board and column, physically
		// moving the caller's task onto a board they cannot read — a card the
		// victim never created appearing on their board, and the caller losing
		// their own task. loadTask only vouches for the task in the :id param;
		// parent_id is a second reference and needs its own check.
		if err := h.sameWorkspaceTask(c, parent, wsID); err != nil {
			return t, err
		}
		// …and it must not sit anywhere below this task. Subtask nesting has no
		// depth limit (CreateTask hangs a subtask off a subtask without complaint),
		// so checking only the direct child — as this did — left the two-step
		// version open: A → B → C, then A under C rings all three with no root.
		// A subtask is drawn under its parent and ListBoardTasks takes only
		// parent_id IS NULL, so a ring nobody can reach from a root is drawn
		// nowhere and the cards drop off the board for every member.
		//
		// Same guard as groups and documents, and the walk starts at the parent
		// itself, so "be your own parent" is covered by the same code.
		ancestors, err := h.q.TaskAncestorIDs(c, parent.ID)
		if err != nil {
			return t, err
		}
		for _, id := range ancestors {
			if id == t.ID {
				return t, userErr("cannot attach a task to itself or to one of its own subtasks")
			}
		}
		boardID, columnID = parent.BoardID, parent.ColumnID
	}
	prevParent := t.ParentID
	// A task that changes parent changes the LIST it is ordered in — a subtask's
	// float8 ladder runs per parent, a card's per column — so it needs a position
	// on the destination ladder, not the one it carried over. Both ladders start
	// at the same positionGap, so keeping the old number put the first subtask of
	// a parent on the exact position of the column's first card (the dead slot
	// positionExhausted describes) and a later sibling into the middle of the
	// column, wherever its index among its old siblings happened to point.
	//
	// The end of the list is the answer the rest of the code already gives for
	// "a task joins this list" (applyCreateSubtask, CreateTask). It is read
	// before the update, so the task itself is still in its OLD list and cannot
	// inflate the maximum of the one it is joining. A no-op re-parent keeps the
	// position it has: standing still must not send the card to the bottom.
	position := t.Position
	if !sameParent(prevParent, parentID) {
		var err error
		if position, err = h.nextTaskPosition(c, columnID, parentID); err != nil {
			return t, err
		}
	}
	// Adopting the parent's board moves the task itself — and a task brings its
	// own subtree with it, exactly as TransferTask does. Without this, attaching
	// a task that has children under a parent on another board left the children
	// on the board their parent just left: drawn on neither (a subtask renders
	// only under a parent card present on the same board), and destroyed outright
	// once that board or column is deleted (both foreign keys are ON DELETE
	// CASCADE). One transaction, so a crash cannot split the tree across boards.
	//
	// Attaching also takes the card OUT of the column it was standing in, and the
	// board's rule for that gesture is applyMove's: what leaves a done column for a
	// column that does not complete is reopened. /transfer — the other door onto
	// board_id and column_id — already follows it; this one did not, so a finished
	// card pulled under a parent in «К работе» landed struck through in a column
	// that completes nothing, still counted as done, and out of reach of the reopen
	// rule, which fires only for a task standing in a done column. Dragging it into
	// «Готово» and back out was the only way to undo that.
	//
	// Resolved before the transaction: it reads the two boards' done-column
	// pointers, which nothing below writes. A detach moves no card (columnID is the
	// task's own), hence the guard.
	var leavingDone *uuid.UUID
	if columnID != t.ColumnID {
		leavingDone = h.doneColumnLeftBy(c, t, boardID, columnID)
	}
	var updated db.Task
	var reopened []uuid.UUID
	var lostMilestones []db.SubtreeMilestonesOffProjectRow
	if err := h.inTx(c, func(q *db.Queries) error {
		// The adopted board may belong to another project, in which case the two
		// statements below drop the subtree's milestones — read what they take
		// while it is still readable (see milestonesLostToBoard). Same board, same
		// project: nothing to lose.
		if boardID != t.BoardID {
			var err error
			if lostMilestones, err = h.milestonesLostToBoard(c, q, t.ID, boardID); err != nil {
				return err
			}
		}
		// Before the writes below: the reopen condition is the column being left,
		// which they are about to overwrite. Which cards are leaving depends on how
		// far the attach reaches — a cross-board attach carries the whole subtree
		// into the parent's column (MoveSubtreeToBoard), a same-board one moves only
		// this task, and reopening a subtree there would clear the flag of a
		// finished child that never left «Готово».
		if leavingDone != nil {
			var err error
			if boardID != t.BoardID {
				reopened, err = q.ReopenSubtreeLeavingDone(c, db.ReopenSubtreeLeavingDoneParams{
					RootID: t.ID, DoneColumnID: *leavingDone,
				})
			} else {
				reopened, err = q.ReopenTaskLeavingDone(c, db.ReopenTaskLeavingDoneParams{
					RootID: t.ID, DoneColumnID: *leavingDone,
				})
			}
			if err != nil {
				return err
			}
		}
		var err error
		updated, err = q.SetTaskParent(c, db.SetTaskParentParams{
			ID: t.ID, ParentID: parentID, BoardID: boardID, ColumnID: columnID, Position: position,
		})
		if err != nil || boardID == t.BoardID {
			// Same board: the descendants stay where they are, keeping whatever
			// column each of them sits in.
			return err
		}
		return q.MoveSubtreeToBoard(c, db.MoveSubtreeToBoardParams{
			ParentID: &t.ID, BoardID: boardID, ColumnID: columnID,
		})
	}); err != nil {
		return t, err
	}
	h.journalLostMilestones(c, lostMilestones)
	actor := middleware.CurrentUser(c)
	// Same trail an in-board reopen leaves, and the one /transfer leaves for the
	// same statement: a journal entry per task, and the completion mirrored to
	// GitLab (the outbox drops the ones with no issue).
	for _, rid := range reopened {
		h.logEvent(c, rid, "reopened", nil)
		h.enqueueWriteback(c, rid, actor, gitlab.TrigCompletion, map[string]any{"completed": false})
	}
	// Mirror the move in GitLab's hierarchy (#2592). Re-parenting to a different parent
	// is one attach: workItemUpdate replaces the parent, so an explicit detach first
	// would only produce a window where the child hangs at top-level.
	switch {
	case parentID != nil:
		h.enqueueChildAttach(c, t.ID, *parentID, actor)
	case prevParent != nil:
		h.enqueueChildDetach(c, t.ID, *prevParent, actor)
	}
	h.broadcastAs(c, wsID, "task.moved", updated)
	return updated, nil
}

// sameWorkspaceTask reports whether a task named in a request body belongs to
// wsID, the workspace the caller was already authorized against. Use it for
// every task id that arrives *besides* the one in the route — those the route
// resolves go through loadTask, these do not.
func (h *API) sameWorkspaceTask(c *gin.Context, t db.Task, wsID uuid.UUID) error {
	other, err := h.q.WorkspaceIDForBoard(c, t.BoardID)
	if err != nil {
		return userErr("invalid parent")
	}
	if other != wsID {
		return forbiddenErr{msg: "task is in another workspace"}
	}
	return nil
}

// sameWorkspaceMilestone is sameWorkspaceTask for a milestone id from a request
// body: milestones hang off a project, so the workspace comes one hop away. The
// milestone itself comes back because the caller journals it by title and would
// otherwise read the same row a second time.
func (h *API) sameWorkspaceMilestone(c *gin.Context, milestoneID, wsID uuid.UUID) (db.Milestone, error) {
	m, err := h.q.GetMilestone(c, milestoneID)
	if err != nil {
		return db.Milestone{}, userErr("invalid milestone")
	}
	other, err := h.q.WorkspaceIDForProject(c, m.ProjectID)
	if err != nil {
		return db.Milestone{}, userErr("invalid milestone")
	}
	if other != wsID {
		return db.Milestone{}, forbiddenErr{msg: "milestone is in another workspace"}
	}
	return m, nil
}

// applyArchive soft-deletes a task. detach keeps its subtasks on the board
// (re-parented to null); otherwise they are archived with the parent.
func (h *API) applyArchive(c *gin.Context, t db.Task, wsID uuid.UUID, detach bool) error {
	var detached []uuid.UUID
	// Transactional: detach-then-archive must be atomic, else a crash leaves the
	// parent archived with its subtasks still active on the board (or vice versa).
	if err := h.inTx(c, func(q *db.Queries) error {
		if detach {
			var err error
			if detached, err = q.DetachChildren(c, &t.ID); err != nil {
				return err
			}
			return q.ArchiveTask(c, t.ID)
		}
		return q.ArchiveTaskCascade(c, t.ID)
	}); err != nil {
		return err
	}
	h.pushDetachedChildren(c, detached, t.ID)
	h.logEvent(c, t.ID, "archived", nil)
	h.notifyTaskParticipants(c, t, wsID, "archived",
		msgArchived(h.actorName(c), t.Number, t.Title))
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
	h.enqueueChildAttach(c, sub.ID, parent.ID, uid) // GitLab child work item (#2592)
	h.broadcastAs(c, wsID, "task.created", sub)
	return sub, nil
}
