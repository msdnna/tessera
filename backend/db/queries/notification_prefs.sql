-- Per-user scheduling prefs + the due/reminder scanner's queries (Phase B).

-- name: GetNotificationPrefs :one
SELECT * FROM notification_prefs WHERE user_id = $1;

-- name: UpsertNotificationPrefs :one
INSERT INTO notification_prefs (
    user_id, due_enabled, due_lead_minutes, due_repeat_minutes, reminder_enabled,
    quiet_enabled, quiet_start_minutes, quiet_end_minutes, quiet_tz, digest_minutes, updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now())
ON CONFLICT (user_id) DO UPDATE
SET due_enabled = EXCLUDED.due_enabled,
    due_lead_minutes = EXCLUDED.due_lead_minutes,
    due_repeat_minutes = EXCLUDED.due_repeat_minutes,
    reminder_enabled = EXCLUDED.reminder_enabled,
    quiet_enabled = EXCLUDED.quiet_enabled,
    quiet_start_minutes = EXCLUDED.quiet_start_minutes,
    quiet_end_minutes = EXCLUDED.quiet_end_minutes,
    quiet_tz = EXCLUDED.quiet_tz,
    digest_minutes = EXCLUDED.digest_minutes,
    updated_at = now()
RETURNING *;

-- ListDueTasksForScan returns incomplete, non-archived tasks with a due date in a
-- bounded window around now (covers leads up to ~31 days and caps overdue nagging
-- at 7 days), so the scanner doesn't walk the whole table.
-- name: ListDueTasksForScan :many
SELECT * FROM tasks
WHERE due_date IS NOT NULL
  AND completed_at IS NULL
  AND archived_at IS NULL
  AND due_date <= now() + interval '31 days'
  AND due_date >= now() - interval '7 days';

-- name: GetDueNotificationState :one
SELECT * FROM due_notification_state WHERE task_id = $1 AND user_id = $2;

-- name: UpsertDueNotificationState :exec
INSERT INTO due_notification_state (task_id, user_id, fired_due, last_fired_at)
VALUES ($1, $2, $3, now())
ON CONFLICT (task_id, user_id) DO UPDATE
SET fired_due = EXCLUDED.fired_due, last_fired_at = now();

-- name: ListDueReminders :many
SELECT * FROM reminders WHERE NOT done AND notified_at IS NULL AND remind_at <= now();

-- name: MarkReminderNotified :exec
UPDATE reminders SET notified_at = now() WHERE id = $1;
