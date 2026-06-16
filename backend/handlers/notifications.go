package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"tessera/internal/db"
	"tessera/internal/mail"
	"tessera/internal/notify"
	"tessera/middleware"
)

// This file implements the user-configurable notification router (Phase A):
// per-user delivery channels (email / telegram / webhook), Alertmanager-style
// routing rules, and the background outbox worker that performs the sends. The
// pure rule engine + channel transports live in internal/notify; the DB-bound
// dispatch, secret sealing and worker live here.

// ── senders registry ───────────────────────────────────────

// buildSenders wires the channel transports. Email reuses the server's SMTP
// mailer (the destination is the user's address, the transport is shared), so it
// needs no per-channel secret; telegram + webhook are self-contained.
func buildSenders(mailer mail.Mailer) map[string]notify.Sender {
	return map[string]notify.Sender{
		"email":    emailSender{mailer: mailer},
		"telegram": notify.ShoutrrrSender{}, // friendly telegram, built into a shoutrrr URL
		"webhook":  notify.WebhookSender{},  // flexible raw-JSON POST
		"shoutrrr": notify.ShoutrrrSender{}, // generic: secret is a full shoutrrr URL
	}
}

// requiredSecret maps a channel type to the secret field that must be present
// (set on create, kept on edit). Other types have no mandatory secret.
var requiredSecret = map[string]string{
	"telegram": "bot_token",
	"shoutrrr": "url",
}

// emailSender delivers via the server mailer (no-op-logs when SMTP is unset).
type emailSender struct{ mailer mail.Mailer }

func (s emailSender) Send(_ context.Context, ch notify.Channel, msg notify.Message) error {
	addr, _ := ch.Config["address"].(string)
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return notify.Permanent(errors.New("email channel needs an address"))
	}
	// msg.Body is the fully rendered message (link included by the template).
	return s.mailer.Send(addr, msg.Title, msg.Body)
}

// ── channel CRUD ───────────────────────────────────────────

// channelView is the JSON shape returned to clients — the encrypted secret is
// never exposed, only whether one is set.
type channelView struct {
	ID        uuid.UUID       `json:"id"`
	Type      string          `json:"type"`
	Label     string          `json:"label"`
	Config    json.RawMessage `json:"config"`
	Template  string          `json:"template"`
	Enabled   bool            `json:"enabled"`
	Verified  bool            `json:"verified"`
	HasSecret bool            `json:"has_secret"`
	CreatedAt time.Time       `json:"created_at"`
}

func channelViewOf(r db.NotificationChannel) channelView {
	cfg := json.RawMessage(r.Config)
	if len(cfg) == 0 {
		cfg = json.RawMessage("{}")
	}
	return channelView{
		ID: r.ID, Type: r.Type, Label: r.Label, Config: cfg, Template: r.Template,
		Enabled: r.Enabled, Verified: r.Verified, HasSecret: r.SecretEnc != "",
		CreatedAt: r.CreatedAt,
	}
}

// channelReq is the create/update body. Secret carries plaintext secret fields
// (bot_token, auth_header); on update it's optional — omitted/empty keeps the
// stored secret, so the client never has to re-send it.
type channelReq struct {
	Type     string            `json:"type"`
	Label    string            `json:"label"`
	Config   json.RawMessage   `json:"config"`
	Secret   map[string]string `json:"secret"`
	Template *string           `json:"template"` // nil on update = keep stored
	Enabled  *bool             `json:"enabled"`
}

// maxTemplateLen caps a channel message template (a generous limit — these are
// short notification bodies, not documents).
const maxTemplateLen = 8192

// validateTemplate rejects an over-long or unparseable template (it must render
// against the sample data without error).
func validateTemplate(tmpl string) error {
	if strings.TrimSpace(tmpl) == "" {
		return nil
	}
	if len(tmpl) > maxTemplateLen {
		return fmt.Errorf("шаблон слишком длинный (макс %d символов)", maxTemplateLen)
	}
	if _, err := notify.Render(tmpl, notify.SampleData()); err != nil {
		return fmt.Errorf("ошибка шаблона: %s", err.Error())
	}
	return nil
}

