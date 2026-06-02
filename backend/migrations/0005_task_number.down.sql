DROP INDEX IF EXISTS idx_tasks_number;
ALTER TABLE tasks DROP COLUMN IF EXISTS number;
ALTER TABLE workspaces DROP COLUMN IF EXISTS task_counter;
