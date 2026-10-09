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
		UserID: userID, WorkspaceID: optionalWorkspace(wsID), TaskID: taskID, ActorID: actorID,
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
			// The in-app toast (#3117) draws the task as a chip with its status
			// glyph: the whole title (the payload inlines only a short one) and
			// the column it stands in. The recipient may read the task — it is
			// the task the notification is about.
			obj["task_title"] = t.Title
			obj["task_done"] = t.CompletedAt != nil
			if col, cerr := h.q.GetColumn(ctx, t.ColumnID); cerr == nil {
				obj["task_column"] = col.Name
				if col.NameKey != nil {
					obj["task_column_key"] = *col.NameKey
				}
			}
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
// optionalWorkspace turns the caller's workspace into the stored one: uuid.Nil
// means the notification belongs to no workspace and the column is left NULL.
//
// The column used to be NOT NULL REFERENCES workspaces(id), so the Nil that
// reminderScope hands over for a reminder tied to no task and no workspace did
// not survive the INSERT — the foreign key rejected it, deliverNotification
// returned on the error, and scanReminders marked the reminder fired anyway.
// The alarm a user set for themselves never rang and never came back: reachable
// for anyone who deleted the personal workspace their account was created with
// and is in no other one (#2932).
func optionalWorkspace(wsID uuid.UUID) *uuid.UUID {
	if wsID == uuid.Nil {
		return nil
	}
	return &wsID
}

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

// notifyTurnedOff is what an outbox row's history shows when the recipient
// switched this kind of notification off between the enqueue and the send.
const notifyTurnedOff = "адресат отключил уведомления этого типа"

// dueTaskClosed / dueTaskArchived are what an outbox row's history shows when the
// task a due-date warning is about stopped being one worth warning about between
// the enqueue and the send. Two reasons rather than one: the gesture is different
// and the history is where the user looks for which of them silenced the message.
const (
	dueTaskClosed   = "задача закрыта"
	dueTaskArchived = "задача убрана в архив"
	dueDateCleared  = "у задачи больше нет срока"
	dueDateMoved    = "срок отодвинули, до него ещё далеко"
)

// taskMovedAway is what an outbox row's history shows when the task a held
// notification names left the recipient's reach between the enqueue and the
// send — the project it lives in was transferred to another workspace.
const taskMovedAway = "задача переехала в другое пространство"

// deliveryStillAllowed re-asks, at send time, the questions that were answered
// when the row was written, and reports a refusal as the error the outbox row
// settles on: is the recipient still in the workspace, and do they still want
// this kind of notification at all.
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
	if err := h.deliveryStillWanted(ctx, n); err != nil {
		return err
	}
	if n.WorkspaceID != nil {
		_, err := h.q.GetMembership(ctx, db.GetMembershipParams{WorkspaceID: *n.WorkspaceID, UserID: n.UserID})
		switch {
		case err == nil:
		case errors.Is(err, pgx.ErrNoRows):
			return notify.Permanent(errors.New(membershipEnded))
		default:
			return fmt.Errorf("membership check: %w", err)
		}
	}
	return h.deliveryStillInReach(ctx, n)
}