// ListNotificationChannels returns the current user's delivery channels.
func (h *API) ListNotificationChannels(c *gin.Context) {
	rows, err := h.q.ListNotificationChannels(c, middleware.CurrentUser(c))
	if err != nil {
		fail(c)
		return
	}
	out := make([]channelView, 0, len(rows))
	for _, r := range rows {
		out = append(out, channelViewOf(r))
	}
	c.JSON(http.StatusOK, out)
}

// CreateNotificationChannel adds a delivery channel for the current user.
func (h *API) CreateNotificationChannel(c *gin.Context) {
	var req channelReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	typ := strings.TrimSpace(req.Type)
	if _, ok := h.senders[typ]; !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("неизвестный тип канала: %q", typ)})
		return
	}
	cfg := parseConfig(req.Config)
	if err := validateChannel(typ, cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// Some types need a secret on create (telegram bot token, shoutrrr URL).
	if k := requiredSecret[typ]; k != "" && strings.TrimSpace(req.Secret[k]) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("канал %q требует %s", typ, k)})
		return
	}
	tmpl := ""
	if req.Template != nil {
		tmpl = *req.Template
	}
	if err := validateTemplate(tmpl); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	enc, err := h.sealSecret(req.Secret)
	if err != nil {
		fail(c)
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	cfgJSON, _ := json.Marshal(cfg)
	row, err := h.q.CreateNotificationChannel(c, db.CreateNotificationChannelParams{
		UserID: middleware.CurrentUser(c), Type: typ, Label: strings.TrimSpace(req.Label),
		Config: cfgJSON, SecretEnc: enc, Enabled: enabled, Template: tmpl,
	})
	if err != nil {
		fail(c)
		return
	}
	c.JSON(http.StatusCreated, channelViewOf(row))
}

// UpdateNotificationChannel edits a channel in place, keeping the stored secret
// unless a new one is supplied.
func (h *API) UpdateNotificationChannel(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	uid := middleware.CurrentUser(c)
	row, err := h.q.GetNotificationChannel(c, db.GetNotificationChannelParams{ID: id, UserID: uid})
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c)
		return
	}
	var req channelReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	cfg := parseConfig(req.Config)
	if err := validateChannel(row.Type, cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// Re-seal only when a new secret was supplied; otherwise keep the stored one.
	secEnc := row.SecretEnc
	if hasSecret(req.Secret) {
		if secEnc, err = h.sealSecret(req.Secret); err != nil {
			fail(c)
			return
		}
	}
	if k := requiredSecret[row.Type]; k != "" && secEnc == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("канал %q требует %s", row.Type, k)})
		return
	}
	tmpl := row.Template
	if req.Template != nil {
		tmpl = *req.Template
	}
	if err := validateTemplate(tmpl); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	enabled := row.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	cfgJSON, _ := json.Marshal(cfg)
	updated, err := h.q.UpdateNotificationChannel(c, db.UpdateNotificationChannelParams{
		ID: id, UserID: uid, Label: strings.TrimSpace(req.Label),
		Config: cfgJSON, SecretEnc: secEnc, Enabled: enabled, Template: tmpl,
	})
	if err != nil {
		fail(c)
		return
	}
	c.JSON(http.StatusOK, channelViewOf(updated))
}

// DeleteNotificationChannel removes one of the current user's channels.
func (h *API) DeleteNotificationChannel(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.q.DeleteNotificationChannel(c, db.DeleteNotificationChannelParams{
		ID: id, UserID: middleware.CurrentUser(c),
	}); err != nil {
		fail(c)
		return
	}
	c.Status(http.StatusNoContent)
}

