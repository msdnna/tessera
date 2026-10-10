-- Per-user scheduling prefs + the due/reminder scanner's queries (Phase B).

-- name: GetNotificationPrefs :one
SELECT * FROM notification_prefs WHERE user_id = $1;

-- name: UpsertNotificationPrefs :one
INSERT INTO notification_prefs (
    user_id, due_enabled, due_lead_minutes, due_repeat_minutes, reminder_enabled,
    quiet_enabled, quiet_start_minutes, quiet_end_minutes, quiet_tz, digest_minutes,
    toasts_enabled, toasts, quiet_toasts, updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, now())
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
    toasts_enabled = EXCLUDED.toasts_enabled,
    toasts = EXCLUDED.toasts,
    quiet_toasts = EXCLUDED.quiet_toasts,
    updated_at = now()
RETURNING *;

-- SetNotificationDnd switches «Не беспокоить» (#3117): until a moment, or off
-- (NULL). A user who never customised gets a row with the column defaults —
-- the same values defaultPrefs answers with.
-- name: SetNotificationDnd :one
INSERT INTO notification_prefs (user_id, dnd_until, updated_at)
VALUES ($1, $2, now())
ON CONFLICT (user_id) DO UPDATE
SET dnd_until = EXCLUDED.dnd_until,
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
  AND deleted_at IS NULL
  AND due_date <= now() + interval '31 days'
  AND due_date >= now() - interval '7 days'
  -- Live board of a live project: a deleted board (0078) and an archived
  -- project (0086) are out of every scanner.
  AND EXISTS (SELECT 1 FROM boards b JOIN projects p ON p.id = b.project_id
              WHERE b.id = tasks.board_id AND b.deleted_at IS NULL AND p.archived_at IS NULL);

-- name: GetDueNotificationState :one
SELECT * FROM due_notification_state WHERE task_id = $1 AND user_id = $2;

-- name: UpsertDueNotificationState :exec
INSERT INTO due_notification_state (task_id, user_id, fired_due, last_fired_at)
VALUES ($1, $2, $3, now())
ON CONFLICT (task_id, user_id) DO UPDATE
SET fired_due = EXCLUDED.fired_due, last_fired_at = now();

-- A reminder pinned to a task fires only while the task is reachable: not in
-- the trash (0086), not on a deleted board (0078), not in an archived project.
-- A task brought back lets the overdue reminder fire on the next pass.
-- name: ListDueReminders :many
SELECT * FROM reminders r
WHERE NOT r.done AND r.notified_at IS NULL AND r.remind_at <= now()
  AND (r.task_id IS NULL OR EXISTS (
        SELECT 1 FROM tasks t
        JOIN boards b ON b.id = t.board_id
        JOIN projects p ON p.id = b.project_id
        WHERE t.id = r.task_id AND t.deleted_at IS NULL
          AND b.deleted_at IS NULL AND p.archived_at IS NULL));

-- name: MarkReminderNotified :exec
UPDATE reminders SET notified_at = now() WHERE id = $1;
