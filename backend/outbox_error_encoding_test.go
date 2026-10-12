// A delivery that fails is only ever retired by the row that records why it
// failed — settleDelivery says so itself ("Recording the outcome is what keeps
// the requeue from re-sending it"). last_error is a text column, so the write
// that records it is only as good as the encoding of the string handed to it:
// Postgres refuses a byte sequence that is not valid UTF-8, soft() swallows the
// refusal, and the row is left exactly as a killed process leaves it.
package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"
)

// TestDeliveryErrorWithCyrillicIsRecorded fails a webhook delivery with an error
// long enough to be capped, whose text carries Russian letters — the webhook URL
// travels inside the transport error, and a raw query string is handed to
// url.Error unescaped. A cap that counts bytes can land between the two bytes of
// a letter; the resulting string never reaches the column, so the delivery keeps
// its 'sending' status, never walks its attempt budget down, and the stale
// requeue sends the notification again on every drain.
func TestDeliveryErrorWithCyrillicIsRecorded(t *testing.T) {
	c := signup(t)
	wsID := mkWorkspace(t, c, "Аутбокс: кириллица в ошибке")

	// A server opened only to borrow its port: closed again, so the send fails
	// with "connection refused" rather than hanging on a dead address.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	base := srv.URL
	srv.Close()

	// Pad the URL so that byte 500 of the error text — where the cap cuts — is
	// the second byte of a Cyrillic letter: the letters start at byte 449, an odd
	// number of bytes before the cut. The error reads `Post "<url>": …`, so
	// everything ahead of the query value is known here.
	head := `Post "` + base + `/?q=`
	pad := 449 - len(head)
	if pad < 0 {
		t.Fatalf("test server URL %q is too long to place the cut", base)
	}
	target := base + "/?q=" + strings.Repeat("a", pad) + strings.Repeat("я", 80)

	chID := mkWebhookChannel(t, c, "Хук с кириллицей", target)
	delivID := pendingDelivery(t, c.UserID, wsID, chID, "уведомление с длинной ошибкой")

	// Not merely "last_error is set": a drain that failed to settle leaves the row
	// in 'sending', and the next drain's stale requeue stamps its own marker over
	// it. Only the send's own verdict counts here.
	drainDeliveriesUntil(t, func() bool {
		e := deliveryLastError(t, delivID)
		return e != "" && e != requeueMarker
	})

	gotErr := deliveryLastError(t, delivID)
	if !utf8.ValidString(gotErr) {
		t.Fatalf("last_error is not valid UTF-8: %q", gotErr)
	}
	if !strings.HasPrefix(gotErr, `Post "`) || !strings.Contains(gotErr, "я") {
		t.Fatalf("last_error is not the send's own verdict: %q", gotErr)
	}
	if len(gotErr) > 500 {
		t.Fatalf("last_error is %d bytes, over the cap", len(gotErr))
	}
	if st := deliveryStatus(t, delivID); st != "pending" {
		t.Fatalf("delivery status = %q after a transient failure, want pending (rescheduled)", st)
	}
}

// requeueMarker is what RequeueStaleDeliveries writes over a row abandoned in
// 'sending' — the stamp that hides an unrecorded outcome.
const requeueMarker = "прервано перезапуском сервиса"

// pendingDelivery writes a notification plus an outbox row ready to be claimed.
func pendingDelivery(t *testing.T, userID, wsID, chID, text string) string {
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
		 VALUES ($1, $2, 'pending', 0, now() - interval '1 minute') RETURNING id`,
		notifID, chID).Scan(&delivID); err != nil {
		t.Fatalf("insert delivery: %v", err)
	}
	return delivID
}

func deliveryLastError(t *testing.T, delivID string) string {
	t.Helper()
	var e string
	if err := testPool.QueryRow(context.Background(),
		`SELECT last_error FROM notification_deliveries WHERE id = $1`, delivID).Scan(&e); err != nil {
		t.Fatalf("read delivery last_error: %v", err)
	}
	return e
}
