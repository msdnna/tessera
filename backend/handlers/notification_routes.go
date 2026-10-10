package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/db"
	"tessera/internal/notify"
	"tessera/middleware"
)

// Alertmanager-style routing rules: CRUD over a user ordered rule list, plus the
// first-match-wins evaluation that turns a fresh notification into outbox rows.

type routeView struct {
	ID         uuid.UUID       `json:"id"`
	Position   float64         `json:"position"`
	Matcher    json.RawMessage `json:"matcher"`
	ChannelIDs []uuid.UUID     `json:"channel_ids"`
	Options    json.RawMessage `json:"options"`
	Enabled    bool            `json:"enabled"`
	CreatedAt  time.Time       `json:"created_at"`
}

func routeViewOf(r db.NotificationRoute) routeView {
	ids := r.ChannelIds
	if ids == nil {
		ids = []uuid.UUID{}
	}
	return routeView{
		ID: r.ID, Position: r.Position, Matcher: orJSONObj(r.Matcher),
		ChannelIDs: ids, Options: orJSONObj(r.Options), Enabled: r.Enabled, CreatedAt: r.CreatedAt,
	}
}

type routeReq struct {
	Position   *float64        `json:"position"`
	Matcher    json.RawMessage `json:"matcher"`
	ChannelIDs []uuid.UUID     `json:"channel_ids"`
	Options    json.RawMessage `json:"options"`
	Enabled    *bool           `json:"enabled"`
}

// parseRule reads a rule's condition and options out of the two JSONB blobs it
// is stored as. Both columns are free-form, so nothing but this parse says
// whether what the user wrote is a rule at all.
//
// It has to be checked, not shrugged off, because the zero value is not a
// neutral one: notify.Matcher documents the empty object as a catch-all, so a
// matcher the server cannot read comes out matching *everything*. The list is
// first-match-wins, so one such rule at the top of it is the only rule the user
// has left. "Silence mentions" written as {"kinds":"mention"} — a string where
// an array belongs — is that mistake in its shortest form, and it swallowed even
// a reminder the user had set as an alarm (#2932). The clients round-trip the
// whole rule and so never produced it; a PAT script writing the one field it
// means to set did.
func parseRule(matcher, options json.RawMessage) (notify.Matcher, notify.RouteOptions, error) {
	var m notify.Matcher
	if err := json.Unmarshal(orJSONObj(matcher), &m); err != nil {
		return m, notify.RouteOptions{}, fmt.Errorf("matcher: %w", err)
	}
	var o notify.RouteOptions
	if err := json.Unmarshal(orJSONObj(options), &o); err != nil {
		return m, o, fmt.Errorf("options: %w", err)
	}
	return m, o, nil
}

// ruleReadable refuses a rule whose matcher or options do not parse. Answering
// 400 is the honest reply: the rule as written has no meaning the router could
// carry out, and storing it would silently widen it to every notification.
func ruleReadable(c *gin.Context, matcher, options json.RawMessage) bool {
	if _, _, err := parseRule(matcher, options); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "правило не разобрать: " + err.Error()})
		return false
	}
	return true
}

// ListNotificationRoutes returns the current user's routing rules, ordered.
func (h *API) ListNotificationRoutes(c *gin.Context) {
	rows, err := h.q.ListNotificationRoutes(c, middleware.CurrentUser(c))
	if err != nil {
		fail(c, err)
		return
	}
	out := make([]routeView, 0, len(rows))
	for _, r := range rows {
		out = append(out, routeViewOf(r))
	}
	c.JSON(http.StatusOK, out)
}