// RegisterDeviceChannel upserts the calling client's "device" channel by its
// stable device_id (auto-registration on app start). The channel then appears in
// the channel list and can be targeted by routing rules — so a notification
// flagged for this device shows as a native OS notification on it. Idempotent.
func (h *API) RegisterDeviceChannel(c *gin.Context) {
	var req struct {
		DeviceID string `json:"device_id" binding:"required"`
		Label    string `json:"label"`
		Platform string `json:"platform"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	uid := middleware.CurrentUser(c)
	label := strings.TrimSpace(req.Label)
	if label == "" {
		label = "Устройство"
	}
	cfg, _ := json.Marshal(map[string]string{"device_id": req.DeviceID, "platform": strings.TrimSpace(req.Platform)})

	if existing, err := h.q.GetDeviceChannel(c, db.GetDeviceChannelParams{UserID: uid, DeviceID: req.DeviceID}); err == nil {
		// Refresh the label/platform; keep enabled + template as the user set them.
		updated, uerr := h.q.UpdateNotificationChannel(c, db.UpdateNotificationChannelParams{
			ID: existing.ID, UserID: uid, Label: label, Config: cfg,
			SecretEnc: existing.SecretEnc, Enabled: existing.Enabled, Template: existing.Template,
		})
		if uerr != nil {
			fail(c)
			return
		}
		c.JSON(http.StatusOK, channelViewOf(updated))
		return
	}
	row, err := h.q.CreateNotificationChannel(c, db.CreateNotificationChannelParams{
		UserID: uid, Type: "device", Label: label, Config: cfg, SecretEnc: "", Enabled: true, Template: "",
	})
	if err != nil {
		fail(c)
		return
	}
	c.JSON(http.StatusCreated, channelViewOf(row))
}

// TestNotificationChannel sends a sample message through the channel right now
// (synchronously, off the outbox) and flips `verified` on success.
func (h *API) TestNotificationChannel(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	uid := middleware.CurrentUser(c)
	row, err := h.q.GetNotificationChannel(c, db.GetNotificationChannelParams{ID: id, UserID: uid})
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c)
		return
	}
	ch, err := h.channelFromRow(row)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	sender, ok := h.senders[row.Type]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "неизвестный тип канала"})
		return
	}
	// Render through the channel's own template (with sample data) so the test
	// reflects exactly what real messages will look like.
	body, rerr := renderChannel(row.Template, notify.SampleData())
	if rerr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "ошибка шаблона: " + rerr.Error()})
		return
	}
	msg := notify.Message{Kind: "test", Title: "Проверка канала", Body: body, Link: h.publicURL}
	if err := sender.Send(c, ch, msg); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"ok": false, "error": err.Error()})
		return
	}
	_ = h.q.SetNotificationChannelVerified(c, db.SetNotificationChannelVerifiedParams{
		ID: id, UserID: uid, Verified: true,
	})
	resp := gin.H{"ok": true}
	if row.Type == "email" && !h.mailer.Enabled() {
		resp["warning"] = "SMTP на сервере не настроен — письмо записано в лог, но не отправлено."
	}
	c.JSON(http.StatusOK, resp)
}

// PreviewNotificationTemplate renders a template against sample data so the editor
// can show a live preview. Parse/field errors come back as {ok:false,error} (200)
// so the editor can display them inline rather than as a request failure.
func (h *API) PreviewNotificationTemplate(c *gin.Context) {
	var req struct {
		Template string `json:"template"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.Template) > maxTemplateLen {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("шаблон слишком длинный (макс %d символов)", maxTemplateLen)})
		return
	}
	out, err := renderChannel(req.Template, notify.SampleData())
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "text": out})
}

// ── route CRUD ─────────────────────────────────────────────

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

// ── dispatch (enqueue) ─────────────────────────────────────

// deliverNotification persists a notification, pushes it live over the workspace
// socket, and routes it to the user's external channels. actorID is nil for
// system-generated notifications (the due/reminder scanner). Best-effort — used
// both on the request path (via notify) and off it (the scanner).
func (h *API) deliverNotification(ctx context.Context, userID, wsID uuid.UUID, taskID, actorID *uuid.UUID, kind, text string) {
	n, err := h.q.CreateNotification(ctx, db.CreateNotificationParams{
		UserID: userID, WorkspaceID: wsID, TaskID: taskID, ActorID: actorID, Kind: kind, Text: text,
	})
	if err != nil {
		return
	}
	// Route first so the broadcast can carry which devices should raise a native
	// notification (external channels are enqueued to the outbox in here).
	deviceTargets := h.routeNotification(ctx, n)
	// Enrich the live payload with the task's board id + number so a freshly
	// pushed notification is clickable (the list endpoint joins these in).
	obj := gin.H{
		"id": n.ID, "user_id": n.UserID, "workspace_id": n.WorkspaceID,
		"task_id": n.TaskID, "actor_id": n.ActorID, "kind": n.Kind,
		"text": n.Text, "read_at": n.ReadAt, "created_at": n.CreatedAt,
	}
	if taskID != nil {
		if t, terr := h.q.GetTask(ctx, *taskID); terr == nil {
			obj["task_board_id"] = t.BoardID
			obj["task_number"] = t.Number
		}
	}
	h.broadcast(wsID, "notification", gin.H{
		"user_id": userID, "notification": obj, "device_targets": deviceTargets,
	})
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

// ── delivery worker ────────────────────────────────────────

const (
	notifyWorkerTick    = 10 * time.Second
	notifyBatchSize     = 20
	maxDeliveryAttempts = 5
)

// RunNotificationWorker drains the delivery outbox on a timer: claims due pending
// rows, sends them, and marks each sent / retried (backoff) / failed. Blocks
// until ctx is done; start it in a goroutine. Idle (a cheap claim query) until a
// user configures channels and rules.
func (h *API) RunNotificationWorker(ctx context.Context) {
	ticker := time.NewTicker(notifyWorkerTick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.drainDeliveries(ctx)
		}
	}
}

