package handlers

import (
	"context"
	"hash/fnv"
	"sync"
	"time"
)

// Worker advisory-lock names. Kept together here, and never spelled out at a call
// site, because a second way into the same work has to reuse the same name to be
// guarded at all: a literal typed next to one caller is invisible to the next one
// added (that is how "run now" shipped without a lock). Several workers share a
// name on purpose — see gitlabSyncLock.
const (
	// gitlabSyncLock covers both GitLab sync paths: the auto-sync cron and the
	// webhook debounce worker. The dirty set is process-local, but the sync it
	// triggers must not run beside the cron one on another process.
	gitlabSyncLock      = "gitlab_sync"
	gitlabWritebackLock = "gitlab_writeback"
	// NotifyDeliveryLock and NotifyScanLock are exported so an integration test in
	// another package can hold one the way a second instance would.
	NotifyDeliveryLock = "notify_delivery"
	NotifyScanLock     = "notify_scan"
	recurrenceLock     = "recurrence"
	confRecordingsLock = "conference_recordings"
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
	release, ok := h.tryAdvisoryLock(ctx, name)
	if !ok {
		return false
	}
	defer release()
	fn()
	return true
}

// tryAdvisoryLock takes the named worker lock and returns the function that frees
// it, or ok=false when another run holds it (or the pool is unavailable). release
// is idempotent and may be called from another goroutine than the one that took
// the lock.
//
// Split out of withAdvisoryLock for the caller that must answer before the work is
// finished: the admin panel's "run now" starts a worker detached, so it has to
// learn at request time whether it got the lock — otherwise the operator is told a
// run started that was in fact skipped — and then keep holding it while the
// goroutine works.
func (h *API) tryAdvisoryLock(ctx context.Context, name string) (release func(), ok bool) {
	key := advisoryKey(name)
	conn, err := h.pool.Acquire(ctx)
	if err != nil {
		return func() {}, false
	}
	var locked bool
	if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&locked); err != nil || !locked {
		conn.Release()
		return func() {}, false
	}
	// Session-level lock held on this one connection until release. Unlock on a
	// context detached from ctx: by the time release runs ctx is often already
	// cancelled (shutdown, a returned HTTP request, or a test tearing the worker
	// down mid-drain), and pgx skips sending the query entirely on an
	// already-cancelled context — the connection would then go back to the pool
	// healthy but still holding the session lock, and every other pooled
	// connection would fail the try-lock until this one is recycled (up to
	// MaxConnIdleTime/MaxConnLifetime).
	var once sync.Once
	return func() {
		once.Do(func() {
			defer conn.Release()
			rel, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			_, _ = conn.Exec(rel, "SELECT pg_advisory_unlock($1)", key)
		})
	}, true
}

// AdvisoryLockKey exposes a worker's lock id so an integration test in another
// package can hold it the way a second instance would.
func AdvisoryLockKey(name string) int64 { return advisoryKey(name) }

// advisoryKey derives a stable 64-bit lock id from a worker name.
func advisoryKey(name string) int64 {
	hsh := fnv.New64a()
	_, _ = hsh.Write([]byte("tessera:worker:" + name))
	// #nosec G115 -- deliberate bit reinterpretation: pg_advisory_lock takes a
	// signed key and any 64-bit value is a valid lock id.
	return int64(hsh.Sum64())
}
