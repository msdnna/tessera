-- name: CreateReminder :one
INSERT INTO reminders (user_id, task_id, remind_at, message)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListReminders :many
SELECT * FROM reminders WHERE user_id = $1 ORDER BY remind_at;

-- name: GetReminder :one
SELECT * FROM reminders WHERE id = $1;

-- name: UpdateReminder :one
UPDATE reminders
SET remind_at = $2, message = $3, done = $4
WHERE id = $1
RETURNING *;

-- name: DeleteReminder :exec
DELETE FROM reminders WHERE id = $1;
