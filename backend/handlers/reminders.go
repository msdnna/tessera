package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/db"
	"tessera/middleware"
)

// Reminders are personal (scoped to the current user), so authorization is a
// plain owner check rather than workspace membership.
//
// Since 0081 (#2972) a reminder also carries a body (Markdown; message stays its
// title), done_at, snoozed_from and the workspace whose «Входящие» it lives in.
// The older clients (Android) keep working: they send remind_at/message/done,
// and every new field is optional on the way in.

// reminderWorkspace picks the workspace a new reminder belongs to: its task's,
// else the one the client named (the caller must be a member), else none.
func (h *API) reminderWorkspace(c *gin.Context, taskID, wsID *uuid.UUID) (*uuid.UUID, bool) {
	if taskID != nil {
		// The task arrives in the body, so nothing in the route vouches for it —
		// the same seam TestTagMustBeInSameWorkspace pins for POST /tasks/:id/tags.
		// Unchecked, a reminder is a reference into a workspace the caller was
		// never in; it reads nothing back today, but the id is stored, travels with
		// the notification, and is one join away from being rendered.
		_, taskWS, ok := h.loadTask(c, *taskID)
		if !ok {
			return nil, false
		}
		return &taskWS, true
	}
	if wsID != nil {
		if !h.requireMember(c, *wsID) {
			return nil, false
		}
		return wsID, true
	}
	return nil, true
}

// broadcastReminder tells the owner's other tabs and devices about a change.
// The event is addressed to the owner alone (it carries their private text) and
// rides on the reminder's workspace — or, for one outside every workspace, on
// whichever workspace reminderScope would deliver it through: one socket holds
// all of the user's workspaces, so any of them reaches every client.
func (h *API) broadcastReminder(c *gin.Context, r db.Reminder, eventType string, payload any) {
	var ws uuid.UUID
	if r.WorkspaceID != nil {
		ws = *r.WorkspaceID
	} else {
		ws, _ = h.reminderScope(c, r)
	}
	if ws == uuid.Nil {
		return
	}
	h.broadcastToUser(ws, r.UserID, eventType, payload)
}

// CreateReminder schedules a personal reminder, optionally linked to a task.
func (h *API) CreateReminder(c *gin.Context) {
	var req struct {
		TaskID      *uuid.UUID  `json:"task_id"`
		RemindAt    time.Time   `json:"remind_at" binding:"required"`
		Message     string      `json:"message"`
		Body        string      `json:"body"`
		WorkspaceID *uuid.UUID  `json:"workspace_id"`
		Tags        []string    `json:"tags"`
		Mentions    []uuid.UUID `json:"mentions"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ws, ok := h.reminderWorkspace(c, req.TaskID, req.WorkspaceID)
	if !ok {
		return
	}
	tags, ok := normalizeRecordTags(c, req.Tags)
	if !ok {
		return
	}
	// A reminder is its owner's alone: the people it names are a reference,
	// never notified (#3040).
	mentions, ok := h.recordMentions(c, ws, req.Mentions)
	if !ok {
		return
	}
	r, err := h.q.CreateReminder(c, db.CreateReminderParams{
		UserID:      middleware.CurrentUser(c),
		TaskID:      req.TaskID,
		RemindAt:    req.RemindAt,
		Message:     req.Message,
		Body:        req.Body,
		WorkspaceID: ws,
		Tags:        h.canonRecordTags(c, h.taskProjectID(c, req.TaskID), tags),
		Mentions:    mentions,
	})
	if err != nil {
		fail(c, err)
		return
	}
	h.broadcastReminder(c, r, "reminder.created", r)
	c.JSON(http.StatusCreated, r)
}

// ListReminders returns the current user's reminders (earliest first).
func (h *API) ListReminders(c *gin.Context) {
	rs, err := h.q.ListReminders(c, middleware.CurrentUser(c))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, rs)
}

// UpdateReminder edits a reminder. Every field is optional: absent keeps the
// current value (the old clients send remind_at/message/done every time, so
// they keep their full-replace behaviour), task_id and snoozed_from also take
// null. A new time that does not come with snoozed_from is a plain reschedule
// and drops the «отложено» mark; done toggles done_at with it.
func (h *API) UpdateReminder(c *gin.Context) {
	r, ok := h.loadReminder(c)
	if !ok {
		return
	}
	var req struct {
		RemindAt    *time.Time      `json:"remind_at"`
		Message     *string         `json:"message"`
		Done        *bool           `json:"done"`
		Body        *string         `json:"body"`
		TaskID      json.RawMessage `json:"task_id"`
		SnoozedFrom json.RawMessage `json:"snoozed_from"`
		Tags        *[]string       `json:"tags"`
		Mentions    *[]uuid.UUID    `json:"mentions"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	p := db.UpdateReminderParams{
		ID: r.ID, RemindAt: r.RemindAt, Message: r.Message, Done: r.Done, Body: r.Body,
		TaskID: r.TaskID, WorkspaceID: r.WorkspaceID, SnoozedFrom: r.SnoozedFrom,
		Tags: r.Tags, Mentions: r.Mentions,
	}
	if req.Tags != nil {
		if p.Tags, ok = normalizeRecordTags(c, *req.Tags); !ok {
			return
		}
	}
	if req.Message != nil {
		p.Message = *req.Message
	}
	if req.Done != nil {
		p.Done = *req.Done
	}
	if req.Body != nil {
		p.Body = *req.Body
	}
	if req.RemindAt != nil {
		if !req.RemindAt.Equal(r.RemindAt) {
			p.SnoozedFrom = nil
		}
		p.RemindAt = *req.RemindAt
	}
	switch {
	case req.SnoozedFrom == nil:
	case bytes.Equal(bytes.TrimSpace(req.SnoozedFrom), []byte("null")):
		p.SnoozedFrom = nil
	default:
		var at time.Time
		if err := json.Unmarshal(req.SnoozedFrom, &at); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid snoozed_from"})
			return
		}
		p.SnoozedFrom = &at
	}
	taskID, taskSet, err := readTriStateID(req.TaskID, r.TaskID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task_id"})
		return
	}
	if taskSet {
		p.TaskID = taskID
		if taskID != nil {
			ws, ok := h.reminderWorkspace(c, taskID, nil)
			if !ok {
				return
			}
			p.WorkspaceID = ws
		}
	}
	if req.Mentions != nil {
		if p.Mentions, ok = h.recordMentions(c, p.WorkspaceID, *req.Mentions); !ok {
			return
		}
	}
	p.Tags = h.canonRecordTags(c, h.taskProjectID(c, p.TaskID), p.Tags)
	h.saveReminder(c, r, p)
}

