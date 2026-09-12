// The due/reminder scanner is a background sweep like any other, and background
// sweeps here run under a named Postgres advisory lock: two Tessera processes
// overlap on every deploy (the old one keeps ticking while the new one boots),
// and worklock.go names this exact scanner's failure mode — "double-sending a
// due notification" — as the reason the lock exists.
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

// TestNotificationScanRunsUnderWorkerLock holds the scanner's lock the way a
// second instance would and checks that a tick then does nothing: a due reminder
// stays unsent until the lock frees, instead of being delivered twice.
func TestNotificationScanRunsUnderWorkerLock(t *testing.T) {
	ctx := context.Background()
	c := signup(t)

	// Take the lock BEFORE creating the reminder, so the harness's own scanner
	// (which ticks on its own schedule) cannot be the one that delivers it.
	holder, err := testPool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire holder conn: %v", err)
	}
	defer holder.Release()
	key := handlers.AdvisoryLockKey(handlers.NotifyScanLock)
	var locked bool
	if err := holder.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&locked); err != nil || !locked {
		t.Fatalf("could not take the scanner lock: locked=%v err=%v", locked, err)
	}
	released := false
	release := func() {
		if !released {
			released = true
			_, _ = holder.Exec(ctx, "SELECT pg_advisory_unlock($1)", key)
		}
	}
	defer release()

	c.expect(t, c.post("/reminders", map[string]any{
		"remind_at": time.Now().Add(-time.Minute).Format(time.RFC3339),
		"message":   "напоминание под замком",
	}), http.StatusCreated)

	// A second instance's scanner tick: it must skip while the lock is held.
	api := handlers.NewAPI(testQueries, testPool, realtime.NewHub(), testUploadDir,
		"integration-test-encryption-key", mail.New(mail.Config{}), "http://test", "")
	if ran := api.ScanNotificationsOnce(ctx); ran {
		t.Fatal("scanner tick reported it ran while another instance held the lock")
	}
	if n := reminderNotifications(t, c.UserID); n != 0 {
		t.Fatalf("scanner delivered %d reminder notification(s) while the lock was held", n)
	}

	// Released → the same tick does the work, exactly once.
	release()
	if ran := api.ScanNotificationsOnce(ctx); !ran {
		t.Fatal("scanner tick skipped after the lock was released")
	}
	if n := reminderNotifications(t, c.UserID); n != 1 {
		t.Fatalf("after the lock freed: %d reminder notification(s), want 1", n)
	}
	// The reminder is marked processed, so a second tick is a no-op rather than a
	// second copy.
	if ran := api.ScanNotificationsOnce(ctx); !ran {
		t.Fatal("second scanner tick skipped unexpectedly")
	}
	if n := reminderNotifications(t, c.UserID); n != 1 {
		t.Fatalf("after a second tick: %d reminder notification(s), want 1", n)
	}
}

// reminderNotifications counts the reminder notifications a user has received.
func reminderNotifications(t *testing.T, userID string) int {
	t.Helper()
	var n int
	if err := testPool.QueryRow(context.Background(),
		"SELECT count(*) FROM notifications WHERE user_id = $1 AND kind = 'reminder'", userID).Scan(&n); err != nil {
		t.Fatalf("count reminder notifications: %v", err)
	}
	return n
}
