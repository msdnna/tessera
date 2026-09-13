package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/db"
	"tessera/internal/i18n"
	"tessera/internal/notify"
	"tessera/middleware"
)

// Per-user notification channels (email / telegram / webhook / shoutrrr / device):
// CRUD, config + template validation, secret sealing and the test/preview probes.
// The channel transports themselves live in internal/notify.

// requiredSecret maps a channel type to the secret field that must be present
// (set on create, kept on edit). Other types have no mandatory secret.
var requiredSecret = map[string]string{
	"telegram": "bot_token",
	"shoutrrr": "url",
}

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
	// ClearSecret wipes the stored secret blob. An empty Secret map means "keep",
	// so erasing needs an explicit flag. A supplied Secret in the same request wins.
	ClearSecret bool `json:"clear_secret"`
}

// maxTemplateLen caps a channel message template (a generous limit — these are
// short notification bodies, not documents).
const maxTemplateLen = 8192

// validateTemplate rejects an over-long or unparseable template (it must render
// against the sample data without error).
//
// The refusals are English sentinels, like every other refusal on this API: both
// clients keep a catalog (frontend/src/utils/errors.js, android Errors.kt) and
// print anything they can't place verbatim — so a Russian sentence here lands
// untranslated under an English reader's form (#2932). The ceiling is named in
// the message so each client can word it in the reader's own units.
//
// The sample is rendered in the default language on purpose: what can fail here
// is a parse error or a field the struct doesn't have, and neither depends on the
// wording.
func validateTemplate(tmpl string) error {
	if strings.TrimSpace(tmpl) == "" {
		return nil
	}
	if len(tmpl) > maxTemplateLen {
		return fmt.Errorf("template exceeds %d characters", maxTemplateLen)
	}
	if _, err := notify.Render(tmpl, notify.SampleData(i18n.Default)); err != nil {
		// The parser's own words name the offending action and have no
		// translation; only the wrapper around them is ours.
		return fmt.Errorf("template error: %s", err.Error())
	}
	return nil
}

