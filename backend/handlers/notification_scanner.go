package handlers

import (
	"context"
	"time"

	"github.com/google/uuid"

	"tessera/internal/db"
)

// Phase B scanner: the background sweep that emits notifications for upcoming or
// overdue task due dates and for reminders whose time has arrived. Emitted
// notifications flow through the same routing + outbox as interactive ones.

const notifyScanTick = 60 * time.Second

// RunNotificationScanner periodically emits notifications for upcoming/overdue
// task due dates (per the user's lead/repeat prefs) and for reminders whose time
// has arrived. The emitted notifications flow through the same routing + outbox as
// any other. Blocks until ctx is done; start it in a goroutine.
func (h *API) RunNotificationScanner(ctx context.Context) {
	ticker := time.NewTicker(notifyScanTick)
	defer ticker.Stop()
	h.tick(jobNotifyScanner, opDueScan)
	h.ScanNotificationsOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.tick(jobNotifyScanner, opDueScan)
			h.ScanNotificationsOnce(ctx)
		}
	}
}

// ScanNotificationsOnce runs a single scanner tick under the worker lock, the
// way every other background sweep here runs.
//
// Neither scan claims its rows: ListDueReminders and ListDueTasksForScan just
// select what is due, and the due-date state is read and written in two separate
// statements. So two processes ticking at once — a deploy overlap, where the old
// instance keeps ticking while the new one boots — both see the same reminder
// and the same due task, and the recipient gets each notification twice, out to
// e-mail and push. Marking the reminder notified afterwards is too late.
//
// Reports whether the tick ran; false means another instance holds the lock and
// is doing the work.
func (h *API) ScanNotificationsOnce(ctx context.Context) bool {
	return h.withAdvisoryLock(ctx, NotifyScanLock, func() {
		h.scanDueTasks(ctx)
		h.scanReminders(ctx)
	})
}

// defaultPrefs is the scheduling config for a user who hasn't customised it.
func defaultPrefs(uid uuid.UUID) db.NotificationPref {
	return db.NotificationPref{
		UserID: uid, DueEnabled: true, DueLeadMinutes: 60, DueRepeatMinutes: 0, ReminderEnabled: true,
		QuietEnabled: false, QuietStartMinutes: 1320, QuietEndMinutes: 480,
	}
}

// quietWindow reports whether now falls inside the user's quiet window and, if so,
// the absolute time the window ends (so a deferred delivery resumes then). Bounds
// are minutes-since-midnight in tz (IANA; "" = UTC); a window may wrap past
// midnight (start > end, e.g. 22:00–08:00).
func quietWindow(enabled bool, startMin, endMin int, tz string, now time.Time) (time.Time, bool) {
	if !enabled || startMin == endMin {
		return time.Time{}, false
	}
	loc := time.UTC
	if tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		}
	}
	lt := now.In(loc)
	mins := lt.Hour()*60 + lt.Minute()
	var inQuiet bool
	if startMin < endMin {
		inQuiet = mins >= startMin && mins < endMin
	} else {
		inQuiet = mins >= startMin || mins < endMin
	}
	if !inQuiet {
		return time.Time{}, false
	}
	// The bound is a reading on the user's wall clock, so it is built as one —
	// time.Date normalises the out-of-range minute — and not as a duration from
	// local midnight. A day with a clock change is not 24 hours long: in autumn
	// midnight+7h lands at 06:00 and the phone starts buzzing an hour before the
	// window says it may, in spring it lands at 08:00 and holds an hour too long.
	// Same reason "tomorrow" is the next calendar day rather than +24h.
	end := time.Date(lt.Year(), lt.Month(), lt.Day(), 0, endMin, 0, 0, loc)
	if !end.After(lt) {
		// end already passed today → resumes tomorrow
		end = time.Date(lt.Year(), lt.Month(), lt.Day()+1, 0, endMin, 0, 0, loc)
	}
	return end, true
}

