package handlers

import (
	"context"
	"encoding/json"
	"fmt"
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
	ev := notify.Event{Kind: n.Kind, WorkspaceID: n.WorkspaceID}
	for _, r := range routes {
		if !r.Enabled {
			continue
		}
		var m notify.Matcher
		_ = json.Unmarshal(orJSONObj(r.Matcher), &m)
		if !m.Matches(ev) {
			continue
		}
		var opts notify.RouteOptions
		_ = json.Unmarshal(orJSONObj(r.Options), &opts)
		if opts.Mute {
			return nil // matched a mute rule — deliver nowhere
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
		quietAt := func(t time.Time) (time.Time, bool) {
			return quietWindow(p.QuietEnabled, int(p.QuietStartMinutes), int(p.QuietEndMinutes), p.QuietTz, t)
		}
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

// timedKinds are the notifications that name an instant the reader is already
// waiting for, rather than reporting that something happened. A reminder is an
// alarm set for a minute the user chose; a due-date warning is the same minute
// counted backwards from the deadline ("warn me 15 minutes before"); a
// conference invitation may point at a call that is live right now.
var timedKinds = map[string]bool{
	"reminder":           true,
	"due_soon":           true,
	kindConferenceInvite: true,
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

// digestSlot decides when an outbox row for chID may be sent and which digest
// group it joins. quietAt reports the quiet window covering an instant (the end
// of it, and whether the instant is inside at all); digestMinutes <= 0 means the
// user wants no digest (see digestMinutesFor for the kinds that always mean it).
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
