ALTER TABLE task_gitlab_assignees DROP COLUMN IF EXISTS gl_avatar_url;
ALTER TABLE gitlab_links DROP COLUMN IF EXISTS gl_author_avatar_url;