// scanDueTasks fires due-date notifications. For each candidate task and each of
// its participants it resolves the effective (per-task override → user default)
// enable/lead/repeat, then uses the per-(task,user) state to fire once at the lead
// window and, when a repeat interval is set, again every interval. The state
// snapshots the due_date it fired for, so editing the due date re-arms it.
func (h *API) scanDueTasks(ctx context.Context) {
	tasks, err := h.q.ListDueTasksForScan(ctx)
	if err != nil {
		return
	}
	now := time.Now()
	prefsCache := map[uuid.UUID]db.NotificationPref{}
	getPrefs := func(uid uuid.UUID) db.NotificationPref {
		if p, ok := prefsCache[uid]; ok {
			return p
		}
		p, perr := h.q.GetNotificationPrefs(ctx, uid)
		if perr != nil {
			p = defaultPrefs(uid)
		}
		prefsCache[uid] = p
		return p
	}
	for _, t := range tasks {
		if t.DueDate == nil {
			continue
		}
		wsID, werr := h.q.WorkspaceIDForBoard(ctx, t.BoardID)
		if werr != nil {
			continue
		}
		for _, uid := range h.dueRecipients(ctx, t) {
			p := getPrefs(uid)
			enabled := p.DueEnabled
			if t.DueNotifyEnabled != nil {
				enabled = *t.DueNotifyEnabled
			}
			if !enabled {
				continue
			}
			lead := p.DueLeadMinutes
			if t.DueLeadMinutes != nil {
				lead = *t.DueLeadMinutes
			}
			repeat := p.DueRepeatMinutes
			if t.DueRepeatMinutes != nil {
				repeat = *t.DueRepeatMinutes
			}
			var prior *db.DueNotificationState
			if st, serr := h.q.GetDueNotificationState(ctx, db.GetDueNotificationStateParams{TaskID: t.ID, UserID: uid}); serr == nil {
				prior = &st
			}
			if !dueShouldFire(now, *t.DueDate, lead, repeat, prior) {
				continue
			}
			// A repeat is a nag — "keep reminding me until I deal with it" — and a
			// nag that cannot reach anyone is not a nag, it is a queue. Inside quiet
			// hours routeNotification defers every external delivery to the end of
			// the window, and the deferred rows all land on that same instant: a
			// task repeating every 15 minutes across a 22:00–08:00 night stockpiles
			// forty identical messages and empties them into the inbox at 08:00, out
			// of the one feature whose whole purpose is that the night stays quiet.
			// So the repeat is dropped rather than deferred. The first warning is
			// not: it still fires and still waits for the window to end, so the
			// morning carries it exactly once. Nothing is advanced here either —
			// last_fired_at keeps its old value, so the repeat resumes on the first
			// tick after the window (#2932).
			if dueAlreadyFired(*t.DueDate, prior) {
				if _, inQuiet := quietWindow(p.QuietEnabled, int(p.QuietStartMinutes),
					int(p.QuietEndMinutes), p.QuietTz, now); inQuiet {
					continue
				}
			}
			h.deliverNotification(ctx, uid, wsID, &t.ID, nil, "due_soon", dueMsg(t))
			soft(ctx, "UpsertDueNotificationState", h.q.UpsertDueNotificationState(ctx, db.UpsertDueNotificationStateParams{
				TaskID: t.ID, UserID: uid, FiredDue: *t.DueDate,
			}))
		}
	}
}

// dueShouldFire decides whether a due-date notification should fire now, given the
// effective lead/repeat (minutes), the task's due date, the prior per-(task,user)
// state (nil = never fired), and the current time. It fires once when now enters
// the lead window [due-lead, ∞); if the due date changed since the last fire it
// re-arms; and with a positive repeat it fires again every repeat minutes.
func dueShouldFire(now, due time.Time, lead, repeat int32, prior *db.DueNotificationState) bool {
	if now.Before(due.Add(-time.Duration(lead) * time.Minute)) {
		return false // not yet inside the lead window
	}
	if !dueAlreadyFired(due, prior) {
		return true // never fired, or the due date moved → re-arm
	}
	if repeat <= 0 {
		return false // one-shot, already fired
	}
	return !now.Before(prior.LastFiredAt.Add(time.Duration(repeat) * time.Minute))
}