// SnoozeReminder moves a reminder to a later time («Отложить», #2997) and keeps
// the time it was first due in snoozed_from — the chip «с 12:00» — however many
// times it is snoozed. A done reminder snoozed is open again.
func (h *API) SnoozeReminder(c *gin.Context) {
	r, ok := h.loadReminder(c)
	if !ok {
		return
	}
	var req struct {
		RemindAt time.Time `json:"remind_at" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	from := r.SnoozedFrom
	if from == nil {
		from = &r.RemindAt
	}
	h.saveReminder(c, r, db.UpdateReminderParams{
		ID: r.ID, RemindAt: req.RemindAt, Message: r.Message, Done: false, Body: r.Body,
		TaskID: r.TaskID, WorkspaceID: r.WorkspaceID, SnoozedFrom: from,
		Tags: r.Tags, Mentions: r.Mentions,
	})
}

func (h *API) saveReminder(c *gin.Context, before db.Reminder, p db.UpdateReminderParams) {
	// The columns are NOT NULL and pgx sends a nil slice as NULL.
	if p.Tags == nil {
		p.Tags = []string{}
	}
	if p.Mentions == nil {
		p.Mentions = []uuid.UUID{}
	}
	updated, err := h.q.UpdateReminder(c, p)
	if err != nil {
		fail(c, err)
		return
	}
	// Moved to another workspace's feed (linked to a task over there): the old
	// one's clients drop it, the new one's pick it up.
	if before.WorkspaceID != nil && (updated.WorkspaceID == nil || *updated.WorkspaceID != *before.WorkspaceID) {
		h.broadcastReminder(c, before, "reminder.deleted", gin.H{"id": before.ID})
	}
	h.broadcastReminder(c, updated, "reminder.updated", updated)
	c.JSON(http.StatusOK, updated)
}

// DeleteReminder removes a reminder.
func (h *API) DeleteReminder(c *gin.Context) {
	r, ok := h.loadReminder(c)
	if !ok {
		return
	}
	if err := h.q.DeleteReminder(c, r.ID); err != nil {
		fail(c, err)
		return
	}
	h.broadcastReminder(c, r, "reminder.deleted", gin.H{"id": r.ID})
	c.Status(http.StatusNoContent)
}

// ListTaskReminders lists the caller's own reminders on a task (the task
// modal's «Напоминания и заметки», #2998). Other members' reminders on the same
// task are theirs and never shown.
func (h *API) ListTaskReminders(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	t, _, ok := h.loadTask(c, id)
	if !ok {
		return
	}
	rs, err := h.q.ListTaskReminders(c, db.ListTaskRemindersParams{
		TaskID: &t.ID, UserID: middleware.CurrentUser(c),
	})
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, orEmpty(rs))
}

func (h *API) loadReminder(c *gin.Context) (db.Reminder, bool) {
	id, ok := parseID(c, "id")
	if !ok {
		return db.Reminder{}, false
	}
	r, err := h.q.GetReminder(c, id)
	if notFound(c, err) {
		return db.Reminder{}, false
	}
	if err != nil {
		fail(c, err)
		return db.Reminder{}, false
	}
	if r.UserID != middleware.CurrentUser(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "not your reminder"})
		return db.Reminder{}, false
	}
	return r, true
}
