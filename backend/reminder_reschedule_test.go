// A reminder that already fired can be moved to a new time — "напомни завтра" is
// the whole point of a reminder list. The scanner picks up only rows with
// notified_at IS NULL, and nothing in the project ever clears that column, so a
// rescheduled reminder silently never fires again. The same design question is
// answered right next door for due dates: due_notification_state snapshots the
// due_date it fired for, "so editing the due date re-arms it".
package main

import (
	"context"
	"net/http"
	"testing"
	"time"

	"tessera/handlers"
	"tessera/internal/mail"
	"tessera/internal/realtime"
)

// TestRescheduledReminderFiresAgain moves a delivered reminder to a new time and
// expects the scanner to deliver it once more.
func TestRescheduledReminderFiresAgain(t *testing.T) {
	ctx := context.Background()
	c := signup(t)

	// Hold the scanner lock while the reminder is created, so the harness's own
	// scanner cannot deliver it before the test drives its first tick.
	release := holdScannerLock(t)

	created := c.expect(t, c.post("/reminders", map[string]any{
		"remind_at": time.Now().Add(-time.Minute).Format(time.RFC3339),
		"message":   "перезвонить в банк",
	}), http.StatusCreated)
	id, _ := created["id"].(string)
	if id == "" {
		t.Fatalf("created reminder has no id: %v", created)
	}

	release()
	api := handlers.NewAPI(testQueries, testPool, realtime.NewHub(), testUploadDir,
		"integration-test-encryption-key", mail.New(mail.Config{}), "http://test", "")
	api.ScanNotificationsOnce(ctx)
	if n := reminderNotifications(t, c.UserID); n != 1 {
		t.Fatalf("first pass: %d reminder notification(s), want 1", n)
	}

	// Editing only the message must NOT re-send what already went out: the same
	// remind_at goes back, the way the UI round-trips the untouched field.
	sameTime, _ := created["remind_at"].(string)
	if sameTime == "" {
		t.Fatalf("created reminder has no remind_at: %v", created)
	}
	c.expect(t, c.patch("/reminders/"+id, map[string]any{
		"remind_at": sameTime,
		"message":   "перезвонить в банк до обеда",
	}), http.StatusOK)
	api.ScanNotificationsOnce(ctx)
	if n := reminderNotifications(t, c.UserID); n != 1 {
		t.Fatalf("after a message-only edit: %d reminder notification(s), want 1 (no re-send)", n)
	}

	// Moving it to a new time is a reschedule: it must fire again.
	c.expect(t, c.patch("/reminders/"+id, map[string]any{
		"remind_at": time.Now().Add(-30 * time.Second).Format(time.RFC3339),
		"message":   "перезвонить в банк до обеда",
	}), http.StatusOK)
	api.ScanNotificationsOnce(ctx)
	if n := reminderNotifications(t, c.UserID); n != 2 {
		t.Fatalf("after rescheduling to a new time: %d reminder notification(s), want 2", n)
	}
}

// holdScannerLock takes the notification scanner's advisory lock and returns the
// release func (idempotent; also released when the test ends).
func holdScannerLock(t *testing.T) func() {
	t.Helper()
	ctx := context.Background()
	holder, err := testPool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire holder conn: %v", err)
	}
	key := handlers.AdvisoryLockKey(handlers.NotifyScanLock)
	var locked bool
	if err := holder.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&locked); err != nil || !locked {
		holder.Release()
		t.Fatalf("could not take the scanner lock: locked=%v err=%v", locked, err)
	}
	released := false
	release := func() {
		if released {
			return
		}
		released = true
		_, _ = holder.Exec(ctx, "SELECT pg_advisory_unlock($1)", key)
		holder.Release()
	}
	t.Cleanup(release)
	return release
}
