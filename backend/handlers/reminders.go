package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/db"
	"tessera/middleware"
)

// Reminders are personal (scoped to the current user), so authorization is a
// plain owner check rather than workspace membership.

// CreateReminder schedules a personal reminder, optionally linked to a task.
func (h *API) CreateReminder(c *gin.Context) {
	var req struct {
		TaskID   *uuid.UUID `json:"task_id"`
		RemindAt time.Time  `json:"remind_at" binding:"required"`
		Message  string     `json:"message"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	r, err := h.q.CreateReminder(c, db.CreateReminderParams{
		UserID:   middleware.CurrentUser(c),
		TaskID:   req.TaskID,
		RemindAt: req.RemindAt,
		Message:  req.Message,
	})
	if err != nil {
		fail(c, err)
		return
	}
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

// UpdateReminder edits time/message or marks it done.
func (h *API) UpdateReminder(c *gin.Context) {
	r, ok := h.loadReminder(c)
	if !ok {
		return
	}
	var req struct {
		RemindAt time.Time `json:"remind_at" binding:"required"`
		Message  string    `json:"message"`
		Done     bool      `json:"done"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	updated, err := h.q.UpdateReminder(c, db.UpdateReminderParams{
		ID: r.ID, RemindAt: req.RemindAt, Message: req.Message, Done: req.Done,
	})
	if err != nil {
		fail(c, err)
		return
	}
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
	c.Status(http.StatusNoContent)
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
