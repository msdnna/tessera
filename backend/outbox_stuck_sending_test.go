// Both outboxes here claim a row by flipping it to 'sending', and only ever move
// it out of that status in the same process, in the same drain. Nothing else
// does, and ClaimPending* selects `status = 'pending'` — so a row whose drain
// never got to settle it is invisible to every later drain, forever. The project
// closes exactly this case elsewhere (FailStaleGitlabSyncRuns: "closes runs left
// in 'running' by a process that died mid-sync"), which is what these tests ask
// the delivery outbox for.
package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"
)

// TestStaleSendingDeliveryIsRequeued leaves a delivery row exactly as a process
// killed between claim and settle leaves it — status 'sending', attempts already
// bumped — and requires a later drain to deliver it. Without recovery the
// notification is never sent, never retried and never marked failed: a deploy
// landing mid-drain silently drops up to a batch of e-mails, telegram messages
// and pushes.
func TestStaleSendingDeliveryIsRequeued(t *testing.T) {
	ctx := context.Background()
	c := signup(t)
	wsID := mkWorkspace(t, c, "Аутбокс: осиротевшая строка")

	var mu sync.Mutex
	posts := 0
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			mu.Lock()
			posts++
			mu.Unlock()
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer hook.Close()

	chID := mkWebhookChannel(t, c, "Хук аутбокса", hook.URL)
	delivID := stuckDelivery(t, c.UserID, wsID, chID, "уведомление, застрявшее в отправке", 1)

	drainDeliveriesUntil(t, func() bool { return deliveryStatus(t, delivID) == "sent" })

	mu.Lock()
	got := posts
	mu.Unlock()
	if got != 1 {
		t.Fatalf("webhook received %d POST(s) for the recovered delivery, want 1", got)
	}
	// attempts is not reset by the recovery, so a row whose send keeps killing the
	// process still walks its budget down to 'failed' instead of looping forever.
	var attempts int
	if err := testPool.QueryRow(ctx,
		`SELECT attempts FROM notification_deliveries WHERE id = $1`, delivID).Scan(&attempts); err != nil {
		t.Fatalf("read attempts: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts after recovery = %d, want 2 (the claim before the crash plus this one)", attempts)
	}
}

// TestStaleSendingWritebackIsRequeued is the write-back twin: a GitLab push
// frozen mid-send by a dying process has to be picked up again, not dropped. Same
// claim/settle shape, same missing recovery — and here the loss is silent
// divergence, with Tessera showing the new value while the linked issue keeps the
// old one.
func TestStaleSendingWritebackIsRequeued(t *testing.T) {
	t.Parallel()
	st := newWritebackStand(t, "gl-wbstale-user", "grp-wbstale")

	st.putBindings(t, []map[string]any{
		{"enabled": true,
			"trigger": map[string]any{"type": "completion"},
			"action":  map[string]any{"type": "set_state"}},
	})
	st.c.expect(t, st.c.patch("/tasks/"+st.taskID, map[string]any{
		"title": "WB target", "completed": true,
	}), http.StatusOK)

	// Freeze this task's rows mid-send, the state a killed process leaves behind.
	// Whether the harness's own worker already delivered them doesn't matter: the
	// push is idempotent, and what is under test is that a claimed row is never
	// abandoned.
	if _, err := testPool.Exec(context.Background(),
		`UPDATE gitlab_writebacks SET status = 'sending', attempts = 1 WHERE task_id = $1::uuid`,
		st.taskID); err != nil {
		t.Fatalf("freeze writeback rows: %v", err)
	}

	drainOutboxUntil(t, func() bool { return writebackRows(t, st.taskID)["completion"].Status == "sent" })

	// The recovered row really pushed, rather than just being marked done.
	closed := false
	for _, call := range st.w.callsMatching(http.MethodPut, "/issues/1") {
		if call.Form.Get("state_event") == "close" {
			closed = true
		}
	}
	if !closed {
		t.Fatal("the recovered write-back never reached GitLab: no state_event=close on issue !1")
	}
}

