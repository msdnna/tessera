-- name: CreateReminder :one
INSERT INTO reminders (user_id, task_id, remind_at, message)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListReminders :many
SELECT * FROM reminders WHERE user_id = $1 ORDER BY remind_at;

-- name: GetReminder :one
SELECT * FROM reminders WHERE id = $1;

-- name: UpdateReminder :one
-- Moving a reminder to a new time re-arms channel delivery (notified_at back to
-- NULL), the way due_notification_state re-arms when a task's due date moves.
-- Editing only the message or the done flag keeps the fired mark, so what already
-- went out is not sent a second time.
UPDATE reminders
SET remind_at   = $2,
    message     = $3,
    done        = $4,
    notified_at = CASE WHEN remind_at IS DISTINCT FROM $2 THEN NULL ELSE notified_at END
WHERE id = $1
RETURNING *;

-- name: DeleteReminder :exec
DELETE FROM reminders WHERE id = $1;