// deliveryStillInReach asks the membership question the check above cannot: not
// "is the recipient still in the workspace the row was written for", but "is the
// task this message names still in a workspace they belong to".
//
// The two stopped being the same question the moment a project could be
// transferred. TransferProject re-stamps the denormalized workspace_id across a
// project's tags, notes, documents and GitLab bindings, and severs the six kinds
// of link whose two ends the move would put in different workspaces — relations,
// milestones, document links, document parents, task tags, conferences — each
// because the read on the other side never re-checks the boundary. A notification
// is in neither list: its workspace_id keeps pointing at the source, and the
// recipient never left the source, so the gate above sees a membership and waves
// the row through. What travels with it is the part templateData reads live —
// the task's *current* title, written by the team the task now belongs to, and
// its number, re-stamped out of that workspace's counter.
//
// So this is the third door into the leak deliveryStillAllowed exists to close,
// after the recipient walking out (RemoveMember) and the message waiting too
// long (quiet hours): here it is the task that walks out, under a row that was
// correct when it was written.
//
// A reminder is the exception, for the reason reminderScope already gives: its
// text is the user's own, about their own alarm, and refusing it would silence
// a private message over someone else's reorganisation. It is delivered — minus
// the task fields, which templateData drops for exactly this case.
//
// A refusal is permanent (a project does not transfer back between two retries),
// and a read error is not a refusal: taskStillInReach answers "yes" on one, so a
// database hiccup cannot drop a delivery the recipient never lost.
func (h *API) deliveryStillInReach(ctx context.Context, n db.Notification) error {
	if n.TaskID == nil || n.Kind == kindReminder {
		return nil
	}
	if h.taskStillInReach(ctx, n.UserID, *n.TaskID) {
		return nil
	}
	return notify.Permanent(errors.New(taskMovedAway))
}

// taskStillInReach reports whether userID may still be shown taskID, asked about
// the workspace the task is in *now* rather than the one a stored row remembers.
// An unreadable task or board answers yes: the caller's job is to refuse a
// delivery that would leak, not one whose lookup failed. A board that reads
// fine but is soft-deleted (0078) answers no — the task is in the trash, and a
// held delivery must not render its title after the user threw it away.
func (h *API) taskStillInReach(ctx context.Context, userID, taskID uuid.UUID) bool {
	t, err := h.q.GetTask(ctx, taskID)
	if err != nil {
		return true
	}
	if b, berr := h.q.GetBoardAny(ctx, t.BoardID); berr == nil && b.DeletedAt != nil {
		return false
	}
	ws, werr := h.q.WorkspaceIDForBoard(ctx, t.BoardID)
	if werr != nil {
		return true
	}
	return h.notifiableInWorkspace(ctx, userID, ws)
}

// deliveryStillWanted re-asks the recipient's own switch for the kinds that have
// one: "уведомления о сроке" (the user default, or the per-task override the card
// carries) and "напоминания".
//
// The switch is read once, in scanDueTasks / scanReminders, when the notification
// is created. The outbox row it leads to is not sent then: quiet hours hold it to
// the end of the window — a whole night — and a digest to the end of its slot.
// Inside that gap the user gets to change their mind, and the gesture that says
// so is exactly this switch: turning the type off at 23:30, or clearing the
// due-date checkbox on the very card the held message is about. Nothing between
// there and the transport looked again, so the morning delivered a warning about
// a deadline the user had already told the server to stop warning about, from a
// screen still showing the switch turned off.
//
// Only kinds that name a switch are asked: everything else (a mention, a comment,
// an assignment) has no such knob, and answering "no" for them would turn one
// checkbox into a global blackhole. The per-task override wins over the user
// default exactly as it does in scanDueTasks — same precedence, or the two places
// would disagree about the same card. A due warning is asked one more question on
// the way: whether its task is still one the scanner would warn about at all (see
// dueStillWorthWarningAbout).
//
// A refusal is permanent: a switch left off does not come back between two
// retries, and last_error is where the channel's history explains the gap. Unread
// prefs fall back to the defaults (both on), and an unreadable task leaves the
// user default in place: a database hiccup must not drop a message the user never
// refused.
func (h *API) deliveryStillWanted(ctx context.Context, n db.Notification) error {
	// The kind first, the prefs after: every other notification has no switch to
	// ask about, and the drain already reads four rows per delivery.
	if n.Kind != kindDueSoon && n.Kind != kindReminder {
		return nil
	}
	p, perr := h.q.GetNotificationPrefs(ctx, n.UserID)
	if perr != nil {
		p = defaultPrefs(n.UserID)
	}
	wanted := p.ReminderEnabled
	if n.Kind == kindDueSoon {
		wanted = p.DueEnabled
		if n.TaskID != nil {
			if t, terr := h.q.GetTask(ctx, *n.TaskID); terr == nil {
				lead := p.DueLeadMinutes
				if t.DueLeadMinutes != nil {
					lead = *t.DueLeadMinutes
				}
				if err := dueStillWorthWarningAbout(t, lead, time.Now()); err != nil {
					return err
				}
				if t.DueNotifyEnabled != nil {
					wanted = *t.DueNotifyEnabled
				}
			}
		}
	}
	if !wanted {
		return notify.Permanent(errors.New(notifyTurnedOff))
	}
	return nil
}

