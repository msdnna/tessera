-- GitLab write-back (phase B): outbox queue + the integration lookup the worker
-- needs. Mirrors the notification delivery outbox (claim / backoff / settle).

-- name: GetGitlabIntegrationByID :one
SELECT * FROM gitlab_integrations WHERE id = $1;

-- name: CreateGitlabWriteback :exec
INSERT INTO gitlab_writebacks (task_id, integration_id, change_kind, payload)
VALUES ($1, $2, $3, $4);

-- CoalescePendingWriteback refreshes an already-pending row for the same task and
-- change_kind (collapsing a burst of edits into one push, latest value wins) and
-- re-arms it for immediate delivery. Returns the rows updated; 0 means the caller
-- should insert a fresh row.
-- name: CoalescePendingWriteback :execrows
UPDATE gitlab_writebacks
SET payload = $3, next_attempt_at = now(), updated_at = now()
WHERE task_id = $1 AND change_kind = $2 AND status = 'pending';

-- ClaimPendingWritebacks atomically grabs up to $1 due pending rows, marking them
-- 'sending' and bumping attempts, so concurrent/queued workers never pick the same
-- row (FOR UPDATE SKIP LOCKED).
-- name: ClaimPendingWritebacks :many
UPDATE gitlab_writebacks
SET status = 'sending', attempts = attempts + 1, updated_at = now()
WHERE id IN (
    SELECT id FROM gitlab_writebacks
    WHERE status = 'pending' AND next_attempt_at <= now()
    ORDER BY next_attempt_at
    LIMIT $1
    FOR UPDATE SKIP LOCKED
)
RETURNING *;

-- name: MarkWritebackSent :exec
UPDATE gitlab_writebacks SET status = 'sent', last_error = '', updated_at = now() WHERE id = $1;

-- MarkWritebackRetry reschedules a transient failure (back to pending, backed-off).
-- name: MarkWritebackRetry :exec
UPDATE gitlab_writebacks
SET status = 'pending', last_error = $2, next_attempt_at = $3, updated_at = now()
WHERE id = $1;

-- name: MarkWritebackFailed :exec
UPDATE gitlab_writebacks SET status = 'failed', last_error = $2, updated_at = now() WHERE id = $1;

-- SetCommentGlNoteID tags a Tessera comment with the GitLab note id created by a
-- write-back push, so the next pull's comment upsert (keyed on gl_note_id) updates
-- that row instead of inserting a duplicate.
-- name: SetCommentGlNoteID :exec
UPDATE task_comments SET gl_note_id = $2, updated_at = now() WHERE id = $1;

-- ── Write-back conflicts ────────────────────────────────────

-- MarkWritebackConflict parks a claimed row as a conflict (both sides changed the
-- same field since the last sync). It stays parked until the user resolves it.
-- name: MarkWritebackConflict :exec
UPDATE gitlab_writebacks
SET status = 'conflict', conflict = $2, updated_at = now()
WHERE id = $1;

-- GetOpenConflict returns the open conflict row for a (task, change_kind), if any.
-- name: GetOpenConflict :one
SELECT * FROM gitlab_writebacks
WHERE task_id = $1 AND change_kind = $2 AND status = 'conflict';

-- RefreshConflict updates an open conflict's desired payload + conflict snapshot
-- when the user edits the same field again before resolving (latest intent wins).
-- name: RefreshConflict :exec
UPDATE gitlab_writebacks
SET payload = $3, conflict = $4, updated_at = now()
WHERE task_id = $1 AND change_kind = $2 AND status = 'conflict';

-- GetGitlabWriteback fetches one outbox row by id (for the resolve endpoint).
-- name: GetGitlabWriteback :one
SELECT * FROM gitlab_writebacks WHERE id = $1;

-- ListOpenConflictKinds lists the change kinds with an open conflict for a task, so
-- the pull can freeze those fields (not overwrite the user's pending value).
-- name: ListOpenConflictKinds :many
SELECT change_kind FROM gitlab_writebacks WHERE task_id = $1 AND status = 'conflict';

-- ListOpenConflicts returns every open conflict for an integration with its task's
-- title/number, newest first — powers the conflicts inbox.
-- name: ListOpenConflicts :many
SELECT w.*, t.title AS task_title, t.number AS task_number
FROM gitlab_writebacks w
JOIN tasks t ON t.id = w.task_id
WHERE w.integration_id = $1 AND w.status = 'conflict'
ORDER BY w.updated_at DESC;

-- ReArmConflict re-queues a resolved conflict for delivery (ours/manual): the
-- worker re-fetches GitLab and pushes the now-acknowledged value. Records the choice.
-- name: ReArmConflict :exec
UPDATE gitlab_writebacks
SET status = 'pending', attempts = 0, last_error = '', next_attempt_at = now(),
    conflict = '{}'::jsonb, resolution = $2, resolved_by = $3, resolved_at = now(),
    updated_at = now()
WHERE id = $1;

-- ResolveConflictSettled closes a conflict with no push (theirs): the task already
-- holds GitLab's value, so nothing is sent.
-- name: ResolveConflictSettled :exec
UPDATE gitlab_writebacks
SET status = 'sent', conflict = '{}'::jsonb, resolution = $2, resolved_by = $3,
    resolved_at = now(), updated_at = now()
WHERE id = $1;
