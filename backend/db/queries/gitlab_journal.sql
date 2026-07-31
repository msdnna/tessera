-- GitLab sync journal: run + action history written by the pull engine and the
-- write-back worker, read back by the journal modal. See migration 0033.

-- CreateGitlabSyncRun opens a run row. started_at is supplied by the caller (the
-- real moment the sync began, not the moment it was recorded), and status is
-- normally 'running' — FinishGitlabSyncRun stamps the outcome afterwards.
-- name: CreateGitlabSyncRun :one
INSERT INTO gitlab_sync_runs (integration_id, kind, trigger, actor_id, status, started_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- FinishGitlabSyncRun stamps the final status, counts and finish time once a run
-- (and all its actions) is recorded.
-- name: FinishGitlabSyncRun :exec
UPDATE gitlab_sync_runs
SET status = $2, created_count = $3, updated_count = $4, deleted_count = $5,
    action_count = $6, error = $7, finished_at = now()
WHERE id = $1;

-- FailStaleGitlabSyncRuns closes runs left in 'running' by a process that died
-- mid-sync (restart/crash). Called once at startup — nothing can legitimately be
-- running at that point, so no in-flight run is clobbered.
-- name: FailStaleGitlabSyncRuns :exec
UPDATE gitlab_sync_runs
SET status = 'error', error = 'прервано перезапуском сервиса', finished_at = now()
WHERE status = 'running' AND finished_at IS NULL;

-- name: CreateGitlabSyncAction :exec
INSERT INTO gitlab_sync_actions (
    run_id, seq, direction, entity_type, op, task_id, gl_iid, summary, detail, status, error
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11);

-- name: ListGitlabSyncRuns :many
SELECT * FROM gitlab_sync_runs
WHERE integration_id = $1
ORDER BY started_at DESC
LIMIT $2;

-- ListGitlabSyncRunsByWorkspace aggregates recent runs across every binding in a
-- workspace (multi-binding journal), newest first.
-- name: ListGitlabSyncRunsByWorkspace :many
SELECT r.* FROM gitlab_sync_runs r
JOIN gitlab_integrations i ON i.id = r.integration_id
WHERE i.workspace_id = $1
ORDER BY r.started_at DESC
LIMIT $2;

-- GetGitlabSyncRun fetches a run scoped to its integration (ownership check).
-- name: GetGitlabSyncRun :one
SELECT * FROM gitlab_sync_runs WHERE id = $1 AND integration_id = $2;

-- GetGitlabSyncRunInWorkspace fetches a run scoped to a workspace (ownership check
-- across all its bindings).
-- name: GetGitlabSyncRunInWorkspace :one
SELECT r.* FROM gitlab_sync_runs r
JOIN gitlab_integrations i ON i.id = r.integration_id
WHERE r.id = $1 AND i.workspace_id = $2;

-- name: ListGitlabSyncActions :many
SELECT * FROM gitlab_sync_actions
WHERE run_id = $1
ORDER BY seq;

-- name: GetGitlabSyncAction :one
SELECT a.* FROM gitlab_sync_actions a
JOIN gitlab_sync_runs r ON r.id = a.run_id
WHERE a.id = $1 AND r.integration_id = $2;

-- GetGitlabSyncActionInWorkspace fetches an action scoped to a workspace (across
-- all its bindings), returning its run's integration_id so a retry can re-enqueue.
-- name: GetGitlabSyncActionInWorkspace :one
SELECT a.*, r.integration_id AS run_integration_id
FROM gitlab_sync_actions a
JOIN gitlab_sync_runs r ON r.id = a.run_id
JOIN gitlab_integrations i ON i.id = r.integration_id
WHERE a.id = $1 AND i.workspace_id = $2;

-- PruneGitlabSyncRuns keeps only the most recent $2 runs of an integration; older
-- runs (and their actions, via ON DELETE CASCADE) are dropped so the journal stays
-- bounded.
-- name: PruneGitlabSyncRuns :exec
DELETE FROM gitlab_sync_runs r
WHERE r.integration_id = $1
  AND r.id NOT IN (
    SELECT s.id FROM gitlab_sync_runs s
    WHERE s.integration_id = $1
    ORDER BY s.started_at DESC
    LIMIT $2
  );
