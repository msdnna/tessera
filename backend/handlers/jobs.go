package handlers

import (
	"context"
	"time"

	"tessera/internal/jobs"
	"tessera/internal/observability"
)

// Background-worker keys — stable identifiers for the persistent tick loops, used
// both for their heartbeat entries and for the /admin/jobs "run now" action.
const (
	jobGitlabSyncCron    = "gitlab_sync_cron"
	jobGitlabWebhookCron = "gitlab_webhook_cron"
	jobGitlabWriteback   = "gitlab_writeback"
	jobNotifyDelivery    = "notify_delivery"
	jobNotifyScanner     = "notify_scanner"
	jobRecurrence        = "recurrence"
	jobConfRecordings    = "conference_recordings"
	jobTrashPurge        = "trash_purge"
)

// backgroundWorkers is the fixed roster of tick-loop workers, in display order.
// intervalSec must match each worker's actual ticker so the UI's next-run estimate
// is correct.
var backgroundWorkers = []struct {
	key, name, typ string
	intervalSec    int
}{
	{jobGitlabSyncCron, "Автосинхронизация GitLab", jobs.TypeGitlab, 30},
	{jobGitlabWebhookCron, "Синхронизация GitLab по вебхуку", jobs.TypeGitlab, 1},
	{jobGitlabWriteback, "Выгрузка изменений в GitLab", jobs.TypeGitlab, 10},
	{jobNotifyDelivery, "Доставка уведомлений", jobs.TypeNotify, 10},
	{jobNotifyScanner, "Сканирование сроков и напоминаний", jobs.TypeNotify, 60},
	{jobRecurrence, "Повторяющиеся задачи", jobs.TypeMaintenance, 60},
	{jobConfRecordings, "Записи конференций", jobs.TypeMaintenance, 60},
	{jobTrashPurge, "Уборка удалённого", jobs.TypeMaintenance, 600},
}

// workerType names a worker's job family (for its on-demand runs' history).
func workerType(key string) string {
	for _, w := range backgroundWorkers {
		if w.key == key {
			return w.typ
		}
	}
	return jobs.TypeMaintenance
}

// workerName is a worker's rendered fallback name.
func workerName(key string) string {
	for _, w := range backgroundWorkers {
		if w.key == key {
			return w.name
		}
	}
	return key
}

// RegisterBackgroundWorkers records the tick-loop workers as heartbeat entries so
// they're visible in the jobs registry from startup (each worker refreshes its
// last-tick via jobs.Tick as it loops). Call once before starting the workers.
func (h *API) RegisterBackgroundWorkers() {
	for _, w := range backgroundWorkers {
		h.jobs.RegisterWorkerType(w.key, w.name, w.typ, w.intervalSec)
	}
}

// Worker operation keys — what a tick loop is doing right now. The client renders
// them from its own catalog; workerOps keeps the Russian wording as a fallback for
// anything reading the API (or the supervisor log) without that catalog.
const (
	opSyncScan    = "sync_scan"
	opWebhookScan = "webhook_scan"
	opWriteback   = "writeback"
	opDelivery    = "delivery"
	opDueScan     = "due_scan"
	opRecurrence  = "recurrence"

	opRecordingSweep = "recording_sweep"
	opTrashPurge     = "trash_purge"
)

// #nosec G101 -- Russian worker labels shown in the admin UI; the entropy check
// misreads them as a secret
var workerOps = map[string]string{
	opSyncScan:       "проверка интеграций к синхронизации",
	opWebhookScan:    "проверка событий вебхука GitLab",
	opWriteback:      "выгрузка изменений в GitLab",
	opDelivery:       "рассылка уведомлений",
	opDueScan:        "проверка сроков и напоминаний",
	opRecurrence:     "продвижение повторяющихся задач",
	opRecordingSweep: "проверка записей конференций и уборка по сроку",
	opTrashPurge:     "окончательное удаление досок и колонок (через сутки), задач и документов из корзины (через 30 дней), уборка старых адресов проектов, истории фоновых заданий (через 30 дней) и файлов экспорта (через 7 дней)",
}

// tick refreshes a worker's heartbeat; a thin wrapper so worker loops don't import
// the jobs package directly. opKey is a key from workerOps — the rendered wording is
// looked up here so a loop never spells it out.
func (h *API) tick(key, opKey string) { h.jobs.Tick(key, opKey, workerOps[opKey]) }

// runDetached starts background work that outlives the request that asked for
// it, inside the process's background scope (WireBackgroundScope): the run gets
// the shutdown context the tick loops get, and shutdown waits for it before the
// pool closes. Panics are reported rather than taking the process down, the same
// as in superviseWorker.
//
// Reports false — having started nothing — once shutdown is under way. The
// caller has a lock to give back and an answer to write, and "202, started" for
// a run that will be cancelled before its first query is the kind of report that
// sends an operator looking for its results.
func (h *API) runDetached(name string, fn func(context.Context)) bool {
	ctx := h.bgCtx
	if ctx == nil {
		// Unwired: no process lifecycle to join (the test harnesses). Keep the
		// old behaviour rather than refusing to run at all.
		ctx = context.Background()
	} else if ctx.Err() != nil {
		return false
	}
	// Add before the goroutine, not inside it: shutdown may reach Wait while the
	// goroutine is still being scheduled, and a counter raised afterwards is a
	// wait that already returned. Safe here because the HTTP server drains first,
	// so the request that registered this has finished before Wait is called.
	if h.bgWG != nil {
		h.bgWG.Add(1)
	}
	go func() {
		if h.bgWG != nil {
			defer h.bgWG.Done()
		}
		defer observability.Recover(name)
		fn(ctx)
	}()
	return true
}

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
