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
	h.scanDueTasks(ctx)
	h.scanReminders(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.tick(jobNotifyScanner, opDueScan)
			h.scanDueTasks(ctx)
			h.scanReminders(ctx)
		}
	}
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
	midnight := time.Date(lt.Year(), lt.Month(), lt.Day(), 0, 0, 0, 0, loc)
	end := midnight.Add(time.Duration(endMin) * time.Minute)
	if !end.After(lt) {
		end = end.Add(24 * time.Hour) // end already passed today → resumes tomorrow
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
	if prior == nil || !prior.FiredDue.Equal(due) {
		return true // never fired, or the due date moved → re-arm
	}
	if repeat <= 0 {
		return false // one-shot, already fired
	}
	return !now.Before(prior.LastFiredAt.Add(time.Duration(repeat) * time.Minute))
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
			h.deliverNotification(ctx, r.UserID, h.reminderWorkspace(ctx, r), r.TaskID, nil,
				"reminder", msgReminder(r.Message))
		}
		soft(ctx, "MarkReminderNotified", h.q.MarkReminderNotified(ctx, r.ID))
	}
}

// reminderWorkspace resolves a workspace to scope a reminder notification to: the
// linked task's workspace, else the user's first workspace (reminders aren't
// workspace-scoped, but notifications carry a workspace id).
func (h *API) reminderWorkspace(ctx context.Context, r db.Reminder) uuid.UUID {
	if r.TaskID != nil {
		if t, err := h.q.GetTask(ctx, *r.TaskID); err == nil {
			if ws, werr := h.q.WorkspaceIDForBoard(ctx, t.BoardID); werr == nil {
				return ws
			}
		}
	}
	if wss, err := h.q.ListWorkspacesForUser(ctx, r.UserID); err == nil && len(wss) > 0 {
		return wss[0].ID
	}
	return uuid.Nil
}
