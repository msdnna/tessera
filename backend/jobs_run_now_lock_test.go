// The admin jobs panel can start a tick-loop worker on demand ("Запустить
// сейчас") instead of waiting for its next tick. That is a second way into the
// same work, and background sweeps here run under a named Postgres advisory lock
// for a reason worklock.go spells out: without it two runs of the same worker hit
// the same rows at once and double-send. A manual run has to respect the lock the
// same way a tick does.
package main

import (
	"context"
	"net/http"
	"testing"
	"time"

	"tessera/handlers"
)

// TestRunJobNowRespectsWorkerLock holds the scanner's worker lock the way a second
// instance mid-deploy would, then presses "run now" on the notification scanner: the
// due reminder must stay undelivered until the lock frees, instead of going out a
// second time alongside whoever holds the lock.
func TestRunJobNowRespectsWorkerLock(t *testing.T) {
	ctx := context.Background()
	c := signup(t)
	makeAdmin(t, c)

	// Take the lock BEFORE creating the reminder, so the harness's own scanner
	// (ticking on its own schedule) cannot be the one that delivers it.
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
		"message":   "напоминание под замком (run now)",
	}), http.StatusCreated)

	// The lock is held, so the button must refuse outright — reporting 202 for a run
	// that never happens is the other half of the bug.
	c.expect(t, c.post("/admin/jobs/notify_scanner/run", nil), http.StatusConflict)
	// And it must really not have run: the handler answers before the work would
	// finish, so give a detached goroutine a window to do the damage.
	if n := settledReminderCount(t, c.UserID, 0); n != 0 {
		t.Fatalf("run-now delivered %d reminder notification(s) while another run held the lock", n)
	}

	// Released → the same button does the work, exactly once.
	release()
	c.expect(t, c.post("/admin/jobs/notify_scanner/run", nil), http.StatusAccepted)
	if n := settledReminderCount(t, c.UserID, 1); n != 1 {
		t.Fatalf("after the lock freed: %d reminder notification(s), want 1", n)
	}
}

// settledReminderCount polls the reminder-notification count for a detached run to
// land. It waits for want, then keeps watching a little longer so an extra copy
// arriving late still fails the test rather than slipping past a first read.
func settledReminderCount(t *testing.T, userID string, want int) int {
	t.Helper()
	const (
		step  = 50 * time.Millisecond
		total = 3 * time.Second
	)
	deadline := time.Now().Add(total)
	settle := time.Now().Add(time.Second) // minimum watch window, even when want is 0
	for {
		n := reminderNotifications(t, userID)
		if n > want {
			return n // already wrong; no point waiting
		}
		if n == want && time.Now().After(settle) {
			return n
		}
		if time.Now().After(deadline) {
			return n
		}
		time.Sleep(step)
	}
}