// dueStillWorthWarningAbout re-applies the scanner's own gates to the task a held
// due-date warning is about. There are three, and scanDueTasks asks all three:
// ListDueTasksForScan selects `completed_at IS NULL AND archived_at IS NULL`, the
// loop skips a task with no due date at all, and dueShouldFire refuses anything
// still outside the lead window `now >= due − lead`. A card failing any of them is
// one the scanner would not warn about.
//
// It asked once, when the notification was created, and the outbox row it led to
// is not sent then — quiet hours hold it to the end of the window, a whole night.
// What the user does inside that gap is exactly what the warning was sent to
// provoke: they finish the task, or sweep the card into the archive. Nothing
// between there and the transport looked again, so the morning delivered "срок
// задачи подходит" about a task closed the evening before, from a board where it
// is no longer even visible.
//
// The deadline itself is the third gate and the one the user is likeliest to
// move: "срок подходит" is not a record of an event, it is a claim about the
// present, and it stops being true the moment the deadline does. Dropped
// overnight, it arrives about a task that has no deadline at all; pushed out a
// week, about one nobody needs reminding of yet — and because a moved due date
// re-arms the per-(task,user) state, the correct warning still comes later, in
// its own time. The stale message does not replace the right one, it is added to
// it. Only "further away than the warning reaches" refuses: a deadline pulled in,
// or merely nudged while still inside the lead window, is approaching, which is
// all the message ever claimed.
//
// Only the state at send time counts — a card closed and reopened overnight is
// open, and its warning still goes out. The check is scoped to the due kind
// (deliveryStillWanted's own guard): a closed task still has a discussion, and
// answering "no" for a comment or a mention on it would turn a checkbox into a
// blackhole for the whole card.
func dueStillWorthWarningAbout(t db.Task, lead int32, now time.Time) error {
	if t.CompletedAt != nil {
		return notify.Permanent(errors.New(dueTaskClosed))
	}
	if t.ArchivedAt != nil {
		return notify.Permanent(errors.New(dueTaskArchived))
	}
	if t.DueDate == nil {
		return notify.Permanent(errors.New(dueDateCleared))
	}
	// Same arithmetic as dueShouldFire, clamp included: the stored lead may hold a
	// value no client offers, and `time.Duration(x) * time.Minute` overflows to a
	// NEGATIVE window from 153722868 minutes up (see maxDueMinutes).
	if now.Before(t.DueDate.Add(-time.Duration(clampDueMinutes(lead)) * time.Minute)) {
		return notify.Permanent(errors.New(dueDateMoved))
	}
	return nil
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
	h.withAdvisoryLock(ctx, NotifyDeliveryLock, func() { h.drainDeliveries(ctx) }) // drain backlog at startup
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.tick(jobNotifyDelivery, opDelivery)
			h.withAdvisoryLock(ctx, NotifyDeliveryLock, func() { h.drainDeliveries(ctx) })
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
			ID: d.ID, LastError: truncErr(err), NextAttemptAt: h.outsideQuietHours(ctx, d, time.Now()),
		}))
		return
	}
	if notify.IsPermanent(err) || int(d.Attempts) >= maxDeliveryAttempts {
		soft(ctx, "MarkDeliveryFailed", h.q.MarkDeliveryFailed(ctx, db.MarkDeliveryFailedParams{ID: d.ID, LastError: truncErr(err)}))
		log.Printf("notify: delivery %s gave up after %d attempt(s): %v", d.ID, d.Attempts, err)
		return
	}
	next := time.Now().Add(time.Duration(d.Attempts*d.Attempts) * time.Minute) // 1, 4, 9, 16 min
	soft(ctx, "MarkDeliveryRetry", h.q.MarkDeliveryRetry(ctx, db.MarkDeliveryRetryParams{
		ID: d.ID, LastError: truncErr(err), NextAttemptAt: h.outsideQuietHours(ctx, d, next),
	}))
}

