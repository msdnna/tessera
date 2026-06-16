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
	body := msg.Body
	if msg.Link != "" {
		body += "\n\n" + msg.Link
	}
	return s.mailer.Send(addr, msg.Title, body)
}

// ── channel CRUD ───────────────────────────────────────────

// channelView is the JSON shape returned to clients — the encrypted secret is
// never exposed, only whether one is set.
type channelView struct {
	ID        uuid.UUID       `json:"id"`
	Type      string          `json:"type"`
	Label     string          `json:"label"`
	Config    json.RawMessage `json:"config"`
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
		ID: r.ID, Type: r.Type, Label: r.Label, Config: cfg,
		Enabled: r.Enabled, Verified: r.Verified, HasSecret: r.SecretEnc != "",
		CreatedAt: r.CreatedAt,
	}
}

// channelReq is the create/update body. Secret carries plaintext secret fields
// (bot_token, auth_header); on update it's optional — omitted/empty keeps the
// stored secret, so the client never has to re-send it.
type channelReq struct {
	Type    string            `json:"type"`
	Label   string            `json:"label"`
	Config  json.RawMessage   `json:"config"`
	Secret  map[string]string `json:"secret"`
	Enabled *bool             `json:"enabled"`
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
		Config: cfgJSON, SecretEnc: enc, Enabled: enabled,
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
	enabled := row.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	cfgJSON, _ := json.Marshal(cfg)
	updated, err := h.q.UpdateNotificationChannel(c, db.UpdateNotificationChannelParams{
		ID: id, UserID: uid, Label: strings.TrimSpace(req.Label),
		Config: cfgJSON, SecretEnc: secEnc, Enabled: enabled,
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
	msg := notify.Message{
		Kind:  "test",
		Title: "Проверка канала",
		Body:  "Это тестовое уведомление. Если вы его получили — канал настроен верно.",
		Link:  h.publicURL,
	}
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

// routeNotification evaluates a freshly created notification against the user's
// routing rules and enqueues an outbox delivery per channel of the first matching
// enabled rule (first-match-wins; a matching rule with mute=true drops it). The
// in-app notification has already been created/broadcast by notify() — this only
// gates external channels. Best-effort: a routing failure never breaks the
// originating mutation.
func (h *API) routeNotification(ctx context.Context, n db.Notification) {
	routes, err := h.q.ListNotificationRoutes(ctx, n.UserID)
	if err != nil || len(routes) == 0 {
		return
	}
	chans, err := h.q.ListNotificationChannels(ctx, n.UserID)
	if err != nil {
		return
	}
	enabled := make(map[uuid.UUID]bool, len(chans))
	for _, ch := range chans {
		if ch.Enabled {
			enabled[ch.ID] = true
		}
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
			return // matched a mute rule — deliver nowhere
		}
		for _, chID := range r.ChannelIds {
			if enabled[chID] {
				_ = h.q.CreateNotificationDelivery(ctx, db.CreateNotificationDeliveryParams{
					NotificationID: n.ID, ChannelID: chID,
				})
			}
		}
		return // first matching rule wins
	}
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
	for _, d := range rows {
		err := h.deliverOne(ctx, d)
		if err == nil {
			_ = h.q.MarkDeliverySent(ctx, d.ID)
			continue
		}
		// Claim already bumped attempts, so d.Attempts is this attempt's number.
		if notify.IsPermanent(err) || int(d.Attempts) >= maxDeliveryAttempts {
			_ = h.q.MarkDeliveryFailed(ctx, db.MarkDeliveryFailedParams{ID: d.ID, LastError: truncErr(err)})
			log.Printf("notify: delivery %s gave up after %d attempt(s): %v", d.ID, d.Attempts, err)
			continue
		}
		// Quadratic backoff: 1, 4, 9, 16 minutes.
		next := time.Now().Add(time.Duration(d.Attempts*d.Attempts) * time.Minute)
		_ = h.q.MarkDeliveryRetry(ctx, db.MarkDeliveryRetryParams{
			ID: d.ID, LastError: truncErr(err), NextAttemptAt: next,
		})
	}
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
	return sender.Send(ctx, ch, h.messageFor(n))
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
func (h *API) messageFor(n db.Notification) notify.Message {
	return notify.Message{Kind: n.Kind, Title: notifyTitle(n.Kind), Body: n.Text, Link: h.publicURL}
}

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
