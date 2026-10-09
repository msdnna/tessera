package handlers

import (
	"bytes"
	"encoding/json"
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
	// In-app toasts (#3117): the «Показывать» switch, what to show (an object
	// only the client reads), «И всплывающие в приложении» of the quiet hours
	// and «Не беспокоить» — the last one is set through PUT
	// /notification-prefs/dnd.
	ToastsEnabled bool            `json:"toasts_enabled"`
	Toasts        json.RawMessage `json:"toasts"`
	QuietToasts   bool            `json:"quiet_toasts"`
	DndUntil      *time.Time      `json:"dnd_until"`
}

// prefsUpdate is the PUT body. The scheduling fields are full-replace as they
// always were; the toast fields came later (#3117) and are pointers, because
// the Android client PUTs the prefs it knows about — an absent field must keep
// what the web client stored, not reset it to false.
type prefsUpdate struct {
	DueEnabled        bool            `json:"due_enabled"`
	DueLeadMinutes    int32           `json:"due_lead_minutes"`
	DueRepeatMinutes  int32           `json:"due_repeat_minutes"`
	ReminderEnabled   bool            `json:"reminder_enabled"`
	QuietEnabled      bool            `json:"quiet_enabled"`
	QuietStartMinutes int32           `json:"quiet_start_minutes"`
	QuietEndMinutes   int32           `json:"quiet_end_minutes"`
	QuietTz           string          `json:"quiet_tz"`
	DigestMinutes     int32           `json:"digest_minutes"`
	ToastsEnabled     *bool           `json:"toasts_enabled"`
	Toasts            json.RawMessage `json:"toasts"`
	QuietToasts       *bool           `json:"quiet_toasts"`
}

// maxToastsConfig bounds the client's toast object: a handful of switches, not
// a place to park data.
const maxToastsConfig = 4096

func prefsViewOf(p db.NotificationPref) prefsView {
	toasts := json.RawMessage(p.Toasts)
	if len(toasts) == 0 {
		toasts = json.RawMessage(`{}`)
	}
	// A moment already behind us is "off" for every reader; the row keeps it
	// until the next write rather than being swept.
	dnd := p.DndUntil
	if dnd != nil && !dnd.After(time.Now()) {
		dnd = nil
	}
	return prefsView{
		DueEnabled: p.DueEnabled, DueLeadMinutes: p.DueLeadMinutes,
		DueRepeatMinutes: p.DueRepeatMinutes, ReminderEnabled: p.ReminderEnabled,
		QuietEnabled: p.QuietEnabled, QuietStartMinutes: p.QuietStartMinutes,
		QuietEndMinutes: p.QuietEndMinutes, QuietTz: p.QuietTz, DigestMinutes: p.DigestMinutes,
		ToastsEnabled: p.ToastsEnabled, Toasts: toasts, QuietToasts: p.QuietToasts, DndUntil: dnd,
	}
}

// toastsConfig validates the client's toast object: absent (or null) keeps the
// stored one, anything else must be a JSON object of a sane size.
func toastsConfig(raw json.RawMessage, keep []byte) ([]byte, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		if len(keep) == 0 {
			return []byte(`{}`), true
		}
		return keep, true
	}
	if len(trimmed) > maxToastsConfig || trimmed[0] != '{' {
		return nil, false
	}
	var obj map[string]any
	if err := json.Unmarshal(trimmed, &obj); err != nil {
		return nil, false
	}
	return trimmed, true
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
	var req prefsUpdate
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
	// Read before write: the outbox rows already deferred by the old window have
	// to be re-anchored to the new one, and only a real change may do it (see
	// reanchorDeliveries / scheduleChanged).
	uid := middleware.CurrentUser(c)
	before, berr := h.q.GetNotificationPrefs(c, uid)
	if berr != nil {
		before = defaultPrefs(uid)
	}
	toasts, ok := toastsConfig(req.Toasts, before.Toasts)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "настройки всплывающих — объект JSON не больше 4 КБ"})
		return
	}
	toastsOn := before.ToastsEnabled
	if req.ToastsEnabled != nil {
		toastsOn = *req.ToastsEnabled
	}
	quietToasts := before.QuietToasts
	if req.QuietToasts != nil {
		quietToasts = *req.QuietToasts
	}
	p, err := h.q.UpsertNotificationPrefs(c, db.UpsertNotificationPrefsParams{
		UserID: uid, DueEnabled: req.DueEnabled,
		DueLeadMinutes: req.DueLeadMinutes, DueRepeatMinutes: req.DueRepeatMinutes,
		ReminderEnabled:   req.ReminderEnabled,
		QuietEnabled:      req.QuietEnabled,
		QuietStartMinutes: clampMinuteOfDay(req.QuietStartMinutes),
		QuietEndMinutes:   clampMinuteOfDay(req.QuietEndMinutes),
		QuietTz:           tz,
		DigestMinutes:     max(0, req.DigestMinutes),
		ToastsEnabled:     toastsOn,
		Toasts:            toasts,
		QuietToasts:       quietToasts,
	})
	if err != nil {
		fail(c, err)
		return
	}
	if scheduleChanged(before, p) {
		h.reanchorDeliveries(c, uid, p)
	}
	c.JSON(http.StatusOK, prefsViewOf(p))
}

// maxDnd bounds «Не беспокоить»: the menu offers an hour and «until tomorrow
// 9:00»; a week covers any client-side preset without letting a typo silence
// the toasts for good.
const maxDnd = 7 * 24 * time.Hour

// SetMyNotificationDnd switches «Не беспокоить» (#3117): `{"until": <RFC 3339>}`
// silences the in-app toasts until then, `{"until": null}` turns it off. Its
// own door, because the feed's moon sets it without holding the rest of the
// prefs, and PUT /notification-prefs is a full replace of those.
func (h *API) SetMyNotificationDnd(c *gin.Context) {
	var req struct {
		Until *time.Time `json:"until"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Until != nil {
		now := time.Now()
		if !req.Until.After(now) {
			req.Until = nil
		} else if req.Until.Sub(now) > maxDnd {
			c.JSON(http.StatusBadRequest, gin.H{"error": "«Не беспокоить» включается не больше чем на неделю"})
			return
		}
	}
	p, err := h.q.SetNotificationDnd(c, db.SetNotificationDndParams{
		UserID: middleware.CurrentUser(c), DndUntil: req.Until,
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