// outsideQuietHours moves a retry that would land inside the recipient's quiet
// window to the moment the window ends, and leaves every other retry alone.
//
// Quiet hours were asked once, at enqueue time: routeNotification defers the row
// to the end of the window and the device path does not even write a row. Nothing
// asked again after a failed send, and the backoff is free to pick any minute —
// a delivery enqueued at 21:59 against an endpoint that is down retries at 22:00,
// 22:04, 22:13, 22:29 and 22:54, so the message arrives the moment the endpoint
// recovers, in the middle of the silence the user declared. A push is the loudest
// case: during quiet hours its row is never written at all, and the backoff was
// the one way one got in.
//
// The window is read off the recipient's prefs, so the answer is the same one
// routeNotification would give; an unreadable notification or prefs row leaves
// the backoff untouched rather than holding a delivery on a database hiccup.
func (h *API) outsideQuietHours(ctx context.Context, d db.NotificationDelivery, next time.Time) time.Time {
	n, err := h.q.GetNotification(ctx, d.NotificationID)
	if err != nil {
		return next
	}
	p, perr := h.q.GetNotificationPrefs(ctx, n.UserID)
	if perr != nil {
		p = defaultPrefs(n.UserID)
	}
	if end, quiet := quietWindow(p.QuietEnabled, int(p.QuietStartMinutes), int(p.QuietEndMinutes), p.QuietTz, next); quiet {
		return end
	}
	return next
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
	if n.TaskID != nil && h.taskRenderable(ctx, n) {
		if t, err := h.q.GetTask(ctx, *n.TaskID); err == nil {
			d.TaskTitle = t.Title
			d.TaskNumber = t.Number
		}
	}
	// A deleted task has no row left to read and the notification no task_id
	// (msgDeleted): what it was travels in the payload, and the template's task
	// fields are filled from there so a channel template naming {{.TaskTitle}}
	// still says which task went.
	if n.Kind == kindDeleted && n.TaskID == nil {
		d.TaskTitle, d.TaskNumber = deletedTaskOf(n)
	}
	if n.ActorID != nil {
		if u, err := h.q.GetUserByID(ctx, *n.ActorID); err == nil {
			d.Actor = u.Name
		}
	}
	// A notification outside every workspace (a personal reminder) leaves the
	// caption empty, which the templates already render as an absent field.
	if n.WorkspaceID != nil {
		if w, err := h.q.GetWorkspace(ctx, *n.WorkspaceID); err == nil {
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
	}
	return d
}

// taskRenderable reports whether a delivery may render the task it names into
// its message — the second half of deliveryStillInReach, and the half a reminder
// needs.
//
// Only a reminder is asked. Every other kind that names an out-of-reach task was
// already refused upstream and never reaches a template, so repeating the two
// reads for it would buy nothing; a reminder is deliberately let through, and it
// is here that it drops the one part of the message that is not its owner's —
// the current title and number of a task that walked into another workspace.
// This mirrors reminderScope's split on the scanning side: the alarm still
// rings, the stranger's edits stay behind.
func (h *API) taskRenderable(ctx context.Context, n db.Notification) bool {
	if n.Kind != kindReminder {
		return true
	}
	return h.taskStillInReach(ctx, n.UserID, *n.TaskID)
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

// deletedTaskOf reads the title and number a `deleted` notification carries in
// its payload in place of a task row.
func deletedTaskOf(n db.Notification) (string, *int64) {
	var p struct {
		Title      string `json:"title"`
		TaskNumber *int64 `json:"task_number"`
	}
	if len(n.Payload) == 0 || json.Unmarshal(n.Payload, &p) != nil {
		return "", nil
	}
	return p.Title, p.TaskNumber
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
