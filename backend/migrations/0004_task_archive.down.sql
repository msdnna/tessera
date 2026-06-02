DROP INDEX IF EXISTS idx_tasks_archived;
ALTER TABLE tasks DROP COLUMN IF EXISTS archived_at;
