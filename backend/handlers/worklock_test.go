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
	api.withAdvisoryLock(ctx, name, func() { ran = true })
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
	api.withAdvisoryLock(ctx, name, func() { ran2 = true })
	if !ran2 {
		t.Fatal("work did not run after the lock was released")
	}
}