// CreateNotificationRoute adds a routing rule (appended to the end by default).
func (h *API) CreateNotificationRoute(c *gin.Context) {
	uid := middleware.CurrentUser(c)
	var req routeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !h.channelsOwned(c, uid, req.ChannelIDs) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "одного из каналов не существует"})
		return
	}
	if !ruleReadable(c, req.Matcher, req.Options) {
		return
	}
	pos := req.Position
	if pos == nil {
		// Append to the end of the user's rule list.
		existing, _ := h.q.ListNotificationRoutes(c, uid)
		var maxPos float64
		for _, r := range existing {
			if r.Position > maxPos {
				maxPos = r.Position
			}
		}
		p := positionBetween(&maxPos, nil)
		pos = &p
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	row, err := h.q.CreateNotificationRoute(c, db.CreateNotificationRouteParams{
		UserID: uid, Position: *pos, Matcher: orJSONObj(req.Matcher),
		ChannelIds: orIDs(req.ChannelIDs), Options: orJSONObj(req.Options), Enabled: enabled,
	})
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, routeViewOf(row))
}

// UpdateNotificationRoute edits a routing rule.
//
// A field the request left out keeps its stored value — this is a PATCH, and
// here the defaults were not merely untidy: the list is first-match-wins, so a
// missing position fell to 0 and pulled the rule ahead of every other one, and
// notify.Matcher documents the empty object as a catch-all, so a missing matcher
// widened that promoted rule to every notification the user gets. Both shipped
// clients echo the whole rule back and so never saw it; a PAT script sending the
// one field it means to change did.
//
// An explicitly empty value still replaces: `"channel_ids": []` is what a mute
// rule stores and `"matcher": {}` is a deliberate catch-all, and JSON absence is
// distinguishable from either.
func (h *API) UpdateNotificationRoute(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	uid := middleware.CurrentUser(c)
	var req routeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// The stored rule supplies whatever the request did not name. Reading the
	// user's list also scopes the lookup: a rule that is not theirs is simply not
	// in it, and the UPDATE below is scoped again anyway.
	stored, ok := h.routeOf(c, uid, id)
	if !ok {
		return
	}
	arg := db.UpdateNotificationRouteParams{
		ID: id, UserID: uid, Position: stored.Position, Matcher: orJSONObj(stored.Matcher),
		ChannelIds: orIDs(stored.ChannelIds), Options: orJSONObj(stored.Options), Enabled: stored.Enabled,
	}
	if req.Position != nil {
		arg.Position = *req.Position
	}
	if req.Matcher != nil {
		arg.Matcher = orJSONObj(req.Matcher)
	}
	if req.ChannelIDs != nil {
		arg.ChannelIds = req.ChannelIDs
	}
	if req.Options != nil {
		arg.Options = orJSONObj(req.Options)
	}
	if req.Enabled != nil {
		arg.Enabled = *req.Enabled
	}
	if !h.channelsOwned(c, uid, arg.ChannelIds) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "одного из каналов не существует"})
		return
	}
	if !ruleReadable(c, arg.Matcher, arg.Options) {
		return
	}
	row, err := h.q.UpdateNotificationRoute(c, arg)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, routeViewOf(row))
}

