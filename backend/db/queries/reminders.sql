-- name: CreateReminder :one
-- client_id (#3042): see CreateNote.
INSERT INTO reminders (id, user_id, task_id, remind_at, message, body, workspace_id, tags, mentions)
VALUES (COALESCE(sqlc.narg(client_id)::uuid, gen_random_uuid()), $1, $2, $3, $4, $5, $6, @tags::text[], @mentions::uuid[])
RETURNING *;

-- InboxReminders is the reminder half of the «Входящие» feed (#2972): the
-- user's reminders of this workspace plus those outside every workspace (0081:
-- old ones without a task), open ones and the ones done in the last day and a
-- half — the client keeps «done today» struck through until its own midnight.
-- name: InboxReminders :many
SELECT * FROM reminders
WHERE user_id = @user_id
  AND (workspace_id = @workspace_id OR workspace_id IS NULL)
  AND (NOT done OR done_at > now() - interval '36 hours')
ORDER BY remind_at;

-- name: ListTaskReminders :many
SELECT * FROM reminders
WHERE task_id = @task_id AND user_id = @user_id
ORDER BY done, remind_at;

-- name: ListReminders :many
SELECT * FROM reminders WHERE user_id = $1 ORDER BY remind_at;

-- name: GetReminder :one
SELECT * FROM reminders WHERE id = $1;

-- name: UpdateReminder :one
-- Moving a reminder to a new time re-arms channel delivery (notified_at back to
-- NULL), the way due_notification_state re-arms when a task's due date moves.
-- Editing only the message or the done flag keeps the fired mark, so what already
-- went out is not sent a second time. done_at (0081) follows the flag: stamped
-- when it turns on, kept while it stays on, cleared when it turns off.
UPDATE reminders
SET remind_at    = @remind_at,
    message      = @message,
    done         = @done,
    body         = @body,
    task_id      = @task_id,
    workspace_id = @workspace_id,
    snoozed_from = @snoozed_from,
    tags         = @tags::text[],
    mentions     = @mentions::uuid[],
    done_at      = CASE WHEN NOT @done::boolean THEN NULL
                        WHEN done THEN done_at
                        ELSE now() END,
    notified_at  = CASE WHEN remind_at IS DISTINCT FROM @remind_at THEN NULL ELSE notified_at END
WHERE id = @id
RETURNING *;

-- name: DeleteReminder :exec
DELETE FROM reminders WHERE id = $1;