// dueAlreadyFired reports whether the per-(task,user) state records a fire for
// this exact due date — i.e. whether a fire now would be a repeat nag rather
// than the first warning. A due date that moved since the last fire re-arms the
// state, so it counts as never fired.
func dueAlreadyFired(due time.Time, prior *db.DueNotificationState) bool {
	return prior != nil && prior.FiredDue.Equal(due)
}

// dueRecipients are the users who hear about a task's due date: its assignees plus
// its creator, deduped.
func (h *API) dueRecipients(ctx context.Context, t db.Task) []uuid.UUID {
	seen := map[uuid.UUID]bool{}
	var out []uuid.UUID
	if as, err := h.q.ListTaskAssignees(ctx, t.ID); err == nil {
		for _, a := range as {
			if !seen[a.ID] {
				seen[a.ID] = true
				out = append(out, a.ID)
			}
		}
	}
	if t.CreatedBy != nil && !seen[*t.CreatedBy] {
		out = append(out, *t.CreatedBy)
	}
	return out
}

// dueMsg is the content of a due-date notification: the structured payload plus
// the default Russian sentence (templates can reformat the latter via the
// channel template).
func dueMsg(t db.Task) notifyMsg {
	return msgDueSoon(t.Number, t.Title)
}

// scanReminders routes reminders whose time has come to the user's channels (once,
// alongside the Android local alarm). Each due reminder is marked processed so it
// isn't reconsidered — toggling reminder delivery on later won't replay old ones.
func (h *API) scanReminders(ctx context.Context) {
	rs, err := h.q.ListDueReminders(ctx)
	if err != nil {
		return
	}
	for _, r := range rs {
		p, perr := h.q.GetNotificationPrefs(ctx, r.UserID)
		if perr != nil {
			p = defaultPrefs(r.UserID)
		}
		if p.ReminderEnabled {
			ws, taskID := h.reminderScope(ctx, r)
			h.deliverNotification(ctx, r.UserID, ws, taskID, nil,
				"reminder", msgReminder(r.Message))
		}
		soft(ctx, "MarkReminderNotified", h.q.MarkReminderNotified(ctx, r.ID))
	}
}

// reminderScope resolves what a reminder's notification may name: a workspace to
// scope it to — the linked task's, else the user's first (reminders aren't
// workspace-scoped, but notifications carry a workspace id) — and the task link
// itself, kept only while the reader may still follow it.
//
// The linked task's workspace is used only while the user is still a member of
// it. Otherwise this function would hand deliverNotification a workspace its
// membership gate then refuses, and the reminder would be dropped in silence —
// on a task that walked out of reach after the reminder was set (the project was
// transferred, the member was removed) that silence falls on a legitimate
// reminder whose text is the user's own. The reminder still has to fire.
//
// The task link is dropped on that same fallback, because everything downstream
// follows a task_id without re-asking about membership: ListNotifications LEFT
// JOINs tasks for number/board, deliverNotification enriches the socket payload
// the same way, and templateData renders the task's *current* title into the
// e-mail or telegram message. Carried past the gate, the fallback that keeps a
// private reminder alive would also carry a former colleague's edits out with
// it. What survives is the part that was always the user's own: their text.
func (h *API) reminderScope(ctx context.Context, r db.Reminder) (uuid.UUID, *uuid.UUID) {
	if r.TaskID != nil {
		if t, err := h.q.GetTask(ctx, *r.TaskID); err == nil {
			if ws, werr := h.q.WorkspaceIDForBoard(ctx, t.BoardID); werr == nil &&
				h.notifiableInWorkspace(ctx, r.UserID, ws) {
				return ws, r.TaskID
			}
		}
	}
	if wss, err := h.q.ListWorkspacesForUser(ctx, r.UserID); err == nil && len(wss) > 0 {
		return wss[0].ID, nil
	}
	return uuid.Nil, nil
}
