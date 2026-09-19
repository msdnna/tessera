package handlers

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"

	"tessera/internal/db"
	"tessera/internal/jobs"
)

// ── Admin: background jobs panel ───────────────────────────
// Instance-level, so every handler is gated on global admin. Live state (running
// jobs + worker heartbeats) comes from the in-memory registry; finished sync runs
// are read from the durable gitlab_sync_runs so they survive a restart.

// jobDTO is the unified panel row: either a live registry entry (worker or running
// job) or a finished sync run pulled from the journal.
// Name/CurrentOp are the pre-rendered Russian fallback; the *_key fields carry the
// same thing as catalog keys so the client can render the panel in its own language
// (a name key comes with name_arg for the part that isn't translatable — a project
// label). An unknown key simply falls back to the rendered text.
type jobDTO struct {
	Key          string     `json:"key"`
	Name         string     `json:"name"`
	NameKey      string     `json:"name_key,omitempty"`
	NameArg      string     `json:"name_arg,omitempty"`
	Kind         string     `json:"kind"`
	Status       string     `json:"status"`
	CurrentOp    string     `json:"current_op,omitempty"`
	CurrentOpKey string     `json:"current_op_key,omitempty"`
	Mode         string     `json:"mode,omitempty"`
	Trigger     string     `json:"trigger,omitempty"`
	IntervalSec int        `json:"interval_sec,omitempty"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
	LastTickAt  *time.Time `json:"last_tick_at,omitempty"`
	Created     int        `json:"created"`
	Updated     int        `json:"updated"`
	Error       string     `json:"error,omitempty"`
	Cancelable  bool       `json:"cancelable"`
	Persisted   bool       `json:"persisted"` // came from durable storage (survives restart)
}

// jobsJournalWindow is how far back finished sync runs are shown in the panel,
// configurable via JOBS_JOURNAL_TTL (Go duration, e.g. "1h", "30m"). Default 1h.
func jobsJournalWindow() time.Duration {
	if v := os.Getenv("JOBS_JOURNAL_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return time.Hour
}

func entryToDTO(e jobs.Entry) jobDTO {
	return jobDTO{
		Key: e.Key, Name: e.Name, NameKey: e.NameKey, NameArg: e.NameArg, Kind: e.Kind,
		Status: string(e.Status), CurrentOp: e.CurrentOp, CurrentOpKey: e.CurrentOpKey,
		IntervalSec: e.IntervalSec, StartedAt: e.StartedAt, FinishedAt: e.FinishedAt,
		LastTickAt: e.LastTickAt, Created: int(e.Created), Updated: int(e.Updated),
		Error: e.Error, Cancelable: e.Cancelable,
	}
}

// runStatus maps a journal run status to a panel job status.
func runStatus(s string) string {
	switch s {
	case "error":
		return string(jobs.StatusFailed)
	case "running":
		return string(jobs.StatusRunning)
	default: // ok | partial
		return string(jobs.StatusDone)
	}
}

// A journal row keeps its Russian name as written; the key + label travel beside it
// so a finished run reads in the client's language too, and history stays as it was
// recorded.
func syncRunToDTO(r db.ListRecentGitlabSyncRunsRow) jobDTO {
	name := gitlabSyncJournalPrefix + r.IntegrationLabel
	return jobDTO{
		Key: "syncrun:" + r.ID.String(), Name: name, NameKey: gitlabSyncNameKey, NameArg: r.IntegrationLabel,
		Kind: jobs.KindSync, Status: runStatus(r.Status),
		Mode: r.Mode, Trigger: r.Trigger, StartedAt: &r.StartedAt, FinishedAt: r.FinishedAt,
		Created: int(r.CreatedCount), Updated: int(r.UpdatedCount), Error: r.Error, Persisted: true,
	}
}

// ListJobs returns the background-jobs panel: live worker heartbeats and running
// jobs from the registry, plus finished sync runs from the durable journal (within
// the retention window) so recent outcomes and errors survive a restart.
func (h *API) ListJobs(c *gin.Context) {
	if _, ok := h.requireGlobalAdmin(c); !ok {
		return
	}
	out := make([]jobDTO, 0, 16)
	// Live: workers + in-flight jobs. Finished jobs come from the durable journal
	// below, so drop finished syncs here to avoid showing the last run twice.
	for _, e := range h.jobs.Snapshot() {
		if e.Kind == jobs.KindSync && (e.Status == jobs.StatusDone || e.Status == jobs.StatusFailed) {
			continue
		}
		out = append(out, entryToDTO(e))
	}
	// Durable sync runs that finished within the retention window.
	since := time.Now().Add(-jobsJournalWindow())
	if runs, err := h.q.ListRecentGitlabSyncRuns(c, db.ListRecentGitlabSyncRunsParams{Since: since, Lim: 200}); err == nil {
		for _, r := range runs {
			out = append(out, syncRunToDTO(r))
		}
	}
	c.JSON(http.StatusOK, out)
}

// GetJob returns one live registry entry by key (status, current op, timings).
func (h *API) GetJob(c *gin.Context) {
	if _, ok := h.requireGlobalAdmin(c); !ok {
		return
	}
	e, ok := h.jobs.Get(c.Param("key"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}
	c.JSON(http.StatusOK, entryToDTO(e))
}

// RunJob triggers an immediate execution of a tick-loop worker (the "run now"
// action) instead of waiting for its next tick. Runs detached; returns 202. Only
// the fixed worker keys are runnable — a per-integration sync is started through
// its own /gitlab/.../sync endpoint.
//
// The run takes the worker's advisory lock, exactly as its tick loop does, and
// answers 409 when it can't get it: these sweeps don't claim the rows they work
// on, so a manual run beside a tick (or beside a second instance mid-deploy, or
// beside a second press of the button) double-sends a notification and
// double-advances a recurrence. The lock is taken here, before the response,
// rather than inside the goroutine, so a skipped run isn't reported as started.
//
// It runs through runDetached, on the process's background context rather than a
// fresh one, because it is the same sweep a tick loop runs and shutdown has to
// treat it the same way. On its own context the run is invisible to shutdown:
// the pool closes under it mid-sweep (drain's whole ordering rests on nothing
// being left to write), and settleDelivery — which reads ctx.Err() to tell a
// deploy stopping mid-send from a verdict on the delivery — is told the process
// is fine, so a notification that never went out is either marked failed for
// good or left claimed for RequeueStaleDeliveries to send a second time (#2932).
func (h *API) RunJob(c *gin.Context) {
	if _, ok := h.requireGlobalAdmin(c); !ok {
		return
	}
	key := c.Param("key")
	runner, ok := h.workerRunners()[key]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "job is not runnable on demand"})
		return
	}
	release, locked := h.tryAdvisoryLock(c.Request.Context(), runner.lock)
	if !locked {
		c.JSON(http.StatusConflict, gin.H{"error": "job is already running"})
		return
	}
	// Free the lock however the run ends, including when it never starts.
	if !h.runDetached("job:"+key, func(ctx context.Context) { defer release(); runner.run(ctx) }) {
		release()
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "server is shutting down"})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"started": true})
}

// CancelJob cancels a running cancelable job (currently: GitLab syncs). Returns 200
// on success, 409 when the job isn't cancelable or already finished.
func (h *API) CancelJob(c *gin.Context) {
	if _, ok := h.requireGlobalAdmin(c); !ok {
		return
	}
	if !h.jobs.Cancel(c.Param("key")) {
		c.JSON(http.StatusConflict, gin.H{"error": "job is not running or cannot be cancelled"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"cancelled": true})
}

// workerRunner is one on-demand entry: the work a tick does, plus the advisory
// lock that tick holds while doing it. The lock belongs here and not inside run,
// because "run now" has to know whether it got the lock before it answers.
type workerRunner struct {
	lock string
	run  func(context.Context)
}

// workerRunners maps a worker key to a single run of its work. Used by the "run
// now" action so an operator doesn't have to wait for the next tick. Each entry
// must name the same lock its tick loop uses — this is a second path into the very
// work those locks exist to keep from overlapping.
func (h *API) workerRunners() map[string]workerRunner {
	return map[string]workerRunner{
		jobGitlabSyncCron:    {gitlabSyncLock, h.autoSyncDue},
		jobGitlabWebhookCron: {gitlabSyncLock, h.drainWebhookDirty},
		jobGitlabWriteback:   {gitlabWritebackLock, h.drainWritebacks},
		jobNotifyDelivery:    {notifyDeliveryLock, h.drainDeliveries},
		jobNotifyScanner: {NotifyScanLock, func(ctx context.Context) {
			h.scanDueTasks(ctx)
			h.scanReminders(ctx)
		}},
		jobRecurrence:     {recurrenceLock, h.advanceScheduleDue},
		jobConfRecordings: {confRecordingsLock, h.SweepConferenceRecordings},
	}
}
