package handlers

import (
	"context"
	"hash/fnv"
	"time"
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
//
// Reports whether fn ran; false means the lock was unavailable (or the pool was)
// and this tick did nothing. Without that signal a skipped tick is
// indistinguishable from a completed one, which is exactly how a worker that
// never ran once can look healthy from the outside.
func (h *API) withAdvisoryLock(ctx context.Context, name string, fn func()) bool {
	key := advisoryKey(name)
	conn, err := h.pool.Acquire(ctx)
	if err != nil {
		return false
	}
	defer conn.Release()

	var locked bool
	if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&locked); err != nil || !locked {
		return false
	}
	// Session-level lock held on this one connection for the whole tick. Release
	// on a context detached from ctx: by the time this runs ctx is often already
	// cancelled (shutdown, or a test tearing the worker down mid-drain), and pgx
	// skips sending the query entirely on an already-cancelled context — the
	// connection would then go back to the pool healthy but still holding the
	// session lock, and every other pooled connection would fail the try-lock
	// until this one is recycled (up to MaxConnIdleTime/MaxConnLifetime).
	defer func() {
		rel, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_, _ = conn.Exec(rel, "SELECT pg_advisory_unlock($1)", key)
	}()
	fn()
	return true
}

// advisoryKey derives a stable 64-bit lock id from a worker name.
func advisoryKey(name string) int64 {
	hsh := fnv.New64a()
	_, _ = hsh.Write([]byte("tessera:worker:" + name))
	return int64(hsh.Sum64())
}
