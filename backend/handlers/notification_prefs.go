package handlers

import (
	"errors"
	"net/http"
	"strings"
	"time"

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
		fail(c, err)
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
	// Только кламп снизу здесь и стоял, и всё, что выше, уходило в
	// `time.Duration(x) * time.Minute` — переполнение int64 переворачивает знак и
	// превращает «раз в 4084 года» в «каждую минуту», а «за 4084 года до срока» —
	// в «через трое суток после». См. maxDueMinutes.
	if req.DueLeadMinutes > maxDueMinutes || req.DueRepeatMinutes > maxDueMinutes {
		c.JSON(http.StatusBadRequest, gin.H{"error": "срок предупреждения и интервал повтора не могут быть больше года"})
		return
	}
	// The window is read in this zone, and quietWindow resolves it with
	// time.LoadLocation — a name it cannot resolve leaves UTC in place without a
	// word (pinned by TestQuietWindowUnknownZoneSilentlyMeansUTC). Stored
	// unvalidated it makes the feature lie in a way both clients corroborate: the
	// PUT answers 200 and echoes the string, so the screens print the zone the
	// user picked while the server silences a window shifted by their whole
	// offset. Refused at the door instead — a rejected save is visible, a night
	// spent unsilenced is not. Empty stays legal: it is the documented "evaluate
	// in UTC" value and the default for a user who never customised. Checked even
	// with quiet hours off, so a bad zone cannot lie in wait for the switch.
	tz := strings.TrimSpace(req.QuietTz)
	if tz != "" {
		if _, err := time.LoadLocation(tz); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "неизвестный часовой пояс: " + tz})
			return
		}
	}
	// A window nobody could have meant is refused rather than stored: past a day
	// the batching is no longer a delay the user traded for, and past int32's
	// halfway mark the duration arithmetic in digestSlot overflows and starts
	// scheduling into the past — see maxDigestMinutes.
	if req.DigestMinutes > maxDigestMinutes {
		c.JSON(http.StatusBadRequest, gin.H{"error": "окно дайджеста не может быть больше суток"})
		return
	}
	p, err := h.q.UpsertNotificationPrefs(c, db.UpsertNotificationPrefsParams{
		UserID: middleware.CurrentUser(c), DueEnabled: req.DueEnabled,
		DueLeadMinutes: req.DueLeadMinutes, DueRepeatMinutes: req.DueRepeatMinutes,
		ReminderEnabled:   req.ReminderEnabled,
		QuietEnabled:      req.QuietEnabled,
		QuietStartMinutes: clampMinuteOfDay(req.QuietStartMinutes),
		QuietEndMinutes:   clampMinuteOfDay(req.QuietEndMinutes),
		QuietTz:           tz,
		DigestMinutes:     max(0, req.DigestMinutes),
	})
	if err != nil {
		fail(c, err)
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
	// Вторая дверь к той же арифметике: пер-задачный override побеждает
	// пользовательский дефолт, так что граница здесь та же (см. maxDueMinutes).
	if (req.LeadMinutes != nil && *req.LeadMinutes > maxDueMinutes) ||
		(req.RepeatMinutes != nil && *req.RepeatMinutes > maxDueMinutes) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "срок предупреждения и интервал повтора не могут быть больше года"})
		return
	}
	t, err := h.q.SetTaskDueNotify(c, db.SetTaskDueNotifyParams{
		ID: id, DueLeadMinutes: req.LeadMinutes, DueRepeatMinutes: req.RepeatMinutes, DueNotifyEnabled: req.Enabled,
	})
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, t)
}
