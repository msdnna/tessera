-- Откат 0081: поля «Входящих» у заметок и напоминаний.
DROP INDEX IF EXISTS idx_reminders_task;
ALTER TABLE reminders DROP COLUMN IF EXISTS workspace_id;
ALTER TABLE reminders DROP COLUMN IF EXISTS snoozed_from;
ALTER TABLE reminders DROP COLUMN IF EXISTS done_at;
ALTER TABLE reminders DROP COLUMN IF EXISTS body;
DROP INDEX IF EXISTS idx_notes_task;
ALTER TABLE notes DROP CONSTRAINT IF EXISTS notes_visibility_check;
ALTER TABLE notes DROP COLUMN IF EXISTS visibility;
ALTER TABLE notes DROP COLUMN IF EXISTS task_id;