func (h *API) drainDeliveries(ctx context.Context) {
	rows, err := h.q.ClaimPendingDeliveries(ctx, notifyBatchSize)
	if err != nil || len(rows) == 0 {
		return
	}
	// Digest rows (non-empty group) that fell due together are combined into one
	// message per group; everything else delivers individually.
	groups := map[string][]db.NotificationDelivery{}
	for _, d := range rows {
		if d.DigestGroup == "" {
			h.settleDelivery(ctx, d, h.deliverOne(ctx, d))
			continue
		}
		groups[d.DigestGroup] = append(groups[d.DigestGroup], d)
	}
	for _, ds := range groups {
		if len(ds) == 1 {
			h.settleDelivery(ctx, ds[0], h.deliverOne(ctx, ds[0]))
			continue
		}
		err := h.deliverGroup(ctx, ds)
		for _, d := range ds {
			h.settleDelivery(ctx, d, err)
		}
	}
}

// settleDelivery marks a delivery sent, retried (quadratic backoff) or failed
// based on the send error. Claim already bumped attempts, so d.Attempts is this
// attempt's number.
func (h *API) settleDelivery(ctx context.Context, d db.NotificationDelivery, err error) {
	if err == nil {
		_ = h.q.MarkDeliverySent(ctx, d.ID)
		return
	}
	if notify.IsPermanent(err) || int(d.Attempts) >= maxDeliveryAttempts {
		_ = h.q.MarkDeliveryFailed(ctx, db.MarkDeliveryFailedParams{ID: d.ID, LastError: truncErr(err)})
		log.Printf("notify: delivery %s gave up after %d attempt(s): %v", d.ID, d.Attempts, err)
		return
	}
	next := time.Now().Add(time.Duration(d.Attempts*d.Attempts) * time.Minute) // 1, 4, 9, 16 min
	_ = h.q.MarkDeliveryRetry(ctx, db.MarkDeliveryRetryParams{ID: d.ID, LastError: truncErr(err), NextAttemptAt: next})
}

// deliverGroup renders each notification in a digest group and sends one combined
// message through the shared channel. All rows in a group share a channel.
func (h *API) deliverGroup(ctx context.Context, ds []db.NotificationDelivery) error {
	row, err := h.q.GetNotificationChannelByID(ctx, ds[0].ChannelID)
	if err != nil {
		return notify.Permanent(fmt.Errorf("channel gone: %w", err))
	}
	if !row.Enabled {
		return notify.Permanent(errors.New("channel disabled"))
	}
	sender, ok := h.senders[row.Type]
	if !ok {
		return notify.Permanent(fmt.Errorf("no sender for channel type %q", row.Type))
	}
	ch, err := h.channelFromRow(row)
	if err != nil {
		return notify.Permanent(err)
	}
	parts := make([]string, 0, len(ds))
	for _, d := range ds {
		n, nerr := h.q.GetNotification(ctx, d.NotificationID)
		if nerr != nil {
			continue
		}
		if body, rerr := renderChannel(row.Template, h.templateData(ctx, n)); rerr == nil {
			parts = append(parts, "• "+body)
		}
	}
	if len(parts) == 0 {
		return nil
	}
	combined := fmt.Sprintf("Сводка — %d уведомлений:\n\n%s", len(parts), strings.Join(parts, "\n\n"))
	return sender.Send(ctx, ch, notify.Message{Kind: "digest", Title: "Сводка уведомлений", Body: combined, Link: h.publicURL})
}