// ListNotificationChannels returns the current user's delivery channels.
func (h *API) ListNotificationChannels(c *gin.Context) {
	rows, err := h.q.ListNotificationChannels(c, middleware.CurrentUser(c))
	if err != nil {
		fail(c, err)
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
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("unknown channel type: %q", typ)})
		return
	}
	cfg := parseConfig(req.Config)
	if err := validateChannel(typ, cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// Some types need a secret on create (telegram bot token, shoutrrr URL).
	if k := requiredSecret[typ]; k != "" && strings.TrimSpace(req.Secret[k]) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("%s channel requires %s", typ, k)})
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
		fail(c, err)
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
		fail(c, err)
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
		fail(c, err)
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
	// An explicit clear_secret wipes it (a supplied secret still wins over the flag).
	secEnc := row.SecretEnc
	if req.ClearSecret {
		secEnc = ""
	}
	if hasSecret(req.Secret) {
		if secEnc, err = h.sealSecret(req.Secret); err != nil {
			fail(c, err)
			return
		}
	}
	if k := requiredSecret[row.Type]; k != "" && secEnc == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("%s channel requires %s", row.Type, k)})
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
		fail(c, err)
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
	uid := middleware.CurrentUser(c)
	// One transaction with the scrub: channel_ids is a bare uuid[], so nothing in
	// the schema clears the reference. Leaving it behind wedges the rule — the
	// settings screen echoes the stored ids back on the next save (even a plain
	// enable toggle) and channelsOwned then rejects it as someone else's channel.
	if err := h.inTx(c, func(q *db.Queries) error {
		if err := q.DeleteNotificationChannel(c, db.DeleteNotificationChannelParams{ID: id, UserID: uid}); err != nil {
			return err
		}
		return q.StripChannelFromRoutes(c, db.StripChannelFromRoutesParams{ChannelID: id, UserID: uid})
	}); err != nil {
		fail(c, err)
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
		// FCMToken is optional: a client without Play Services (or a browser)
		// registers without one and stays on the live-socket path.
		FCMToken string `json:"fcm_token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	uid := middleware.CurrentUser(c)
	label := strings.TrimSpace(req.Label)
	if label == "" {
		// Stored, and renameable afterwards — so the name the user first sees is
		// written in their language rather than the server's.
		label = notify.DeviceLabel(h.userLang(c, uid))
	}
	newCfg := map[string]string{"device_id": req.DeviceID, "platform": strings.TrimSpace(req.Platform)}
	if tok := strings.TrimSpace(req.FCMToken); tok != "" {
		newCfg["fcm_token"] = tok
	}
	cfg, _ := json.Marshal(newCfg)

	if existing, err := h.q.GetDeviceChannel(c, db.GetDeviceChannelParams{UserID: uid, DeviceID: req.DeviceID}); err == nil {
		// Keep a token we already hold when this call didn't bring one: a
		// re-registration that couldn't reach Play Services must not silently
		// switch background push off for the device.
		if newCfg["fcm_token"] == "" {
			if prev := channelCfgString(existing, "fcm_token"); prev != "" {
				newCfg["fcm_token"] = prev
				cfg, _ = json.Marshal(newCfg)
			}
		}
		// Refresh the platform/config only. Keep the label the user set (clients
		// auto-register a generated name on every app start; overwriting it here
		// would wipe a custom rename) — along with enabled + template.
		updated, uerr := h.q.UpdateNotificationChannel(c, db.UpdateNotificationChannelParams{
			ID: existing.ID, UserID: uid, Label: existing.Label, Config: cfg,
			SecretEnc: existing.SecretEnc, Enabled: existing.Enabled, Template: existing.Template,
		})
		if uerr != nil {
			// uerr, not err: err is the *successful* lookup above, so passing it
			// logged "err=<nil>" and left Sentry without a cause to group on.
			fail(c, uerr)
			return
		}
		c.JSON(http.StatusOK, channelViewOf(updated))
		return
	}
	row, err := h.q.CreateNotificationChannel(c, db.CreateNotificationChannelParams{
		UserID: uid, Type: "device", Label: label, Config: cfg, SecretEnc: "", Enabled: true, Template: "",
	})
	if err != nil {
		fail(c, err)
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
		fail(c, err)
		return
	}
	ch, err := h.channelFromRow(row)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	sender, ok := h.senders[row.Type]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("unknown channel type: %q", row.Type)})
		return
	}
	// Render through the channel's own template (with sample data) so the test
	// reflects exactly what real messages will look like — including the language.
	// This message leaves the app, so it is written in the recipient's, exactly as
	// a real delivery is (notification_delivery.go).
	lang := h.userLang(c, uid)
	body, rerr := renderChannel(row.Template, notify.SampleData(lang))
	if rerr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "template error: " + rerr.Error()})
		return
	}
	msg := notify.Message{Kind: "test", Title: notify.Title("test", lang), Body: body, Link: h.publicURL}
	if err := sender.Send(c, ch, msg); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"ok": false, "error": err.Error()})
		return
	}
	soft(c, "SetNotificationChannelVerified", h.q.SetNotificationChannelVerified(c, db.SetNotificationChannelVerifiedParams{
		ID: id, UserID: uid, Verified: true,
	}))
	resp := gin.H{"ok": true}
	if row.Type == "email" && !h.mailer.Enabled() {
		// Shown verbatim by both clients (the web puts it on the channel row,
		// Android in a snackbar), so it travels as a sentinel they translate.
		resp["warning"] = "smtp is not configured"
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
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("template exceeds %d characters", maxTemplateLen)})
		return
	}
	// The preview stands in for a delivery, so it is written in the reader's
	// language — otherwise the editor teaches one wording and the channel carries
	// another.
	out, err := renderChannel(req.Template, notify.SampleData(h.userLang(c, middleware.CurrentUser(c))))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "text": out})
}

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

// validateChannel checks the minimal non-secret config a channel type needs. Its
// refusals are English sentinels for the clients to translate — see
// validateTemplate for why.
func validateChannel(typ string, cfg map[string]any) error {
	switch typ {
	case "email":
		if cfgString(cfg, "address") == "" {
			return errors.New("email channel requires address")
		}
	case "telegram":
		if cfgString(cfg, "chat_id") == "" {
			return errors.New("telegram channel requires chat_id")
		}
	case "webhook":
		if cfgString(cfg, "url") == "" {
			return errors.New("webhook channel requires url")
		}
	case "shoutrrr":
		// The service URL is a secret (it carries credentials) — validated there.
	case "device":
		if cfgString(cfg, "device_id") == "" {
			return errors.New("device channel requires device_id")
		}
	default:
		return fmt.Errorf("unknown channel type: %q", typ)
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