// deliveryDrainMu serialises notification-worker spawns, for the reason drainMu
// does on the write-back side: several queued on the same advisory key spend the
// whole deadline waiting their turn.
var deliveryDrainMu sync.Mutex

// mkWebhookChannel creates a webhook delivery channel aimed at url.
func mkWebhookChannel(t *testing.T, c *client, label, url string) string {
	t.Helper()
	ch := c.expect(t, c.post("/notification-channels", map[string]any{
		"type": "webhook", "label": label, "config": map[string]any{"url": url},
	}), http.StatusCreated)
	return ch["id"].(string)
}

// stuckDelivery writes a notification plus an outbox row frozen mid-send, the
// state a process killed between claim and settle leaves behind. Written directly
// because no endpoint produces it.
func stuckDelivery(t *testing.T, userID, wsID, chID, text string, attempts int) string {
	t.Helper()
	ctx := context.Background()
	var notifID string
	if err := testPool.QueryRow(ctx,
		`INSERT INTO notifications (user_id, workspace_id, kind, text)
		 VALUES ($1, $2, 'comment', $3) RETURNING id`, userID, wsID, text).Scan(&notifID); err != nil {
		t.Fatalf("insert notification: %v", err)
	}
	var delivID string
	if err := testPool.QueryRow(ctx,
		`INSERT INTO notification_deliveries (notification_id, channel_id, status, attempts, next_attempt_at)
		 VALUES ($1, $2, 'sending', $3, now() - interval '1 minute') RETURNING id`,
		notifID, chID, attempts).Scan(&delivID); err != nil {
		t.Fatalf("insert delivery: %v", err)
	}
	return delivID
}

// drainDeliveriesUntil spawns the notification worker (which drains once at
// startup) and polls cond, respawning a few times so a row that appeared between
// drains is still picked up. Mirrors drainOutboxUntil on the write-back side,
// including its lock: the harness runs a worker of its own on the same advisory
// key, and a drain that never got the lock did nothing at all.
func drainDeliveriesUntil(t *testing.T, cond func() bool) {
	t.Helper()
	deliveryDrainMu.Lock()
	defer deliveryDrainMu.Unlock()
	deadline := time.Now().Add(20 * time.Second)
	spawns := 0
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			testAPI.RunNotificationWorker(ctx)
			close(done)
		}()
		spawns++
		ok := false
		for i := 0; i < 20 && !ok; i++ {
			if cond() {
				ok = true
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		cancel()
		<-done
		if ok {
			return
		}
	}
	t.Fatalf("delivery outbox condition never met after %d worker spawns; deliveries now: %s",
		spawns, dumpDeliveries(t))
}

func deliveryStatus(t *testing.T, delivID string) string {
	t.Helper()
	var st string
	if err := testPool.QueryRow(context.Background(),
		`SELECT status FROM notification_deliveries WHERE id = $1`, delivID).Scan(&st); err != nil {
		t.Fatalf("read delivery status: %v", err)
	}
	return st
}

// dumpDeliveries renders the outbox so a failure tells a stuck row apart from an
// untouched one (the lesson dumpOutbox records on the write-back side).
func dumpDeliveries(t *testing.T) string {
	t.Helper()
	rows, err := testPool.Query(context.Background(),
		`SELECT id, status, attempts, last_error FROM notification_deliveries ORDER BY created_at`)
	if err != nil {
		return "query failed: " + err.Error()
	}
	defer rows.Close()
	out := ""
	for rows.Next() {
		var id, status, lastErr string
		var attempts int
		if err := rows.Scan(&id, &status, &attempts, &lastErr); err != nil {
			return "scan failed: " + err.Error()
		}
		out += "\n  " + id + " status=" + status + " attempts=" + strconv.Itoa(attempts) + " err=" + lastErr
	}
	if out == "" {
		return "(empty)"
	}
	return out
}

