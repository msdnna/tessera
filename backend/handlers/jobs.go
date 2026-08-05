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
// intervalSec must match each worker's actual ticker so the UI's next-run estimate
// is correct.
var backgroundWorkers = []struct {
	key, name   string
	intervalSec int
}{
	{jobGitlabSyncCron, "Автосинхронизация GitLab", 30},
	{jobGitlabWriteback, "Выгрузка изменений в GitLab", 10},
	{jobNotifyDelivery, "Доставка уведомлений", 10},
	{jobNotifyScanner, "Сканирование сроков и напоминаний", 60},
	{jobRecurrence, "Повторяющиеся задачи", 60},
}

// RegisterBackgroundWorkers records the tick-loop workers as heartbeat entries so
// they're visible in the jobs registry from startup (each worker refreshes its
// last-tick via jobs.Tick as it loops). Call once before starting the workers.
func (h *API) RegisterBackgroundWorkers() {
	for _, w := range backgroundWorkers {
		h.jobs.RegisterWorker(w.key, w.name, w.intervalSec)
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
