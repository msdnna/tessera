package handlers

import (
	"context"
	"time"
)

// Background-worker keys — stable identifiers for the persistent tick loops, used
// both for their heartbeat entries and for the /admin/jobs "run now" action.
const (
	jobGitlabSyncCron  = "gitlab_sync_cron"
	jobGitlabWriteback = "gitlab_writeback"
	jobNotifyDelivery  = "notify_delivery"
	jobNotifyScanner   = "notify_scanner"
	jobRecurrence      = "recurrence"
)

// backgroundWorkers is the fixed roster of tick-loop workers, in display order.
var backgroundWorkers = []struct{ key, name string }{
	{jobGitlabSyncCron, "GitLab автосинк"},
	{jobGitlabWriteback, "GitLab write-back"},
	{jobNotifyDelivery, "Доставка уведомлений"},
	{jobNotifyScanner, "Сканер сроков/напоминаний"},
	{jobRecurrence, "Повторяющиеся задачи"},
}

// RegisterBackgroundWorkers records the tick-loop workers as heartbeat entries so
// they're visible in the jobs registry from startup (each worker refreshes its
// last-tick via jobs.Tick as it loops). Call once before starting the workers.
func (h *API) RegisterBackgroundWorkers() {
	for _, w := range backgroundWorkers {
		h.jobs.RegisterWorker(w.key, w.name)
	}
}

// tick refreshes a worker's heartbeat; a thin wrapper so worker loops don't import
// the jobs package directly.
func (h *API) tick(key, op string) { h.jobs.Tick(key, op) }

// RunJobSupervisor periodically prints a logfmt summary of in-flight background
// jobs (queue depth, current job, elapsed) so operators can see activity in the
// log without the API. Silent when nothing is running. Blocks until ctx is done;
// start it in a goroutine.
func (h *API) RunJobSupervisor(ctx context.Context) {
	const tick = time.Minute
	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.jobs.LogRunningSummary()
		}
	}
}
