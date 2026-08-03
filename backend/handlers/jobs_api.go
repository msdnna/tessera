package handlers

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ── Admin: background jobs panel ───────────────────────────
// Instance-level, so every handler is gated on global admin. The registry is
// in-memory (a restart clears it); durable sync history stays in gitlab_sync_runs.

// ListJobs returns a snapshot of the jobs registry (tick-loop workers + recent
// sync runs) for the admin management modal.
func (h *API) ListJobs(c *gin.Context) {
	if _, ok := h.requireGlobalAdmin(c); !ok {
		return
	}
	c.JSON(http.StatusOK, h.jobs.Snapshot())
}

// GetJob returns one registry entry by key (status, current op, timings, counts).
func (h *API) GetJob(c *gin.Context) {
	if _, ok := h.requireGlobalAdmin(c); !ok {
		return
	}
	e, ok := h.jobs.Get(c.Param("key"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}
	c.JSON(http.StatusOK, e)
}

// RunJob triggers an immediate execution of a tick-loop worker (the "run now"
// action) instead of waiting for its next tick. Runs detached; returns 202. Only
// the fixed worker keys are runnable — a per-integration sync is started through
// its own /gitlab/.../sync endpoint.
func (h *API) RunJob(c *gin.Context) {
	if _, ok := h.requireGlobalAdmin(c); !ok {
		return
	}
	key := c.Param("key")
	run, ok := h.workerRunners()[key]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "job is not runnable on demand"})
		return
	}
	go run(context.Background())
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

// workerRunners maps a worker key to a function that runs its work once. Used by
// the "run now" action so an operator doesn't have to wait for the next tick.
func (h *API) workerRunners() map[string]func(context.Context) {
	return map[string]func(context.Context){
		jobGitlabSyncCron:  h.autoSyncDue,
		jobGitlabWriteback: h.drainWritebacks,
		jobNotifyDelivery:  h.drainDeliveries,
		jobNotifyScanner: func(ctx context.Context) {
			h.scanDueTasks(ctx)
			h.scanReminders(ctx)
		},
		jobRecurrence: h.advanceScheduleDue,
	}
}
