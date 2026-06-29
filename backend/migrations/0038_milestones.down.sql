DROP INDEX IF EXISTS idx_tasks_milestone;
ALTER TABLE tasks DROP COLUMN IF EXISTS milestone_id;
DROP TABLE IF EXISTS milestones;