// deliverOne sends a single outbox row. A missing notification/channel or a
// disabled channel is permanent (don't retry); send errors carry their own
// transient/permanent classification.
func (h *API) deliverOne(ctx context.Context, d db.NotificationDelivery) error {
	n, err := h.q.GetNotification(ctx, d.NotificationID)
	if err != nil {
		return notify.Permanent(fmt.Errorf("notification gone: %w", err))
	}
	row, err := h.q.GetNotificationChannelByID(ctx, d.ChannelID)
	if err != nil {
		return notify.Permanent(fmt.Errorf("channel gone: %w", err))
	}
	if !row.Enabled {
		return notify.Permanent(errors.New("channel disabled"))
	}
	sender, ok := h.senders[row.Type]
	if !ok {
		return notify.Permanent(fmt.Errorf("no sender for channel type %q", row.Type))
	}
	ch, err := h.channelFromRow(row)
	if err != nil {
		return notify.Permanent(err)
	}
	body, err := renderChannel(row.Template, h.templateData(ctx, n))
	if err != nil {
		// A broken template can't fix itself between retries.
		return notify.Permanent(fmt.Errorf("template render: %w", err))
	}
	return sender.Send(ctx, ch, notify.Message{Kind: n.Kind, Title: notifyTitle(n.Kind), Body: body, Link: h.publicURL})
}

// renderChannel renders a channel's template (or the built-in default) against
// the data.
func renderChannel(tmpl string, data notify.TemplateData) (string, error) {
	if strings.TrimSpace(tmpl) == "" {
		tmpl = notify.DefaultTemplate
	}
	return notify.Render(tmpl, data)
}

// templateData enriches a notification into the data its channel templates render
// against (task, actor and workspace looked up best-effort).
func (h *API) templateData(ctx context.Context, n db.Notification) notify.TemplateData {
	d := notify.TemplateData{Kind: n.Kind, Title: notifyTitle(n.Kind), Text: n.Text, Link: h.publicURL}
	if n.TaskID != nil {
		if t, err := h.q.GetTask(ctx, *n.TaskID); err == nil {
			d.TaskTitle = t.Title
			d.TaskNumber = t.Number
		}
	}
	if n.ActorID != nil {
		if u, err := h.q.GetUserByID(ctx, *n.ActorID); err == nil {
			d.Actor = u.Name
		}
	}
	if w, err := h.q.GetWorkspace(ctx, n.WorkspaceID); err == nil {
		d.Workspace = w.Name
	}
	return d
}

// channelFromRow decodes a stored channel into the transport-facing form,
// decrypting its secret blob.
func (h *API) channelFromRow(row db.NotificationChannel) (notify.Channel, error) {
	cfg := map[string]any{}
	if len(row.Config) > 0 {
		_ = json.Unmarshal(row.Config, &cfg)
	}
	secret := map[string]string{}
	if row.SecretEnc != "" {
		plain, err := h.sealer.Decrypt(row.SecretEnc)
		if err != nil {
			return notify.Channel{}, fmt.Errorf("decrypt channel secret: %w", err)
		}
		_ = json.Unmarshal([]byte(plain), &secret)
	}
	return notify.Channel{Type: row.Type, Label: row.Label, Config: cfg, Secret: secret}, nil
}

// messageFor renders a notification into a deliverable message.
// notifyTitle maps a notification kind to a human subject line. No app-name
// prefix — the channel is fully app-managed, so the source is implicit.
func notifyTitle(kind string) string {
	switch kind {
	case "assigned":
		return "Назначена задача"
	case "comment":
		return "Новый комментарий"
	case "mention":
		return "Вас упомянули"
	case "due_soon":
		return "Скоро дедлайн"
	default:
		return "Уведомление"
	}
}

// ── due/reminder scanner (Phase B) ─────────────────────────

const notifyScanTick = 60 * time.Second