// routeOf returns one of the user's routing rules. There is no single-row query
// for it: the list is per-user and short (it is a hand-written rule list, not a
// feed), and reusing it keeps the ownership check in one place.
func (h *API) routeOf(c *gin.Context, uid, id uuid.UUID) (db.NotificationRoute, bool) {
	rows, err := h.q.ListNotificationRoutes(c, uid)
	if err != nil {
		fail(c, err)
		return db.NotificationRoute{}, false
	}
	for _, r := range rows {
		if r.ID == id {
			return r, true
		}
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
	return db.NotificationRoute{}, false
}

// DeleteNotificationRoute removes a routing rule.
func (h *API) DeleteNotificationRoute(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.q.DeleteNotificationRoute(c, db.DeleteNotificationRouteParams{
		ID: id, UserID: middleware.CurrentUser(c),
	}); err != nil {
		fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// routeNotification evaluates a freshly created notification against the user's
// routing rules and enqueues an outbox delivery per channel of the first matching
// enabled rule (first-match-wins; a matching rule with mute=true drops it). The
// in-app notification has already been created/broadcast by notify() — this only
// gates external channels. Best-effort: a routing failure never breaks the
// originating mutation.
func (h *API) routeNotification(ctx context.Context, n db.Notification) []string {
	routes, err := h.q.ListNotificationRoutes(ctx, n.UserID)
	if err != nil || len(routes) == 0 {
		return nil
	}
	chans, err := h.q.ListNotificationChannels(ctx, n.UserID)
	if err != nil {
		return nil
	}
	chByID := make(map[uuid.UUID]db.NotificationChannel, len(chans))
	for _, ch := range chans {
		chByID[ch.ID] = ch
	}
	// A reminder tied to no task belongs to no workspace. Reminders are personal,
	// and the workspace id on the row is carriage rather than scope:
	// reminderScope stamps the user's first workspace so the notification is a
	// valid row and reaches the socket, and it hands back a task id only when that
	// workspace is genuinely the reminder's. Read as scope, the stamp lets a rule
	// about a workspace claim an alarm clock that has nothing to do with it —
	// "не беспокоить меня ничем из рабочего пространства" silences the user's own
	// reminders, and the non-mute form sends them to the channel picked for work
	// while the rule below never sees them. Usually the stamp lands on the personal
	// workspace every account is created with, which hides this; a user who works
	// only in a shared one and deletes the personal one moves the stamp there
	// (#2932). Muting reminders as such is a matcher on the kind, which still
	// applies — that is the knob for it.
	ev := notify.Event{Kind: n.Kind}
	if n.WorkspaceID != nil {
		ev.WorkspaceID = *n.WorkspaceID
	}
	if n.Kind == evReminder && n.TaskID == nil {
		ev.WorkspaceID = uuid.Nil
	}
	for _, r := range routes {
		if !r.Enabled {
			continue
		}
		// A rule the server cannot read is not applied. Skipping is what the
		// refusal at the door cannot do for a row already in the table: the
		// alternative is the zero matcher, which is a catch-all, so a rule that
		// named one kind would claim every notification the user gets and the
		// rules below it would never be reached (#2932).
		m, opts, perr := parseRule(r.Matcher, r.Options)
		if perr != nil {
			log.Printf("notify: routing rule %s is unreadable, skipped: %v", r.ID, perr)
			continue
		}
		if !m.Matches(ev) {
			continue
		}
		if opts.Mute {
			return nil // matched a mute rule — deliver nowhere
		}
		// A rule with nothing left to deliver through does not get to consume the
		// notification. Silence is what mute is for, and both clients write it as
		// an explicit options.mute (the empty channel list merely comes with it),
		// so an empty list on its own is never a user asking for quiet. It is what
		// the server itself leaves behind: deleting a channel strips its id out of
		// every rule that named it (StripChannelFromRoutes), and a rule that named
		// only that one is left matching first with nowhere to go. Consumed there,
		// the list being first-match-wins, the catch-all below never sees the
		// notification: dropping the telegram channel stopped mentions from
		// reaching e-mail as well, while the settings screen showed a rule with no
		// channels and no "muted" badge, which says nothing at all (#2932).
		//
		// Ids that resolve to nothing count the same way: rules written before the
		// strip existed still carry the id of a channel that is gone, which the
		// loop below skips one by one and so also delivers nowhere, the same
		// blackhole by the other road. A channel that exists but is switched off
		// is not this case: turning one off is a deliberate pause, and rerouting
		// its rule's traffic to whatever sits underneath would be a destination
		// the user never picked.
		live := 0
		for _, chID := range r.ChannelIds {
			if _, ok := chByID[chID]; ok {
				live++
			}
		}
		if live == 0 {
			continue
		}
		// Resolve the user's prefs once for both quiet-hours deferral and digest
		// batching. Quiet hours hold external delivery until the window ends; a
		// digest window holds it long enough to combine a burst (whichever is later
		// wins, so a quiet-release burst still gets combined).
		p, perr := h.q.GetNotificationPrefs(ctx, n.UserID)
		if perr != nil {
			p = defaultPrefs(n.UserID)
		}
		now := time.Now()
		// Asked about an arbitrary instant, not just now: a digest slot is a future
		// time, and whether *it* falls inside the window is what decides when the
		// digest may go out (see digestSlot).
		quietAt := quietFor(p, quietExempt(p, n.Kind, h.taskPriorityOf(ctx, p, n.TaskID)))
		_, inQuiet := quietAt(now)
		var deviceTargets []string
		for _, chID := range r.ChannelIds {
			ch, ok := chByID[chID]
			if !ok || !ch.Enabled {
				continue
			}
			// "device" channels flag the live WS event so an open client shows a
			// native notification. Ephemeral, so quiet hours suppress them (can't
			// be deferred). A device that also registered a push token gets an
			// outbox row on top, which is what reaches it while the app is closed.
			if ch.Type == "device" {
				if !inQuiet {
					if did := deviceIDOf(ch); did != "" {
						deviceTargets = append(deviceTargets, did)
					}
					if _, hasPush := h.senders["device"]; hasPush && channelCfgString(ch, "fcm_token") != "" {
						// No digest group and no deferral: a push that arrives batched
						// half an hour late is worse than no push at all.
						soft(ctx, "CreateNotificationDeliveryAt", h.q.CreateNotificationDeliveryAt(ctx, db.CreateNotificationDeliveryAtParams{
							NotificationID: n.ID, ChannelID: chID, NextAttemptAt: now, DigestGroup: "",
						}))
					}
				}
				continue
			}
			next, group := digestSlot(now, quietAt, digestMinutesFor(n.Kind, p.DigestMinutes), chID)
			soft(ctx, "CreateNotificationDeliveryAt", h.q.CreateNotificationDeliveryAt(ctx, db.CreateNotificationDeliveryAtParams{
				NotificationID: n.ID, ChannelID: chID, NextAttemptAt: next, DigestGroup: group,
			}))
		}
		return deviceTargets // first matching rule wins
	}
	return nil
}

// urgentPriority is the task priority «Срочный» (the top of the 0–4 scale).
const urgentPriority = 4

// quietExempt reports whether a notification may go out inside the quiet window
// anyway — «Только срочное и упоминания» (#3050). Only when the user asked for
// it: a mention of them (in a task, a document or a meeting's chat), a meeting
// about to start, and anything about a task marked urgent. The rest waits for
// the window to end, as before.
func quietExempt(p db.NotificationPref, kind string, taskPriority int32) bool {
	if !p.QuietAllowUrgent {
		return false
	}
	if kind == kindMention || kind == kindConferenceSoon {
		return true
	}
	return taskPriority >= urgentPriority
}

// quietFor binds the quiet window to one notification: the user's window, or
// none at all when the notification is exempt from it.
func quietFor(p db.NotificationPref, exempt bool) quietFn {
	return func(t time.Time) (time.Time, bool) {
		if exempt {
			return time.Time{}, false
		}
		return quietWindow(p.QuietEnabled, int(p.QuietStartMinutes), int(p.QuietEndMinutes), p.QuietTz, t)
	}
}

// taskPriorityOf reads the priority of the task a notification is about, 0 when
// it is about none (or the task is gone). Asked only when an exception could
// hang on it.
func (h *API) taskPriorityOf(ctx context.Context, p db.NotificationPref, taskID *uuid.UUID) int32 {
	if !p.QuietAllowUrgent || taskID == nil {
		return 0
	}
	t, err := h.q.GetTask(ctx, *taskID)
	if err != nil {
		return 0
	}
	return t.Priority
}

// timedKinds are the notifications that name an instant the reader is already
// waiting for, rather than reporting that something happened. A reminder is an
// alarm set for a minute the user chose; a due-date warning is the same minute
// counted backwards from the deadline ("warn me 15 minutes before"); a
// conference invitation may point at a call that is live right now.
var timedKinds = map[string]bool{
	kindReminder:         true,
	kindDueSoon:          true,
	kindConferenceInvite: true,
	kindConferenceSoon:   true,
}

// digestMinutesFor is the digest window that applies to one notification: the
// user's, except for the kinds that carry their own deadline.
//
// A digest slot is always in the future — up to the whole window — and the point
// of it is to trade a little lateness for one message instead of ten. That trade
// is only the user's to make on a stream of activity. On a notification that
// names a moment, the batching silently overrides the timing the user set
// elsewhere: an hourly digest turns a reminder set for 14:00 into one that rings
// at 14:59, delivers "warn me 15 minutes before the deadline" after the deadline
// (a lead shorter than the window cannot survive it), and carries an invitation
// to a call that ended while the window filled. The same argument is already
// made a few lines up for device pushes, which take neither a digest group nor a
// deferral — "a push that arrives batched half an hour late is worse than no
// push at all" (#2932). It holds for e-mail and telegram too.
//
// Quiet hours are untouched: those are silence the user asked for by the clock,
// and a reminder may legitimately wait for the morning. This is only about the
// window whose whole purpose is convenience.
func digestMinutesFor(kind string, digestMinutes int32) int32 {
	if timedKinds[kind] {
		return 0
	}
	return digestMinutes
}

// maxDigestMinutes bounds the digest window. The clients offer 5/15/30/60; a day
// is left reachable so "one digest every morning" stays expressible, and anything
// past it is refused at the door (UpdateMyNotificationPrefs) and clamped here.
//
// The bound is not tidiness. digest_minutes is an int32 that used to be stored
// with only max(0, …) in front of it, so a value no client offers went straight
// into the arithmetic below:
//   - 100000 puts the next slot seven weeks out and 1e9 in the year 2215. That
//     is not batching, it is a notification swallowed whole, and nothing in the
//     outbox reports it as anything but pending.
//   - from 153722868 up, digestMinutes * time.Minute overflows int64 and the
//     window comes out NEGATIVE. Truncate is documented as a no-op for a
//     non-positive duration, so the Add then runs backwards: int32 max lands the
//     slot in 2017. A row that came due in the past is claimed on the very next
//     tick — which throws away the quiet-hours anchor this function had just set
//     and sends at 23:00 the message the window deferred to 08:00 (#2932).
//
// Clamped rather than only refused, because the refusal cannot reach a prefs row
// already holding such a value: the screens render it as a window and the user
// has nothing to correct.
const maxDigestMinutes = 1440

// digestSlot decides when an outbox row for chID may be sent and which digest
// group it joins. quietAt reports the quiet window covering an instant (the end
// of it, and whether the instant is inside at all); digestMinutes <= 0 means the
// user wants no digest (see digestMinutesFor for the kinds that always mean it),
// and anything above maxDigestMinutes is clamped to it.
//
// The digest window is a slot on a fixed grid, not a per-notification timer. Only
// rows that one drain tick claims together are ever combined (drainDeliveries
// groups what ClaimPendingDeliveries returned), so rows of the same window have
// to come due at the same instant — a per-notification now+window gives each its
// own deadline seconds apart, and a burst arrives as the stream of separate
// messages the window exists to replace (#2932). The slot end is also in the
// group key: a spent window must not absorb the next one's rows, nor a retried
// row from an hour ago.
//
// Quiet hours move the anchor, not the grid: a window that would release a
// delivery mid-quiet is pushed to the first slot after the window ends, where the
// whole released burst meets.
func digestSlot(now time.Time, quietAt quietFn, digestMinutes int32, chID uuid.UUID) (time.Time, string) {
	anchor := now
	if qEnd, inQuiet := quietAt(now); inQuiet {
		anchor = qEnd
	}
	if digestMinutes <= 0 {
		return anchor, ""
	}
	if digestMinutes > maxDigestMinutes {
		digestMinutes = maxDigestMinutes
	}
	// The configured windows (5/15/30/60 min) all divide an hour, so truncating on
	// the epoch grid lands on wall-clock boundaries. Always the *next* boundary:
	// an anchor sitting exactly on one still owes its window a chance to fill.
	window := time.Duration(digestMinutes) * time.Minute
	end := anchor.Truncate(window).Add(window)
	// The slot the burst waits for can itself fall inside quiet hours even though
	// none of its notifications did — a mention at 21:40 with an hourly digest and
	// a window opening at 22:00 comes due at exactly 22:00. Without the digest that
	// same mention would have been e-mailed at 21:40, while it was still allowed;
	// the batching then turns an allowed delivery into a nightly one, which is the
	// single thing quiet hours are set for. So the slot is checked too, not just
	// the arrival, and a quiet one is re-anchored to the window's end.
	if qEnd, inQuiet := quietAt(end); inQuiet {
		end = qEnd.Truncate(window).Add(window)
		// A quiet range wider than the grid leaves no free slot to move to (quiet
		// 00:10–00:00 has one non-quiet instant in the whole day). Fall back to the
		// anchor, which is a permitted instant by construction — either an arrival
		// outside the window, or the moment one ends. That costs the batching in a
		// configuration where no two rows could have shared a slot anyway; being
		// late at night is the thing worth avoiding, not being uncombined.
		if _, stillQuiet := quietAt(end); stillQuiet {
			end = anchor
		}
	}
	return end, fmt.Sprintf("%s:%d", chID, end.Unix())
}

// reanchorDeliveries re-applies a freshly saved schedule to the outbox rows that
// are still waiting on the old one.
//
// digestSlot runs once, at enqueue time, and writes its answer into
// next_attempt_at as an absolute instant. The prefs behind that answer then keep
// changing, and nothing used to carry the change across — so the setting lied in
// both directions at once. Turning quiet hours off at 23:30 ("I'm still up, send
// them") answered 200 and left every held row waiting for 08:00: a queue frozen
// by a setting that no longer exists, with the switch on screen saying otherwise.
// Widening the window to 10:00 was worse: the rows deferred to 08:00 still fired
// at 08:00, inside the silence just declared, out of the one feature whose whole
// purpose is that the night stays quiet.
//
// The recomputation is exact rather than approximate: created_at is the arrival
// digestSlot was given, so feeding it back with the new prefs reproduces the
// decision that would have been made had the row arrived now — including the
// digest group, so rows that land on the same new slot still combine into one
// message instead of scattering. Rows that already went out for a send keep their
// own timing; see ListReschedulableDeliveries (#2932).
//
// Best-effort: the prefs are saved either way, and a failure here leaves the
// queue on its old schedule rather than the save on an error.
func (h *API) reanchorDeliveries(ctx context.Context, uid uuid.UUID, p db.NotificationPref) {
	rows, err := h.q.ListReschedulableDeliveries(ctx, uid)
	if err != nil {
		return
	}
	for _, d := range rows {
		quietAt := quietFor(p, quietExempt(p, d.Kind, d.TaskPriority))
		next, group := digestSlot(d.CreatedAt, quietAt, digestMinutesFor(d.Kind, p.DigestMinutes), d.ChannelID)
		soft(ctx, "RescheduleDelivery", h.q.RescheduleDelivery(ctx, db.RescheduleDeliveryParams{
			ID: d.ID, NextAttemptAt: next, DigestGroup: group,
		}))
	}
}

// scheduleChanged reports whether a prefs save touched anything digestSlot reads.
// Saving the same window back must not reshuffle the outbox: the recomputation is
// anchored on created_at while the stored value came from time.Now() a moment
// later, so an untouched row can still shift across a grid boundary.
func scheduleChanged(before, after db.NotificationPref) bool {
	return before.QuietEnabled != after.QuietEnabled ||
		before.QuietStartMinutes != after.QuietStartMinutes ||
		before.QuietEndMinutes != after.QuietEndMinutes ||
		before.QuietTz != after.QuietTz ||
		before.QuietAllowUrgent != after.QuietAllowUrgent ||
		before.DigestMinutes != after.DigestMinutes
}

// quietFn reports the quiet window covering an instant: when it ends, and whether
// the instant is inside one at all. Bound to a user's prefs by routeNotification;
// quietWindow is the implementation.
type quietFn func(time.Time) (time.Time, bool)

// deviceIDOf extracts the stable device id from a device channel's config.
func deviceIDOf(ch db.NotificationChannel) string { return channelCfgString(ch, "device_id") }

// channelCfgString reads a string field out of a stored channel's JSONB config.
func channelCfgString(ch db.NotificationChannel, key string) string {
	var m map[string]any
	if json.Unmarshal(ch.Config, &m) == nil {
		if s, ok := m[key].(string); ok {
			return s
		}
	}
	return ""
}
