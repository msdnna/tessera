ALTER TABLE task_gitlab_assignees DROP COLUMN IF EXISTS source;
DROP INDEX IF EXISTS idx_gitlab_members_username;
DROP TABLE IF EXISTS gitlab_project_members;
