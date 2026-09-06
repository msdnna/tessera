package handlers

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"tessera/internal/db"
	"tessera/internal/mail"
	"tessera/internal/realtime"
)

func TestAdvisoryKeyStable(t *testing.T) {
	names := []string{"gitlab_sync", "gitlab_writeback", "notify_delivery", "notify_scan", "recurrence"}
	seen := make(map[int64]string, len(names))
	for _, n := range names {
		k := advisoryKey(n)
		if again := advisoryKey(n); again != k {
			t.Fatalf("advisoryKey(%q) is not deterministic: %d vs %d", n, k, again)
		}
		if other, dup := seen[k]; dup {
			t.Fatalf("advisoryKey collides: %q and %q both hash to %d", n, other, k)
		}
		seen[k] = n
	}
}

// withAdvisoryLock must skip its work when another session already holds the lock
// (the deploy-overlap case), and run it once the lock frees. Uses only advisory
// locks — no table rows — so it's immune to the main harness's truncation.
func TestWithAdvisoryLockSkipsWhenHeld(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://tessera:tessera@localhost:5432/tessera_test?sslmode=disable"
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Skipf("no test DB: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("no test DB: %v", err)
	}

	api := NewAPI(db.New(pool), pool, realtime.NewHub(), t.TempDir(),
		"integration-test-encryption-key", mail.New(mail.Config{}), "http://test", "")

	const name = "test-worklock-skip"
	key := advisoryKey(name)

	// Hold the lock on a dedicated connection (its own session).
	holder, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var got bool
	if err := holder.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&got); err != nil || !got {
		holder.Release()
		t.Fatalf("could not take the holder lock: got=%v err=%v", got, err)
	}

	ran := false
	if got := api.withAdvisoryLock(ctx, name, func() { ran = true }); got {
		_, _ = holder.Exec(ctx, "SELECT pg_advisory_unlock($1)", key)
		holder.Release()
		t.Fatal("withAdvisoryLock reported it ran while another session held the lock")
	}
	if ran {
		_, _ = holder.Exec(ctx, "SELECT pg_advisory_unlock($1)", key)
		holder.Release()
		t.Fatal("work ran while the lock was held by another session")
	}

	// Release, and confirm the work runs now.
	if _, err := holder.Exec(ctx, "SELECT pg_advisory_unlock($1)", key); err != nil {
		t.Fatal(err)
	}
	holder.Release()

	ran2 := false
	if got := api.withAdvisoryLock(ctx, name, func() { ran2 = true }); !got {
		t.Fatal("withAdvisoryLock reported a skip after the lock was released")
	}
	if !ran2 {
		t.Fatal("work did not run after the lock was released")
	}
}

// The regression this task is about: fn cancels the context it was handed (a
// worker being torn down mid-tick — exactly what drainOutboxUntil does every
// couple of seconds). The deferred pg_advisory_unlock must still reach the
// server; otherwise the connection returns to the pool with the session lock
// still held and every *other* session silently loses the try-lock from then on.
// Checked from an independent pool so the probe cannot land on the very
// connection that leaked (advisory locks are re-entrant within one session and
// would hide the bug).
func TestWithAdvisoryLockReleasesOnCancelledContext(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://tessera:tessera@localhost:5432/tessera_test?sslmode=disable"
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Skipf("no test DB: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("no test DB: %v", err)
	}
	probePool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Skipf("no test DB: %v", err)
	}
	defer probePool.Close()

	api := NewAPI(db.New(pool), pool, realtime.NewHub(), t.TempDir(),
		"integration-test-encryption-key", mail.New(mail.Config{}), "http://test", "")

	const name = "test-worklock-cancel"
	key := advisoryKey(name)

	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	ran := false
	if got := api.withAdvisoryLock(workCtx, name, func() { ran = true; cancel() }); !got {
		t.Fatal("withAdvisoryLock reported a skip on a free lock")
	}
	if !ran {
		t.Fatal("work did not run on a free lock")
	}

	probe, err := probePool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer probe.Release()
	var free bool
	if err := probe.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&free); err != nil {
		t.Fatal(err)
	}
	if !free {
		t.Fatal("advisory lock still held after the work returned: the deferred unlock ran on the cancelled context and never reached the server")
	}
	if _, err := probe.Exec(ctx, "SELECT pg_advisory_unlock($1)", key); err != nil {
		t.Fatal(err)
	}
}
