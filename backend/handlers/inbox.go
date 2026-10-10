package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/db"
	"tessera/middleware"
)

// «Входящие» (#2972): the user's reminders and notes of one workspace as one
// feed. The two kinds stay two tables (decision #3036 — their delivery and life
// cycles differ, and the Android client reads them through their own routes);
// this endpoint only puts them side by side and attaches the task each one is
// tied to. Grouping («Сработало / Сегодня / Позже / Заметки») is the client's:
// it depends on the reader's clock and midnight, not the server's.

type inboxTask = db.InboxTaskChipsRow

type inboxReminder struct {
	db.Reminder
	Task *inboxTask `json:"task"`
}

type inboxNote struct {
	db.Note
	Task *inboxTask `json:"task"`
}

// Inbox answers GET /workspaces/:id/inbox:
//
//	reminders — the caller's reminders of this workspace and those outside any
//	            workspace; open ones and the ones done in the last 36 hours;
//	notes     — the workspace's notes the caller may read (shared + own
//	            private), most recently edited first, capped.
//
// A record's `task` is null when it has none, or when the task is no longer in
// this workspace (transferred) or sits on a deleted board.
//
// `?tag=name` keeps only the records wearing that tag (#3040), compared the way
// tag names are compared everywhere — trimmed, case-insensitive, «#» optional.
func (h *API) Inbox(c *gin.Context) {
	wsID, ok := parseID(c, "id")
	if !ok || !h.requireMember(c, wsID) {
		return
	}
	me := middleware.CurrentUser(c)
	rs, err := h.q.InboxReminders(c, db.InboxRemindersParams{UserID: me, WorkspaceID: &wsID})
	if err != nil {
		fail(c, err)
		return
	}
	ns, err := h.q.InboxNotes(c, db.InboxNotesParams{WorkspaceID: wsID, Viewer: me})
	if err != nil {
		fail(c, err)
		return
	}
	if tag := strings.ToLower(strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(c.Query("tag")), "#"))); tag != "" {
		rs = keepTagged(rs, func(r db.Reminder) []string { return r.Tags }, tag)
		ns = keepTagged(ns, func(n db.Note) []string { return n.Tags }, tag)
	}
	seen := map[uuid.UUID]bool{}
	var ids []uuid.UUID
	add := func(id *uuid.UUID) {
		if id != nil && !seen[*id] {
			seen[*id] = true
			ids = append(ids, *id)
		}
	}
	for _, r := range rs {
		add(r.TaskID)
	}
	for _, n := range ns {
		add(n.TaskID)
	}
	chips := map[uuid.UUID]*inboxTask{}
	if len(ids) > 0 {
		rows, err := h.q.InboxTaskChips(c, db.InboxTaskChipsParams{Ids: ids, WorkspaceID: wsID})
		if err != nil {
			fail(c, err)
			return
		}
		for i := range rows {
			chips[rows[i].ID] = &rows[i]
		}
	}
	chip := func(id *uuid.UUID) *inboxTask {
		if id == nil {
			return nil
		}
		return chips[*id]
	}
	outR := make([]inboxReminder, 0, len(rs))
	for _, r := range rs {
		outR = append(outR, inboxReminder{Reminder: r, Task: chip(r.TaskID)})
	}
	outN := make([]inboxNote, 0, len(ns))
	for _, n := range ns {
		outN = append(outN, inboxNote{Note: n, Task: chip(n.TaskID)})
	}
	c.JSON(http.StatusOK, gin.H{"reminders": outR, "notes": outN})
}

// keepTagged keeps the records one of whose tags is `tag` (already lowercased).
func keepTagged[T any](list []T, tagsOf func(T) []string, tag string) []T {
	out := list[:0]
	for _, r := range list {
		for _, name := range tagsOf(r) {
			if strings.ToLower(strings.TrimSpace(name)) == tag {
				out = append(out, r)
				break
			}
		}
	}
	return out
}
