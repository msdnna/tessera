package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"tessera/internal/db"
	"tessera/internal/jobs"
	"tessera/internal/realtime"
)

// ── Jobs history, publishing and retention (#3067) ─────────────────────────
// The registry keeps live state in memory; the hooks here give it a durable
// history (job_runs, migration 0096) and a live feed over the realtime hub.

// jobHistoryRetention is how long finished runs stay in the jobs window.
const jobHistoryRetention = 30 * 24 * time.Hour

// exportFileRetention is how long an export's file stays downloadable.
const exportFileRetention = 7 * 24 * time.Hour

// exportDir holds the files of finished exports (#3068), named by run id.
func (h *API) exportDir() string { return filepath.Join(h.uploadDir, "exports") }

// jobStoreTimeout bounds one history write: a slow database must not hold a run's
// goroutine (or a shutdown) for long.
const jobStoreTimeout = 5 * time.Second

// wireJobs installs the registry's hooks. Called once from NewAPI.
func (h *API) wireJobs() {
	h.jobs.SetStore(h.saveJobRun)
	h.jobs.SetTypeOf(func(e jobs.Entry) string {
		switch {
		case strings.HasPrefix(e.NameKey, "gitlab"):
			return jobs.TypeGitlab
		case e.Kind == jobs.KindExport || e.Kind == jobs.KindImport:
			return jobs.TypeImportExport
		}
		return jobs.TypeMaintenance
	})
	if v, err := strconv.Atoi(os.Getenv("JOBS_CONCURRENCY")); err == nil && v > 0 {
		h.jobs.SetConcurrency(v)
	}
}

// saveJobRun is the registry's store hook: it upserts the run's row. Best-effort —
// a failed write is logged, the run goes on.
func (h *API) saveJobRun(e jobs.Entry, tail []jobs.LogLine) {
	id, err := uuid.Parse(e.RunID)
	if err != nil || h.q == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), jobStoreTimeout)
	defer cancel()
	steps, _ := json.Marshal(nonNilSteps(e.Steps))
	if tail == nil {
		tail = []jobs.LogLine{}
	}
	logTail, _ := json.Marshal(tail)
	meta := []byte("{}")
	if len(e.Meta) > 0 {
		if b, merr := json.Marshal(e.Meta); merr == nil {
			meta = b
		}
	}
	queued := time.Now()
	if e.QueuedAt != nil {
		queued = *e.QueuedAt
	}
	if err := h.q.SaveJobRun(ctx, db.SaveJobRunParams{
		ID: id, JobKey: e.Key, Kind: e.Kind, Type: e.Type, Name: e.Name, NameKey: e.NameKey, NameArg: e.NameArg,
		WorkspaceID: optUUID(e.WorkspaceID), ActorID: optUUID(e.ActorID), Trigger: e.Trigger,
		Status: string(e.Status), Attempt: int32(e.Attempt), MaxAttempts: int32(e.MaxAttempts),
		ProgressDone: int32(e.ProgressDone), ProgressTotal: int32(e.ProgressTotal),
		CreatedCount: int32(e.Created), UpdatedCount: int32(e.Updated),
		Steps: steps, LogTail: logTail, LogLines: int32(e.LogLines), LogWarns: int32(e.LogWarns), LogErrors: int32(e.LogErrors),
		Meta: meta, Error: e.Error, QueuedAt: queued, StartedAt: e.StartedAt, FinishedAt: e.FinishedAt,
	}); err != nil {
		slog.Warn("jobs: save run", "run", e.RunID, "err", err)
	}
}

func nonNilSteps(s []jobs.Step) []jobs.Step {
	if s == nil {
		return []jobs.Step{}
	}
	return s
}

func optUUID(s string) *uuid.UUID {
	if s == "" {
		return nil
	}
	id, err := uuid.Parse(s)
	if err != nil || id == uuid.Nil {
		return nil
	}
	return &id
}

// FailStaleJobRuns closes history rows a previous process left queued or running.
// Call once at startup, before any job starts.
func (h *API) FailStaleJobRuns(ctx context.Context) {
	if err := h.q.FailStaleJobRuns(ctx); err != nil {
		slog.Warn("jobs: fail stale runs", "err", err)
	}
}

// purgeJobHistory drops runs past the retention period and export files past
// theirs. Part of the trash_purge sweep.
func (h *API) purgeJobHistory(ctx context.Context, now time.Time) {
	if _, err := h.q.PurgeJobRuns(ctx, now.Add(-jobHistoryRetention)); err != nil {
		slog.Warn("jobs: purge history", "err", err)
	}
	dir := h.exportDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, de := range entries {
		info, ierr := de.Info()
		if ierr != nil || de.IsDir() || now.Sub(info.ModTime()) < exportFileRetention {
			continue
		}
		_ = os.Remove(filepath.Join(dir, de.Name()))
	}
}

// jobEvent is the payload of a `job.updated` socket event: the run's state plus
// the log lines added since the previous event of the same run.
type jobEvent struct {
	Job   jobDTO         `json:"job"`
	Lines []jobs.LogLine `json:"lines,omitempty"`
}

// jobPublishCoalesce is how long the publisher lets changes pile up before it
// sends them — a run reporting every row would otherwise send an event per row.
const jobPublishCoalesce = 400 * time.Millisecond

// RunJobPublisher fans job changes out over the realtime hub: every change to the
// global admins' sockets (the jobs window) and, for a run started by a member in a
// workspace (an export, an import), to that member's sockets too. Blocks until ctx
// is done; start it in a goroutine.
func (h *API) RunJobPublisher(ctx context.Context) {
	prune := time.NewTicker(time.Minute)
	defer prune.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-prune.C:
			h.publishJobUpdates()
		case <-h.jobs.Changed():
			t := time.NewTimer(jobPublishCoalesce)
			select {
			case <-ctx.Done():
				t.Stop()
				return
			case <-t.C:
			}
			h.publishJobUpdates()
		}
	}
}

func (h *API) publishJobUpdates() {
	for _, u := range h.jobs.TakeUpdates() {
		ev := jobEvent{Job: entryToDTO(u.Entry), Lines: u.Lines}
		data, err := json.Marshal(ev)
		if err != nil {
			continue
		}
		if h.hub == nil {
			continue
		}
		h.hub.Broadcast(realtime.Event{Type: "job.updated", Data: data, Admin: true})
		ws, actor := optUUID(u.Entry.WorkspaceID), optUUID(u.Entry.ActorID)
		if ws != nil && actor != nil && (u.Entry.Kind == jobs.KindExport || u.Entry.Kind == jobs.KindImport) {
			h.hub.Broadcast(realtime.Event{Scope: ws.String(), Type: "job.updated", Data: data, User: *actor})
		}
	}
}
