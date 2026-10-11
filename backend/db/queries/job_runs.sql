-- Background jobs history (#3067, migration 0096). Written by the jobs registry's
-- store hook as a run starts, changes state and finishes; read by the jobs window.

-- SaveJobRun writes a run's current state, creating the row on first sight. The
-- registry calls it from the run's own goroutine, so writes of one run arrive in
-- order.
-- name: SaveJobRun :exec
INSERT INTO job_runs (
    id, job_key, kind, type, name, name_key, name_arg, workspace_id, actor_id, trigger,
    status, attempt, max_attempts, progress_done, progress_total, created_count, updated_count,
    steps, log_tail, log_lines, log_warns, log_errors, meta, error, queued_at, started_at, finished_at
) VALUES (
    @id, @job_key, @kind, @type, @name, @name_key, @name_arg, @workspace_id, @actor_id, @trigger,
    @status, @attempt, @max_attempts, @progress_done, @progress_total, @created_count, @updated_count,
    @steps, @log_tail, @log_lines, @log_warns, @log_errors, @meta, @error, @queued_at, @started_at, @finished_at
)
ON CONFLICT (id) DO UPDATE SET
    actor_id = EXCLUDED.actor_id, trigger = EXCLUDED.trigger, status = EXCLUDED.status,
    attempt = EXCLUDED.attempt, max_attempts = EXCLUDED.max_attempts,
    progress_done = EXCLUDED.progress_done, progress_total = EXCLUDED.progress_total,
    created_count = EXCLUDED.created_count, updated_count = EXCLUDED.updated_count,
    steps = EXCLUDED.steps, log_tail = EXCLUDED.log_tail, log_lines = EXCLUDED.log_lines,
    log_warns = EXCLUDED.log_warns, log_errors = EXCLUDED.log_errors, meta = EXCLUDED.meta,
    error = EXCLUDED.error, started_at = EXCLUDED.started_at, finished_at = EXCLUDED.finished_at;

-- ListJobRuns is the window's history: runs queued since the cutoff, newest
-- first, optionally of one type ('' = all). The actor's name rides along for
-- «Запустила Мария С.».
-- name: ListJobRuns :many
SELECT r.*, COALESCE(u.name, '')::text AS actor_name
FROM job_runs r
LEFT JOIN users u ON u.id = r.actor_id
WHERE r.queued_at >= @since
  AND (@type::text = '' OR r.type = @type::text)
ORDER BY r.queued_at DESC
LIMIT @lim;

-- CountJobRunsByType backs the type filter's counters over the same window.
-- name: CountJobRunsByType :many
SELECT type, count(*)::int AS n FROM job_runs WHERE queued_at >= @since GROUP BY type;

-- name: GetJobRun :one
SELECT r.*, COALESCE(u.name, '')::text AS actor_name
FROM job_runs r
LEFT JOIN users u ON u.id = r.actor_id
WHERE r.id = $1;

-- FailStaleJobRuns closes runs a previous process left queued or running — a
-- restart drops the in-memory registry, so nothing will ever finish them. Called
-- once at startup, before anything of ours runs.
-- name: FailStaleJobRuns :exec
UPDATE job_runs
SET status = 'failed', error = 'прервано перезапуском сервиса', finished_at = now()
WHERE finished_at IS NULL;

-- PurgeJobRuns drops history past the retention period.
-- name: PurgeJobRuns :execrows
DELETE FROM job_runs WHERE queued_at < @before;
