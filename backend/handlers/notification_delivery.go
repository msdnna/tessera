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
	"github.com/jackc/pgx/v5"

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

// emailSender delivers via the server mailer.
type emailSender struct{ mailer mail.Mailer }

func (s emailSender) Send(_ context.Context, ch notify.Channel, msg notify.Message) error {
	addr, _ := ch.Config["address"].(string)
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return notify.Permanent(errors.New("email channel needs an address"))
	}
	// A mailer with no SMTP behind it logs the body and reports success — which is
	// what keeps a verification or reset link reachable on a self-host that never
	// configured one (mail.noop). Transactional mail has a reader for that log;
	// an outbox row does not. Settling on that nil closes the row as 'sent', so
	// the channel's delivery history reads "доставлено" on every notification
	// while not one e-mail ever left the server — the channel is silently a
	// blackhole and nothing anywhere says so. Permanent: SMTP does not appear
	// between two retries, and a failed row carries the reason where the user
	// looks for it.
	if !s.mailer.Enabled() {
		return notify.Permanent(errors.New(smtpNotConfigured))
	}
	// msg.Body is the fully rendered message (link included by the template).
	return s.mailer.Send(addr, msg.Title, msg.Body)
}

// deliverNotification persists a notification, pushes it live over the workspace
// socket, and routes it to the user's external channels. actorID is nil for
// system-generated notifications (the due/reminder scanner). msg carries both the
// structured payload the client renders from and the legacy Russian text (#2801).
// Best-effort — used both on the request path (via notify) and off it (the scanner).
//
// Every notification is gated on the recipient still being a member of wsID.
// The check lives here rather than in each caller because the callers are the
// problem: notifyMentions and notifyThread do check, while the paths that walk
// a task's assignees and creator (notifyTaskParticipants, the due scanner) walk
// rows that outlive a membership — task_assignees references users, and
// RemoveMember deliberately leaves those rows standing. One gate on the single
// seam every notification passes through cannot be forgotten by the next caller.
func (h *API) deliverNotification(ctx context.Context, userID, wsID uuid.UUID, taskID, actorID *uuid.UUID, kind string, msg notifyMsg) {
	if !h.notifiableInWorkspace(ctx, userID, wsID) {
		return
	}
	n, err := h.q.CreateNotification(ctx, db.CreateNotificationParams{
		UserID: userID, WorkspaceID: wsID, TaskID: taskID, ActorID: actorID,
		Kind: kind, Text: msg.text, Payload: msg.json(),
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
		"text": n.Text, "payload": n.Payload, "read_at": n.ReadAt, "created_at": n.CreatedAt,
	}
	if taskID != nil {
		if t, terr := h.q.GetTask(ctx, *taskID); terr == nil {
			obj["task_board_id"] = t.BoardID
			obj["task_number"] = t.Number
		}
	}
	// Addressed to the recipient alone: the text is theirs (for a reminder it is
	// literally their own private message), so it must not travel to every
	// colleague's socket with a user_id for the client to compare against.
	h.broadcastToUser(wsID, userID, "notification", gin.H{
		"user_id": userID, "notification": obj, "device_targets": deviceTargets,
	})
}

// notifiableInWorkspace reports whether userID may still be told about events in
// wsID.
//
// uuid.Nil passes: reminderScope falls back to it for a reminder that is tied
// to no task and no workspace, and that notification carries the user's own text
// about their own reminder — nothing of anyone else's to leak. A read error is
// treated as "not notifiable": the alternative is leaking on exactly the
// transient failure an attacker cannot arrange, for the sake of a notification
// the next comment re-sends.
func (h *API) notifiableInWorkspace(ctx context.Context, userID, wsID uuid.UUID) bool {
	if wsID == uuid.Nil {
		return true
	}
	_, err := h.q.GetMembership(ctx, db.GetMembershipParams{WorkspaceID: wsID, UserID: userID})
	return err == nil
}

// membershipEnded is what an outbox row's history shows when the recipient left
// the workspace between the enqueue and the send.
const membershipEnded = "адресат больше не участник пространства"

// deliveryStillAllowed re-asks the membership question at send time, and reports
// a refusal as the error the outbox row settles on.
//
// deliverNotification already gated on membership — but it gated hours earlier.
// An outbox row is not sent when it is written: quiet hours hold it until the
// window ends (a whole night), and a digest holds it to the end of its slot. The
// membership can end inside that gap, and nothing between here and the transport
// looks again: the worker renders the row and sends it to a former colleague's
// inbox. Worse than the send itself is what it carries — templateData reads the
// task's *current* title, so the message states a title the team gave it after
// the reader was removed. That is the same leak reminderScope refuses to carry
// (#2932); this is the other door to it, the one that opens with time rather
// than with a stale reference.
//
// A refusal is permanent: a membership does not come back between two retries,
// and the row's last_error is where the channel's history explains the gap. A
// read error is not a refusal — it is transient, and returning it plain (not
// notify.Permanent) lets the row retry instead of being dropped on a database
// hiccup.
func (h *API) deliveryStillAllowed(ctx context.Context, n db.Notification) error {
	if n.WorkspaceID == uuid.Nil {
		return nil
	}
	_, err := h.q.GetMembership(ctx, db.GetMembershipParams{WorkspaceID: n.WorkspaceID, UserID: n.UserID})
	switch {
	case err == nil:
		return nil
	case errors.Is(err, pgx.ErrNoRows):
		return notify.Permanent(errors.New(membershipEnded))
	default:
		return fmt.Errorf("membership check: %w", err)
	}
}

const (
	notifyWorkerTick = 10 * time.Second
	// notifyBatchSize is how many delivery *batches* one drain claims, not rows:
	// a digest group is claimed whole, because only rows returned by the same
	// claim are ever combined into one message (see ClaimPendingDeliveries). An
	// ungrouped delivery is a batch of one.
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
	h.tick(jobNotifyDelivery, opDelivery)
	h.withAdvisoryLock(ctx, notifyDeliveryLock, func() { h.drainDeliveries(ctx) }) // drain backlog at startup
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.tick(jobNotifyDelivery, opDelivery)
			h.withAdvisoryLock(ctx, notifyDeliveryLock, func() { h.drainDeliveries(ctx) })
		}
	}
}