// RunNotificationScanner periodically emits notifications for upcoming/overdue
// task due dates (per the user's lead/repeat prefs) and for reminders whose time
// has arrived. The emitted notifications flow through the same routing + outbox as
// any other. Blocks until ctx is done; start it in a goroutine.
func (h *API) RunNotificationScanner(ctx context.Context) {
	ticker := time.NewTicker(notifyScanTick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
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
			h.deliverNotification(ctx, uid, wsID, &t.ID, nil, "due_soon", dueText(t))
			_ = h.q.UpsertDueNotificationState(ctx, db.UpsertDueNotificationStateParams{
				TaskID: t.ID, UserID: uid, FiredDue: *t.DueDate,
			})
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

// dueText is the default sentence for a due-date notification (templates can
// reformat it via the channel template).
func dueText(t db.Task) string {
	return fmt.Sprintf("Приближается срок задачи #%s «%s»", taskRef(t.Number), t.Title)
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
			text := strings.TrimSpace(r.Message)
			if text == "" {
				text = "Напоминание"
			}
			h.deliverNotification(ctx, r.UserID, h.reminderWorkspace(ctx, r), r.TaskID, nil, "reminder", text)
		}
		_ = h.q.MarkReminderNotified(ctx, r.ID)
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

// ── per-user scheduling prefs ──────────────────────────────

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

// ── helpers ────────────────────────────────────────────────

// sealSecret marshals the non-empty secret fields and encrypts them; returns ""
// for an empty secret (no blob to store).
func (h *API) sealSecret(secret map[string]string) (string, error) {
	clean := map[string]string{}
	for k, v := range secret {
		if strings.TrimSpace(v) != "" {
			clean[k] = v
		}
	}
	if len(clean) == 0 {
		return "", nil
	}
	b, _ := json.Marshal(clean)
	return h.sealer.Encrypt(string(b))
}

// channelsOwned reports whether every id is an existing channel of the user
// (empty list is allowed — a route may temporarily target nothing).
func (h *API) channelsOwned(c *gin.Context, uid uuid.UUID, ids []uuid.UUID) bool {
	for _, id := range ids {
		if _, err := h.q.GetNotificationChannel(c, db.GetNotificationChannelParams{ID: id, UserID: uid}); err != nil {
			return false
		}
	}
	return true
}

// validateChannel checks the minimal non-secret config a channel type needs.
func validateChannel(typ string, cfg map[string]any) error {
	switch typ {
	case "email":
		if cfgString(cfg, "address") == "" {
			return errors.New("email канал требует address")
		}
	case "telegram":
		if cfgString(cfg, "chat_id") == "" {
			return errors.New("telegram канал требует chat_id")
		}
	case "webhook":
		if cfgString(cfg, "url") == "" {
			return errors.New("webhook канал требует url")
		}
	case "shoutrrr":
		// The service URL is a secret (it carries credentials) — validated there.
	case "device":
		if cfgString(cfg, "device_id") == "" {
			return errors.New("device канал требует device_id")
		}
	default:
		return fmt.Errorf("неизвестный тип канала: %q", typ)
	}
	return nil
}

// parseConfig decodes a config object, returning an empty map on absent/invalid
// input (so callers always store canonical JSON).
func parseConfig(raw json.RawMessage) map[string]any {
	cfg := map[string]any{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &cfg)
	}
	return cfg
}

// cfgString reads a config string, coercing a JSON number (e.g. chat_id).
func cfgString(cfg map[string]any, key string) string {
	switch v := cfg[key].(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return ""
	}
}

// hasSecret reports whether a secret map carries any non-empty field.
func hasSecret(secret map[string]string) bool {
	for _, v := range secret {
		if strings.TrimSpace(v) != "" {
			return true
		}
	}
	return false
}

// orJSONObj returns raw, or "{}" when it's empty/null (JSONB columns never store
// a bare null this way, but request bodies may omit the field).
func orJSONObj(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || string(raw) == "null" {
		return json.RawMessage("{}")
	}
	return raw
}

// orIDs normalises a nil id slice to empty (for the uuid[] column).
func orIDs(ids []uuid.UUID) []uuid.UUID {
	if ids == nil {
		return []uuid.UUID{}
	}
	return ids
}

// truncErr renders an error for the last_error column, capped so a verbose
// upstream body doesn't bloat the row.
func truncErr(err error) string {
	s := err.Error()
	if len(s) > 500 {
		s = s[:500]
	}
	return s
}
