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
