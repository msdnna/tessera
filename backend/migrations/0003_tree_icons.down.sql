DROP INDEX IF EXISTS idx_project_groups_parent;
ALTER TABLE project_groups DROP COLUMN IF EXISTS parent_id;
ALTER TABLE projects DROP COLUMN IF EXISTS icon;
