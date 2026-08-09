package handlers

import (
	"context"
	"encoding/json"
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
		fail(c)
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
		fail(c)
		return
	}
	c.JSON(http.StatusCreated, routeViewOf(row))
}

// UpdateNotificationRoute edits a routing rule.
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
	if !h.channelsOwned(c, uid, req.ChannelIDs) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "одного из каналов не существует"})
		return
	}
	pos := 0.0
	if req.Position != nil {
		pos = *req.Position
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	row, err := h.q.UpdateNotificationRoute(c, db.UpdateNotificationRouteParams{
		ID: id, UserID: uid, Position: pos, Matcher: orJSONObj(req.Matcher),
		ChannelIds: orIDs(req.ChannelIDs), Options: orJSONObj(req.Options), Enabled: enabled,
	})
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c)
		return
	}
	c.JSON(http.StatusOK, routeViewOf(row))
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
		fail(c)
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
		base := now
		qEnd, inQuiet := quietWindow(p.QuietEnabled, int(p.QuietStartMinutes), int(p.QuietEndMinutes), p.QuietTz, now)
		if inQuiet {
			base = qEnd
		}
		var deviceTargets []string
		for _, chID := range r.ChannelIds {
			ch, ok := chByID[chID]
			if !ok || !ch.Enabled {
				continue
			}
			// "device" channels aren't sent to an external service — they flag the
			// live WS event so the matching client shows a native notification.
			// Ephemeral, so quiet hours suppress them (can't be deferred).
			if ch.Type == "device" {
				if !inQuiet {
					if did := deviceIDOf(ch); did != "" {
						deviceTargets = append(deviceTargets, did)
					}
				}
				continue
			}
			next, group := base, ""
			if p.DigestMinutes > 0 {
				group = chID.String() // v1 group key — combine everything to this channel
				if d := now.Add(time.Duration(p.DigestMinutes) * time.Minute); d.After(next) {
					next = d
				}
			}
			_ = h.q.CreateNotificationDeliveryAt(ctx, db.CreateNotificationDeliveryAtParams{
				NotificationID: n.ID, ChannelID: chID, NextAttemptAt: next, DigestGroup: group,
			})
		}
		return deviceTargets // first matching rule wins
	}
	return nil
}

// deviceIDOf extracts the stable device id from a device channel's config.
func deviceIDOf(ch db.NotificationChannel) string {
	var m map[string]any
	if json.Unmarshal(ch.Config, &m) == nil {
		if s, ok := m["device_id"].(string); ok {
			return s
		}
	}
	return ""
}
