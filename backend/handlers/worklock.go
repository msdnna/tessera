package handlers

import (
	"context"
	"hash/fnv"
)

// withAdvisoryLock runs fn only if this process can take the named Postgres
// advisory lock. Two Tessera instances overlap on every deploy — the old process
// keeps ticking while the new one boots — and without a guard both would run the
// same background worker against the same rows at once: double-syncing an
// integration, double-sending a due notification, double-advancing a recurrence.
// pg_try_advisory_lock is non-blocking: if another instance holds it, fn is
// skipped this tick (that instance is doing the work) rather than queueing.
// A single-instance deployment (the default) always gets the lock, so this is a
// no-op there.
func (h *API) withAdvisoryLock(ctx context.Context, name string, fn func()) {
	key := advisoryKey(name)
	conn, err := h.pool.Acquire(ctx)
	if err != nil {
		return
	}
	defer conn.Release()

	var locked bool
	if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&locked); err != nil || !locked {
		return
	}
	// Session-level lock held on this one connection for the whole tick; released
	// explicitly (best-effort) and, as a backstop, when the connection is dropped.
	defer func() { _, _ = conn.Exec(ctx, "SELECT pg_advisory_unlock($1)", key) }()
	fn()
}

// advisoryKey derives a stable 64-bit lock id from a worker name.
func advisoryKey(name string) int64 {
	hsh := fnv.New64a()
	_, _ = hsh.Write([]byte("tessera:worker:" + name))
	return int64(hsh.Sum64())
}
