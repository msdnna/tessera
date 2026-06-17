-- GitLab note author's avatar, so synced comments can show it (like task cards).
ALTER TABLE task_comments ADD COLUMN gl_author_avatar_url text NOT NULL DEFAULT '';
