package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"tessera/internal/db"
	"tessera/middleware"
)

// Per-user scheduling preferences for the due/reminder scanner (lead, repeat, quiet
// hours) and the per-task due-notify opt-out.

type prefsView struct {
	DueEnabled        bool   `json:"due_enabled"`
	DueLeadMinutes    int32  `json:"due_lead_minutes"`
	DueRepeatMinutes  int32  `json:"due_repeat_minutes"`
	ReminderEnabled   bool   `json:"reminder_enabled"`
	QuietEnabled      bool   `json:"quiet_enabled"`
	QuietStartMinutes int32  `json:"quiet_start_minutes"`
	QuietEndMinutes   int32  `json:"quiet_end_minutes"`
	QuietTz           string `json:"quiet_tz"`
	DigestMinutes     int32  `json:"digest_minutes"`
}

func prefsViewOf(p db.NotificationPref) prefsView {
	return prefsView{
		DueEnabled: p.DueEnabled, DueLeadMinutes: p.DueLeadMinutes,
		DueRepeatMinutes: p.DueRepeatMinutes, ReminderEnabled: p.ReminderEnabled,
		QuietEnabled: p.QuietEnabled, QuietStartMinutes: p.QuietStartMinutes,
		QuietEndMinutes: p.QuietEndMinutes, QuietTz: p.QuietTz, DigestMinutes: p.DigestMinutes,
	}
}

// clampMinuteOfDay keeps a minutes-since-midnight value in [0, 1439].
func clampMinuteOfDay(m int32) int32 {
	if m < 0 {
		return 0
	}
	if m > 1439 {
		return 1439
	}
	return m
}

// GetMyNotificationPrefs returns the current user's scheduling prefs (defaults when
// never customised).
func (h *API) GetMyNotificationPrefs(c *gin.Context) {
	uid := middleware.CurrentUser(c)
	p, err := h.q.GetNotificationPrefs(c, uid)
	if errors.Is(err, pgx.ErrNoRows) {
		p = defaultPrefs(uid)
	} else if err != nil {
		fail(c)
		return
	}
	c.JSON(http.StatusOK, prefsViewOf(p))
}

// UpdateMyNotificationPrefs upserts the current user's scheduling prefs.
func (h *API) UpdateMyNotificationPrefs(c *gin.Context) {
	var req prefsView
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.DueLeadMinutes < 0 {
		req.DueLeadMinutes = 0
	}
	if req.DueRepeatMinutes < 0 {
		req.DueRepeatMinutes = 0
	}
	p, err := h.q.UpsertNotificationPrefs(c, db.UpsertNotificationPrefsParams{
		UserID: middleware.CurrentUser(c), DueEnabled: req.DueEnabled,
		DueLeadMinutes: req.DueLeadMinutes, DueRepeatMinutes: req.DueRepeatMinutes,
		ReminderEnabled:   req.ReminderEnabled,
		QuietEnabled:      req.QuietEnabled,
		QuietStartMinutes: clampMinuteOfDay(req.QuietStartMinutes),
		QuietEndMinutes:   clampMinuteOfDay(req.QuietEndMinutes),
		QuietTz:           strings.TrimSpace(req.QuietTz),
		DigestMinutes:     max(0, req.DigestMinutes),
	})
	if err != nil {
		fail(c)
		return
	}
	c.JSON(http.StatusOK, prefsViewOf(p))
}

// SetTaskDueNotify sets a task's per-task due-notification overrides (each field
// null = inherit the user default). Driven by the card's due popover.
func (h *API) SetTaskDueNotify(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if _, _, ok := h.loadTask(c, id); !ok {
		return
	}
	var req struct {
		LeadMinutes   *int32 `json:"lead_minutes"`
		RepeatMinutes *int32 `json:"repeat_minutes"`
		Enabled       *bool  `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.LeadMinutes != nil && *req.LeadMinutes < 0 {
		*req.LeadMinutes = 0
	}
	if req.RepeatMinutes != nil && *req.RepeatMinutes < 0 {
		*req.RepeatMinutes = 0
	}
	t, err := h.q.SetTaskDueNotify(c, db.SetTaskDueNotifyParams{
		ID: id, DueLeadMinutes: req.LeadMinutes, DueRepeatMinutes: req.RepeatMinutes, DueNotifyEnabled: req.Enabled,
	})
	if err != nil {
		fail(c)
		return
	}
	c.JSON(http.StatusOK, t)
}
