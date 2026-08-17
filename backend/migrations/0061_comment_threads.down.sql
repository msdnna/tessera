DROP INDEX IF EXISTS idx_task_comments_discussion;
DROP INDEX IF EXISTS idx_task_comments_parent;
ALTER TABLE task_comments DROP COLUMN IF EXISTS gl_discussion_id;
ALTER TABLE task_comments DROP COLUMN IF EXISTS parent_id;
