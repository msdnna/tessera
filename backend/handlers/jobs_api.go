package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/db"
	"tessera/internal/jobs"
	"tessera/middleware"
)

// ── Admin: background jobs window ──────────────────────────
// Instance-level, so every handler is gated on global admin. Live state (running
// and queued jobs, worker heartbeats) comes from the in-memory registry; finished
// runs come from the durable history (job_runs, #3067) so they survive a restart
// and stay visible for 30 days.

// jobDTO is the unified window row: a live registry entry (worker, running or
// queued job) or a finished run read back from the history.
// Name/CurrentOp are the pre-rendered Russian fallback; the *_key fields carry the
// same thing as catalog keys so the client can render the window in its own
// language (a name key comes with name_arg for the part that isn't translatable —
// a project label). An unknown key simply falls back to the rendered text.
type jobDTO struct {
	jobs.Entry
	Mode      string `json:"mode,omitempty"`
	ActorName string `json:"actor_name,omitempty"`
	Persisted bool   `json:"persisted"` // came from durable storage (survives restart)
	// Log is the run's log: the live ring after ?after= for a running job, the
	// kept tail for a finished one. Only on the single-job reads.
	Log []jobs.LogLine `json:"log,omitempty"`
}

// jobsHistoryWindow is how far back finished runs are shown, configurable via
// JOBS_JOURNAL_TTL (Go duration). Default — the full 30-day retention.
func jobsHistoryWindow() time.Duration {
	if v := os.Getenv("JOBS_JOURNAL_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return jobHistoryRetention
}

// jobsHistoryLimit caps one read of the history.
const jobsHistoryLimit = 500

func entryToDTO(e jobs.Entry) jobDTO {
	d := jobDTO{Entry: e}
	if m, ok := e.Meta["mode"].(string); ok {
		d.Mode = m
	}
	return d
}

// jobRunRow is the shape both history reads return.
type jobRunRow = db.ListJobRunsRow

func runToDTO(r jobRunRow) jobDTO {
	e := jobs.Entry{
		Key: "jobrun:" + r.ID.String(), RunID: r.ID.String(), Name: r.Name, NameKey: r.NameKey, NameArg: r.NameArg,
		Kind: r.Kind, Type: r.Type, Trigger: r.Trigger, Status: jobs.Status(r.Status),
		QueuedAt: &r.QueuedAt, StartedAt: r.StartedAt, FinishedAt: r.FinishedAt,
		Created: int(r.CreatedCount), Updated: int(r.UpdatedCount), Error: r.Error,
		Attempt: int(r.Attempt), MaxAttempts: int(r.MaxAttempts),
		ProgressDone: int(r.ProgressDone), ProgressTotal: int(r.ProgressTotal),
		LogLines: int(r.LogLines), LogWarns: int(r.LogWarns), LogErrors: int(r.LogErrors),
	}
	if r.WorkspaceID != nil {
		e.WorkspaceID = r.WorkspaceID.String()
	}
	if r.ActorID != nil {
		e.ActorID = r.ActorID.String()
	}
	_ = json.Unmarshal(r.Steps, &e.Steps)
	_ = json.Unmarshal(r.Meta, &e.Meta)
	d := entryToDTO(e)
	d.ActorName = r.ActorName
	d.Persisted = true
	return d
}

func runTail(r jobRunRow) []jobs.LogLine {
	var tail []jobs.LogLine
	_ = json.Unmarshal(r.LogTail, &tail)
	return tail
}

// ListJobs returns the jobs window: live worker heartbeats, running and queued
// jobs from the registry, plus finished runs from the history (30 days, newest
// first). ?type= narrows to one job family.
func (h *API) ListJobs(c *gin.Context) {
	if _, ok := h.requireGlobalAdmin(c); !ok {
		return
	}
	typ := c.Query("type")
	out := make([]jobDTO, 0, 32)
	live := map[string]bool{}
	names := h.actorNames(c)
	// Live: workers + in-flight jobs. A finished run is read from the history
	// below, so it is dropped here to avoid showing it twice.
	for _, e := range h.jobs.Snapshot() {
		if e.Kind != jobs.KindWorker && (e.FinishedAt != nil) {
			continue
		}
		if typ != "" && e.Type != typ {
			continue
		}
		if e.RunID != "" {
			live[e.RunID] = true
		}
		d := entryToDTO(e)
		d.ActorName = names(e.ActorID)
		out = append(out, d)
	}
	since := time.Now().Add(-jobsHistoryWindow())
	if runs, err := h.q.ListJobRuns(c, db.ListJobRunsParams{Since: since, Type: typ, Lim: jobsHistoryLimit}); err == nil {
		for _, r := range runs {
			if live[r.ID.String()] {
				continue
			}
			out = append(out, runToDTO(r))
		}
	}
	c.JSON(http.StatusOK, out)
}

// actorNames resolves user ids to display names for live rows, caching per request.
func (h *API) actorNames(ctx context.Context) func(string) string {
	cache := map[string]string{}
	return func(id string) string {
		if id == "" {
			return ""
		}
		if n, ok := cache[id]; ok {
			return n
		}
		n := ""
		if uid, err := uuid.Parse(id); err == nil {
			if u, uerr := h.q.GetUserByID(ctx, uid); uerr == nil {
				n = u.Name
			}
		}
		cache[id] = n
		return n
	}
}

// lookupJob finds a job by registry key, by run id, or by a history key
// ("jobrun:<id>"): the live entry when there is one, else the history row.
func (h *API) lookupJob(ctx context.Context, key string, after int) (jobDTO, bool) {
	runKey := strings.TrimPrefix(key, "jobrun:")
	e, ok := h.jobs.Get(key)
	if !ok {
		e, ok = h.jobs.GetRun(runKey)
	}
	if ok {
		d := entryToDTO(e)
		d.ActorName = h.actorNames(ctx)(e.ActorID)
		lookup := e.Key
		if lines, live := h.jobs.LogSince(lookup, after); live {
			d.Log = lines
		}
		return d, true
	}
	id, err := uuid.Parse(runKey)
	if err != nil {
		return jobDTO{}, false
	}
	r, err := h.q.GetJobRun(ctx, id)
	if err != nil {
		return jobDTO{}, false
	}
	d := runToDTO(jobRunRow(r))
	for _, l := range runTail(jobRunRow(r)) {
		if l.Seq > after {
			d.Log = append(d.Log, l)
		}
	}
	return d, true
}

// GetJob returns one job by key or run id, with its log (?after=<seq> for only the
// lines after the ones the client already has).
func (h *API) GetJob(c *gin.Context) {
	if _, ok := h.requireGlobalAdmin(c); !ok {
		return
	}
	after, _ := strconv.Atoi(c.Query("after"))
	d, ok := h.lookupJob(c, c.Param("key"), after)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}
	c.JSON(http.StatusOK, d)
}

