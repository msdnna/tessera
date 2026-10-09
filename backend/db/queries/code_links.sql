-- ── Code changes linked to a task (#2964) ──────────────────
-- Provider-neutral: the rows are written by an integration (today GitLab merge
-- requests) and read by the task window. See migration 0084.

-- name: ListTaskCodeLinks :many
-- Open changes first, then the most recently active.
SELECT * FROM task_code_links
WHERE task_id = $1
ORDER BY (state = 'open') DESC, remote_updated_at DESC NULLS LAST, number DESC;

-- name: ListCodeLinksForChange :many
-- Every task one change is linked to.
SELECT * FROM task_code_links
WHERE provider = $1 AND source_id = $2 AND kind = $3 AND number = $4;

-- name: UpsertTaskCodeLink :many
-- Inserts or refreshes one (task, change) row and returns it only when something a
-- reader sees actually changed — that return is the cue to tell open clients. A
-- delivery older than what is stored (a late hook after a poll) is ignored. CI
-- fields are written on insert only: they move through SetCodeChangeCI.
INSERT INTO task_code_links (
    task_id, provider, source_id, kind, number, external_id, repo, url, title, state,
    draft, source_branch, target_branch, via, remote_updated_at, ci_status, ci_url, ci_run_id
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18
)
ON CONFLICT (task_id, provider, source_id, kind, number) DO UPDATE SET
    external_id = EXCLUDED.external_id,
    repo = EXCLUDED.repo,
    url = EXCLUDED.url,
    title = EXCLUDED.title,
    state = EXCLUDED.state,
    draft = EXCLUDED.draft,
    source_branch = EXCLUDED.source_branch,
    target_branch = EXCLUDED.target_branch,
    via = EXCLUDED.via,
    remote_updated_at = COALESCE(EXCLUDED.remote_updated_at, task_code_links.remote_updated_at),
    updated_at = now()
WHERE (task_code_links.remote_updated_at IS NULL
       OR EXCLUDED.remote_updated_at IS NULL
       OR EXCLUDED.remote_updated_at >= task_code_links.remote_updated_at)
  AND (task_code_links.external_id, task_code_links.repo, task_code_links.url, task_code_links.title,
       task_code_links.state, task_code_links.draft, task_code_links.source_branch,
       task_code_links.target_branch, task_code_links.via)
      IS DISTINCT FROM
      (EXCLUDED.external_id, EXCLUDED.repo, EXCLUDED.url, EXCLUDED.title, EXCLUDED.state,
       EXCLUDED.draft, EXCLUDED.source_branch, EXCLUDED.target_branch, EXCLUDED.via)
RETURNING task_id;

-- name: SetCodeChangeCI :many
-- Records a CI run for some changes. A run older than the newest one already seen
-- never overwrites it (hooks arrive out of order); the same run moving on
-- (running → success) does. Returns the tasks whose rows changed.
UPDATE task_code_links
SET ci_status = sqlc.arg(ci_status), ci_url = sqlc.arg(ci_url), ci_run_id = sqlc.arg(ci_run_id), updated_at = now()
WHERE provider = sqlc.arg(provider) AND source_id = sqlc.arg(source_id) AND kind = sqlc.arg(kind)
  AND number = ANY(sqlc.arg(numbers)::bigint[])
  AND ci_run_id <= sqlc.arg(ci_run_id)
  AND (ci_status, ci_url, ci_run_id) IS DISTINCT FROM (sqlc.arg(ci_status)::text, sqlc.arg(ci_url)::text, sqlc.arg(ci_run_id)::bigint)
RETURNING task_id;

-- name: DeleteStaleCodeLinks :many
-- The change no longer names these tasks: drop every row of it outside keep.
DELETE FROM task_code_links
WHERE provider = $1 AND source_id = $2 AND kind = $3 AND number = $4
  AND NOT (task_id = ANY(sqlc.arg(keep)::uuid[]))
RETURNING task_id;

-- name: OpenCodeChangesOnBranch :many
-- Open changes coming from a branch — where a branch pipeline lands.
SELECT DISTINCT number FROM task_code_links
WHERE provider = $1 AND source_id = $2 AND kind = $3 AND source_branch = $4 AND state = 'open';

-- name: CodeChangesWithRunningCI :many
-- Open changes whose CI is still on its way to a verdict: the poll re-reads them,
-- because a finished pipeline does not show up as an updated change.
SELECT DISTINCT number FROM task_code_links
WHERE provider = $1 AND source_id = $2 AND kind = $3 AND state = 'open' AND ci_status = 'running'
ORDER BY number DESC
LIMIT 100;

-- name: DeleteCodeLinksForSource :exec
-- The provider binding is gone: its changes go with it.
DELETE FROM task_code_links WHERE provider = $1 AND source_id = $2;
