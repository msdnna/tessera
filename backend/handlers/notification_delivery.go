package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/db"
	"tessera/internal/mail"
	"tessera/internal/notify"
)

// The sending half of the notification router: the sender registry, the enqueue
// entry point (deliverNotification) and the background outbox worker that drains
// deliveries, renders templates and applies retry/backoff.

// buildSenders wires the channel transports. Email reuses the server's SMTP
// mailer (the destination is the user's address, the transport is shared), so it
// needs no per-channel secret; telegram + webhook are self-contained.
//
// "device" is the odd one out: it only gets a sender when an FCM service-account
// key is configured. Without one the entry is absent, routeNotification enqueues
// nothing, and device channels behave exactly as they did before background push
// existed — live over the socket while the app is open.
func buildSenders(mailer mail.Mailer, fcmCredentialsFile string) map[string]notify.Sender {
	s := map[string]notify.Sender{
		"email":    emailSender{mailer: mailer},
		"telegram": notify.ShoutrrrSender{}, // friendly telegram, built into a shoutrrr URL
		"webhook":  notify.WebhookSender{},  // flexible raw-JSON POST
		"shoutrrr": notify.ShoutrrrSender{}, // generic: secret is a full shoutrrr URL
	}
	if fcmCredentialsFile == "" {
		return s
	}
	fcm, err := notify.LoadFCMSender(fcmCredentialsFile)
	if err != nil {
		// Warn and carry on: an unreadable key must not keep the server down over
		// a best-effort transport.
		log.Printf("notify: background push disabled — %v", err)
		return s
	}
	log.Printf("notify: background push enabled (FCM project %s)", fcm.ProjectID)
	s["device"] = fcm
	return s
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
	h.tick(jobNotifyDelivery, "рассылка уведомлений")
	h.withAdvisoryLock(ctx, "notify_delivery", func() { h.drainDeliveries(ctx) }) // drain backlog at startup
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.tick(jobNotifyDelivery, "рассылка уведомлений")
			h.withAdvisoryLock(ctx, "notify_delivery", func() { h.drainDeliveries(ctx) })
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
		soft(ctx, "MarkDeliverySent", h.q.MarkDeliverySent(ctx, d.ID))
		return
	}
	if notify.IsPermanent(err) || int(d.Attempts) >= maxDeliveryAttempts {
		soft(ctx, "MarkDeliveryFailed", h.q.MarkDeliveryFailed(ctx, db.MarkDeliveryFailedParams{ID: d.ID, LastError: truncErr(err)}))
		log.Printf("notify: delivery %s gave up after %d attempt(s): %v", d.ID, d.Attempts, err)
		return
	}
	next := time.Now().Add(time.Duration(d.Attempts*d.Attempts) * time.Minute) // 1, 4, 9, 16 min
	soft(ctx, "MarkDeliveryRetry", h.q.MarkDeliveryRetry(ctx, db.MarkDeliveryRetryParams{ID: d.ID, LastError: truncErr(err), NextAttemptAt: next}))
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
	msg := notify.Message{Kind: n.Kind, Title: notifyTitle(n.Kind), Body: body, Link: h.publicURL, ID: n.ID.String()}
	if n.TaskID != nil {
		msg.TaskID = n.TaskID.String()
	}
	err = sender.Send(ctx, ch, msg)
	if errors.Is(err, notify.ErrPushUnregistered) {
		h.dropPushToken(ctx, row)
	}
	return err
}

// dropPushToken clears a dead FCM token from a device channel's config. The
// channel itself stays enabled: it still works over the socket, and the client
// re-registers a fresh token on its next start.
func (h *API) dropPushToken(ctx context.Context, row db.NotificationChannel) {
	cfg := map[string]any{}
	if len(row.Config) > 0 {
		_ = json.Unmarshal(row.Config, &cfg)
	}
	if _, ok := cfg["fcm_token"]; !ok {
		return
	}
	delete(cfg, "fcm_token")
	raw, err := json.Marshal(cfg)
	if err != nil {
		return
	}
	_, uerr := h.q.UpdateNotificationChannel(ctx, db.UpdateNotificationChannelParams{
		ID: row.ID, UserID: row.UserID, Label: row.Label, Config: raw,
		SecretEnc: row.SecretEnc, Enabled: row.Enabled, Template: row.Template,
	})
	soft(ctx, "UpdateNotificationChannel", uerr)
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
	case "updated":
		return "Задача изменена"
	case "moved":
		return "Задача перемещена"
	case "archived":
		return "Задача архивирована"
	case "due_soon":
		return "Скоро дедлайн"
	case "reminder":
		return "Напоминание"
	case "integration_sync":
		// Provider-neutral: the core knows "an integration synced", not GitLab.
		return "Синхронизация завершена"
	default:
		return "Уведомление"
	}
}