func (h *API) drainDeliveries(ctx context.Context) {
	// Recover rows a previous drain claimed but never settled (see
	// RequeueStaleDeliveries). Holding the worker lock is what makes this safe:
	// no other instance is draining, so nothing in 'sending' is in flight.
	if n, err := h.q.RequeueStaleDeliveries(ctx); err != nil {
		soft(ctx, "RequeueStaleDeliveries", err)
	} else if n > 0 {
		log.Printf("notify: requeued %d delivery row(s) left mid-send by a previous run", n)
	}
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
		// Per row, not one verdict for the whole group: a row the digest could not
		// render never travelled with it, and settling it on the send's outcome
		// would close it as sent.
		outcomes := h.deliverGroup(ctx, ds)
		for _, d := range ds {
			h.settleDelivery(ctx, d, outcomes[d.ID])
		}
	}
}

// settleDelivery marks a delivery sent, retried (quadratic backoff) or failed
// based on the send error. Claim already bumped attempts, so d.Attempts is this
// attempt's number.
//
// The settle writes outlive a cancelled ctx (WithoutCancel, as flushJournal does
// on the write-back side). On SIGTERM mid-drain the send fails with the ctx, and
// settling on that same ctx would fail too — leaving the row 'sending' for
// RequeueStaleDeliveries to pick up, which is recovery for a delivery that may
// already have gone out. Recording the outcome is what keeps the requeue from
// re-sending it.
func (h *API) settleDelivery(ctx context.Context, d db.NotificationDelivery, err error) {
	shuttingDown := ctx.Err() != nil
	ctx = context.WithoutCancel(ctx)
	if err == nil {
		soft(ctx, "MarkDeliverySent", h.q.MarkDeliverySent(ctx, d.ID))
		return
	}
	// A send that died with the worker's context is a deploy stopping mid-drain,
	// not a verdict on the delivery: re-arm it for the next drain. Without this
	// the cancellation arrives wrapped as a permanent error ("channel gone:
	// context canceled") and a restart would mark live deliveries failed for
	// good. Deliberately not keyed on DeadlineExceeded too: a webhook that
	// times out on its own client deadline must keep its backoff, or a slow
	// endpoint becomes a hot retry loop that never exhausts its attempts.
	if shuttingDown && errors.Is(err, context.Canceled) {
		soft(ctx, "MarkDeliveryRetry", h.q.MarkDeliveryRetry(ctx, db.MarkDeliveryRetryParams{
			ID: d.ID, LastError: truncErr(err), NextAttemptAt: time.Now(),
		}))
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
//
// Returns the outcome per delivery id, because the rows of a group do not share
// one. A channel template is validated once, against notify.SampleData, where
// every field is filled — a real notification is poorer than the sample (a
// reminder carries no task), so a template can pass the form and still fail on
// the row in front of it. deliverOne reports that as a permanent failure with the
// error on the row, which is what the channel's delivery history shows. A digest
// that settled every row on the send's outcome would instead close the dropped
// ones as sent: the items are missing from the message, the outbox says
// delivered, and nothing anywhere names the broken template. So a row that never
// made it into the message carries its own verdict, exactly the one deliverOne
// would have given it.
func (h *API) deliverGroup(ctx context.Context, ds []db.NotificationDelivery) map[uuid.UUID]error {
	out := make(map[uuid.UUID]error, len(ds))
	groupErr := func(err error) map[uuid.UUID]error {
		for _, d := range ds {
			out[d.ID] = err
		}
		return out
	}
	row, err := h.q.GetNotificationChannelByID(ctx, ds[0].ChannelID)
	if err != nil {
		return groupErr(notify.Permanent(fmt.Errorf("channel gone: %w", err)))
	}
	if !row.Enabled {
		return groupErr(notify.Permanent(errors.New("channel disabled")))
	}
	sender, ok := h.senders[row.Type]
	if !ok {
		return groupErr(notify.Permanent(fmt.Errorf("no sender for channel type %q", row.Type)))
	}
	ch, err := h.channelFromRow(row)
	if err != nil {
		return groupErr(notify.Permanent(err))
	}
	// One channel means one owner, so the whole digest is written in that user's
	// language — looked up once, not per row.
	lang := h.userLang(ctx, row.UserID)
	parts := make([]string, 0, len(ds))
	carried := make([]uuid.UUID, 0, len(ds))
	for _, d := range ds {
		n, nerr := h.q.GetNotification(ctx, d.NotificationID)
		if nerr != nil {
			out[d.ID] = notify.Permanent(fmt.Errorf("notification gone: %w", nerr))
			continue
		}
		// Per row, though the channel's owner is one user: the rows of a digest
		// come from different workspaces, and leaving one behind must not cost the
		// reader the rest of their summary.
		if merr := h.deliveryStillAllowed(ctx, n); merr != nil {
			out[d.ID] = merr
			continue
		}
		body, rerr := renderChannel(row.Template, h.templateData(ctx, n, lang))
		if rerr != nil {
			out[d.ID] = notify.Permanent(fmt.Errorf("template render: %w", rerr))
			continue
		}
		parts = append(parts, "• "+body)
		carried = append(carried, d.ID)
	}
	if len(parts) == 0 {
		return out // nothing to send; every row already carries why it is not in it
	}
	combined := fmt.Sprintf("%s\n\n%s", notify.DigestHeader(len(parts), lang), strings.Join(parts, "\n\n"))
	sendErr := sender.Send(ctx, ch, notify.Message{
		Kind: "digest", Title: notify.DigestTitle(lang), Body: combined, Link: h.publicURL,
	})
	for _, id := range carried {
		out[id] = sendErr
	}
	return out
}

// deliverOne sends a single outbox row. A missing notification/channel or a
// disabled channel is permanent (don't retry); send errors carry their own
// transient/permanent classification.
func (h *API) deliverOne(ctx context.Context, d db.NotificationDelivery) error {
	n, err := h.q.GetNotification(ctx, d.NotificationID)
	if err != nil {
		return notify.Permanent(fmt.Errorf("notification gone: %w", err))
	}
	if err := h.deliveryStillAllowed(ctx, n); err != nil {
		return err
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
	lang := h.userLang(ctx, n.UserID)
	body, err := renderChannel(row.Template, h.templateData(ctx, n, lang))
	if err != nil {
		// A broken template can't fix itself between retries.
		return notify.Permanent(fmt.Errorf("template render: %w", err))
	}
	msg := notify.Message{Kind: n.Kind, Title: notify.Title(n.Kind, lang), Body: body, Link: h.notifyLink(n), ID: n.ID.String()}
	if n.TaskID != nil {
		msg.TaskID = n.TaskID.String()
	}
	msg.ConferenceID = conferenceOf(n)
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
// against (task, actor and workspace looked up best-effort). Text and Title are
// written in lang: outside the app there is no client to translate them.
func (h *API) templateData(ctx context.Context, n db.Notification, lang string) notify.TemplateData {
	d := notify.TemplateData{
		Kind:  n.Kind,
		Title: notify.Title(n.Kind, lang),
		Text:  notifySentence(n, lang),
		Link:  h.notifyLink(n),
	}
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
		// The personal workspace is stored under its Russian seed name (#2800) and
		// captioned from name_key by whoever displays it. This delivery leaves the
		// app, so the caption is picked here, in the recipient's language; a name
		// the user chose has no key and is carried as typed.
		key := ""
		if w.NameKey != nil {
			key = *w.NameKey
		}
		d.Workspace = notify.DefaultName(key, w.Name, lang)
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

// notifySentence words a stored notification for a delivery that leaves the app.
// It renders the structured payload (#2801) in the reader's language; a row
// without a payload — written before migration 0065 — keeps the Russian sentence
// the server composed back then, which is all that row has.
func notifySentence(n db.Notification, lang string) string {
	p := map[string]any{}
	if len(n.Payload) > 0 {
		if err := json.Unmarshal(n.Payload, &p); err != nil {
			return n.Text
		}
	}
	return notify.Sentence(p, lang, n.Text)
}

// conferenceOf returns the conference a notification points at, or "" for the
// task- and workspace-scoped kinds. The id lives in the payload rather than in a
// column of its own: `notifications.task_id` is a foreign key to tasks, and a
// conference is not one (#2875).
func conferenceOf(n db.Notification) string {
	if len(n.Payload) == 0 {
		return ""
	}
	p := map[string]any{}
	if err := json.Unmarshal(n.Payload, &p); err != nil {
		return ""
	}
	id, _ := p["conference_id"].(string)
	return id
}

// notifyLink is the URL an external delivery (email, telegram, webhook, push)
// should open. Task notifications land on the app root — the client opens the
// task from the in-app feed — but a conference invitation is time-boxed, so it
// links straight into the call instead of asking the reader to find it.
func (h *API) notifyLink(n db.Notification) string { return notifyLink(h.publicURL, n) }

func notifyLink(publicURL string, n db.Notification) string {
	confID := conferenceOf(n)
	if confID == "" || publicURL == "" {
		return publicURL
	}
	return strings.TrimRight(publicURL, "/") + "/conferences/" + confID
}