// GetMyJob is GetJob for the member who started a run (an export, an import):
// the run is theirs, or the caller is a global admin.
func (h *API) GetMyJob(c *gin.Context) {
	d, ok := h.ownJob(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, d)
}

// ownJob loads the run named by :id and checks the caller may read it. Writes the
// error response itself.
func (h *API) ownJob(c *gin.Context) (jobDTO, bool) {
	after, _ := strconv.Atoi(c.Query("after"))
	d, ok := h.lookupJob(c, c.Param("id"), after)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return jobDTO{}, false
	}
	uid := middleware.CurrentUser(c)
	if d.ActorID != uid.String() {
		caller, err := h.q.GetUserByID(c, uid)
		if err != nil || !caller.IsAdmin {
			c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
			return jobDTO{}, false
		}
	}
	return d, true
}

// CancelMyJob stops the caller's own queued or running export/import.
func (h *API) CancelMyJob(c *gin.Context) {
	d, ok := h.ownJob(c)
	if !ok {
		return
	}
	if !h.jobs.Cancel(d.Key) {
		c.JSON(http.StatusConflict, gin.H{"error": "job is not running or cannot be cancelled"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"cancelled": true})
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
//
// The pass is a run of its own in the registry («Запустить сейчас» · trigger
// manual), so it lands in the history with who pressed the button (#3067).
func (h *API) RunJob(c *gin.Context) {
	caller, ok := h.requireGlobalAdmin(c)
	if !ok {
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
	handle, begun := h.jobs.BeginSpec(jobs.Spec{
		Key:  "run:" + key,
		Name: jobs.Name{Key: key, Text: workerName(key)},
		Kind: jobs.KindRun, Type: workerType(key), ActorID: caller.ID.String(), Trigger: "manual",
	}, nil)
	if !begun {
		release()
		c.JSON(http.StatusConflict, gin.H{"error": "job is already running"})
		return
	}
	handle.Steps(jobs.Step{Key: "run", Label: workerOps[workerOpOf(key)]})
	// Free the lock however the run ends, including when it never starts.
	if !h.runDetached("job:"+key, func(ctx context.Context) {
		defer release()
		handle.Step("run")
		handle.Logf(jobs.LevelInfo, "Запуск вручную: %s", workerName(key))
		runner.run(ctx)
		handle.Log(jobs.LevelInfo, "Готово")
		handle.Finish(ctx.Err())
	}) {
		release()
		handle.Finish(jobs.ErrShuttingDown)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "server is shutting down"})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"started": true, "run_id": handle.RunID()})
}

// workerOpOf is the op key a worker's tick reports, for its manual run's step.
func workerOpOf(key string) string {
	switch key {
	case jobGitlabSyncCron:
		return opSyncScan
	case jobGitlabWebhookCron:
		return opWebhookScan
	case jobGitlabWriteback:
		return opWriteback
	case jobNotifyDelivery:
		return opDelivery
	case jobNotifyScanner:
		return opDueScan
	case jobRecurrence:
		return opRecurrence
	case jobConfRecordings:
		return opRecordingSweep
	case jobTrashPurge:
		return opTrashPurge
	}
	return ""
}

// CancelJob cancels a running or queued cancelable job (GitLab syncs, exports,
// imports). Returns 200 on success, 409 when the job isn't cancelable or already
// finished.
func (h *API) CancelJob(c *gin.Context) {
	if _, ok := h.requireGlobalAdmin(c); !ok {
		return
	}
	key := c.Param("key")
	if !h.jobs.Cancel(key) {
		if e, ok := h.jobs.GetRun(strings.TrimPrefix(key, "jobrun:")); !ok || !h.jobs.Cancel(e.Key) {
			c.JSON(http.StatusConflict, gin.H{"error": "job is not running or cannot be cancelled"})
			return
		}
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
		jobNotifyDelivery:    {NotifyDeliveryLock, h.drainDeliveries},
		jobNotifyScanner: {NotifyScanLock, func(ctx context.Context) {
			h.scanDueTasks(ctx)
			h.scanReminders(ctx)
		}},
		jobRecurrence:     {recurrenceLock, h.advanceScheduleDue},
		jobConfRecordings: {confRecordingsLock, h.SweepConferenceRecordings},
		jobTrashPurge:     {trashPurgeLock, h.PurgeTrash},
	}
}
