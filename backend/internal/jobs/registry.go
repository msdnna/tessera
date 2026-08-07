// Package jobs is a small in-memory registry that makes background work
// observable and (for long jobs) controllable: the tick-loop workers register a
// heartbeat, GitLab syncs register a cancelable run, and a supervisor prints a
// periodic logfmt summary. It is intentionally lightweight — no durable queue, no
// generic scheduler (sync history stays durable in gitlab_sync_runs); a restart
// simply starts with an empty registry.
package jobs

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Status is a job's lifecycle state.
type Status string

// Job lifecycle states.
const (
	StatusPending Status = "pending" // queued, not started
	StatusRunning Status = "running" // in flight (or, for a worker, its loop is alive)
	StatusDone    Status = "done"    // finished successfully
	StatusFailed  Status = "failed"  // finished with an error
)

// Kind distinguishes a one-shot job from a long-lived tick-loop worker.
const (
	KindSync   = "sync"   // a discrete run (e.g. a GitLab pull) — cancelable, keeps its last outcome
	KindWorker = "worker" // a persistent background loop — heartbeat only, not cancelable
)

// Entry is a snapshot of one tracked job, safe to serialize to JSON for the API.
type Entry struct {
	Key         string     `json:"key"`
	Name        string     `json:"name"`
	Kind        string     `json:"kind"`
	WorkspaceID string     `json:"workspace_id,omitempty"`
	Status      Status     `json:"status"`
	CurrentOp   string     `json:"current_op,omitempty"`
	IntervalSec int        `json:"interval_sec,omitempty"` // worker tick interval (for next-run estimate)
	QueuedAt    *time.Time `json:"queued_at,omitempty"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
	LastTickAt  *time.Time `json:"last_tick_at,omitempty"`
	Created     int        `json:"created"`
	Updated     int        `json:"updated"`
	Error       string     `json:"error,omitempty"`
	Cancelable  bool       `json:"cancelable"`
}

type slot struct {
	e      Entry
	cancel context.CancelFunc
}

// Registry tracks jobs under a mutex. The zero value is not usable — call New.
type Registry struct {
	mu  sync.RWMutex
	m   map[string]*slot
	log *slog.Logger
	now func() time.Time
}

// New builds a registry logging through the given slog.Logger (a TextHandler gives
// the logfmt summary the task asks for).
func New(logger *slog.Logger) *Registry {
	return &Registry{m: map[string]*slot{}, log: logger, now: time.Now}
}

// RegisterWorker records a persistent tick-loop worker as a running heartbeat entry.
// intervalSec is its tick period, so the UI can show when it next fires. Idempotent —
// re-registering the same key refreshes its start time.
func (r *Registry) RegisterWorker(key, name string, intervalSec int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	r.m[key] = &slot{e: Entry{Key: key, Name: name, Kind: KindWorker, Status: StatusRunning, IntervalSec: intervalSec, StartedAt: &now, LastTickAt: &now}}
}

// Tick updates a worker's heartbeat (last-tick time and current operation). No-op
// for an unknown key so a worker that predates registration can't panic.
func (r *Registry) Tick(key, op string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.m[key]
	if !ok {
		return
	}
	now := r.now()
	s.e.LastTickAt = &now
	s.e.CurrentOp = op
}

// Handle is a live reference to one running job, used to update it while it runs.
type Handle struct {
	r   *Registry
	key string
}

// Begin starts a cancelable job under key. If a job with that key is already
// pending or running it returns (nil, false) — the caller should back off (this
// replaces the old runningSyncs busy-guard). A finished entry from a previous run
// is overwritten. cancel may be nil (job is not cancelable).
func (r *Registry) Begin(key, name, kind, workspaceID string, cancel context.CancelFunc) (*Handle, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.m[key]; ok && (s.e.Status == StatusPending || s.e.Status == StatusRunning) {
		return nil, false
	}
	now := r.now()
	r.m[key] = &slot{
		e: Entry{
			Key: key, Name: name, Kind: kind, WorkspaceID: workspaceID,
			Status: StatusRunning, QueuedAt: &now, StartedAt: &now, Cancelable: cancel != nil,
		},
		cancel: cancel,
	}
	return &Handle{r: r, key: key}, true
}

// SetOp records what a running job is currently doing (shown in the API/summary).
func (h *Handle) SetOp(op string) {
	if h == nil {
		return
	}
	h.r.mu.Lock()
	defer h.r.mu.Unlock()
	if s, ok := h.r.m[h.key]; ok {
		s.e.CurrentOp = op
	}
}

// SetCounts records progress counters (e.g. created/updated tasks) for a job.
func (h *Handle) SetCounts(created, updated int) {
	if h == nil {
		return
	}
	h.r.mu.Lock()
	defer h.r.mu.Unlock()
	if s, ok := h.r.m[h.key]; ok {
		s.e.Created, s.e.Updated = created, updated
	}
}

// Finish marks a job done (or failed when err != nil), clears its cancel handle and
// current op, and keeps the entry as the job's last-run record.
func (h *Handle) Finish(err error) {
	if h == nil {
		return
	}
	h.r.mu.Lock()
	defer h.r.mu.Unlock()
	s, ok := h.r.m[h.key]
	if !ok {
		return
	}
	now := h.r.now()
	s.e.FinishedAt = &now
	s.e.CurrentOp = ""
	s.cancel = nil
	s.e.Cancelable = false
	if err != nil {
		s.e.Status, s.e.Error = StatusFailed, err.Error()
	} else {
		s.e.Status = StatusDone
	}
}

// Cancel invokes a running job's cancel func, if it has one. Returns false when the
// key is unknown, already finished, or not cancelable.
func (r *Registry) Cancel(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.m[key]
	if !ok || s.cancel == nil {
		return false
	}
	s.cancel()
	return true
}

// Snapshot returns a copy of every entry, workers first then jobs, each group
// newest-started first — a stable order for the management UI.
func (r *Registry) Snapshot() []Entry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Entry, 0, len(r.m))
	for _, s := range r.m {
		out = append(out, s.e)
	}
	sortEntries(out)
	return out
}

// Get returns one entry by key.
func (r *Registry) Get(key string) (Entry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if s, ok := r.m[key]; ok {
		return s.e, true
	}
	return Entry{}, false
}

// LogRunningSummary prints one logfmt line summarizing background work: the running
// sync jobs (with the current one and how long it's been going) or, when nothing is
// running, an idle heartbeat so the log always shows the supervisor is alive and the
// system is quiet. Returns the number of running/pending sync jobs.
func (r *Registry) LogRunningSummary() int {
	r.mu.RLock()
	var queue, workers int
	var current string
	var since time.Time
	for _, s := range r.m {
		if s.e.Kind == KindWorker {
			workers++
			continue
		}
		if s.e.Status == StatusRunning || s.e.Status == StatusPending {
			queue++
			if s.e.Status == StatusRunning && s.e.StartedAt != nil && (since.IsZero() || s.e.StartedAt.Before(since)) {
				current, since = s.e.Name, *s.e.StartedAt
			}
		}
	}
	r.mu.RUnlock()
	if queue == 0 {
		// Idle: no discrete job in flight; the tick-loop workers are alive (heartbeat).
		r.log.Info("background tasks are idle",
			slog.Int("queue", 0), slog.String("status", string(StatusPending)), slog.Int("workers", workers))
		return 0
	}
	attrs := []any{slog.Int("queue", queue), slog.String("status", string(StatusRunning))}
	if current != "" {
		attrs = append(attrs,
			slog.String("current", current),
			slog.String("processing", r.now().Sub(since).Truncate(time.Second).String()),
		)
	}
	r.log.Info("background tasks are still running", attrs...)
	return queue
}

func sortEntries(e []Entry) {
	// Simple insertion sort keeps the dependency-free package tiny; the registry
	// never holds more than a handful of entries.
	for i := 1; i < len(e); i++ {
		for j := i; j > 0 && lessEntry(e[j], e[j-1]); j-- {
			e[j], e[j-1] = e[j-1], e[j]
		}
	}
}

// lessEntry orders workers before sync jobs, then newest start first.
func lessEntry(a, b Entry) bool {
	if (a.Kind == KindWorker) != (b.Kind == KindWorker) {
		return a.Kind == KindWorker
	}
	at, bt := time.Time{}, time.Time{}
	if a.StartedAt != nil {
		at = *a.StartedAt
	}
	if b.StartedAt != nil {
		bt = *b.StartedAt
	}
	return at.After(bt)
}
