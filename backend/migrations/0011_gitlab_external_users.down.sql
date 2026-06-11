ALTER TABLE task_assignees DROP COLUMN IF EXISTS source;
ALTER TABLE task_tags DROP COLUMN IF EXISTS source;
DROP INDEX IF EXISTS idx_task_comments_gl_note;
ALTER TABLE task_comments DROP COLUMN IF EXISTS gl_author_name;
ALTER TABLE task_comments DROP COLUMN IF EXISTS gl_author_login;
ALTER TABLE task_comments DROP COLUMN IF EXISTS gl_note_id;
DROP TABLE IF EXISTS task_gitlab_assignees;
