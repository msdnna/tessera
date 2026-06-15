-- GitLab user avatars on synced tasks (author + external assignees). The GraphQL
-- avatarUrl is stored as-is (usually absolute); clients render it directly and
-- fall back to initials on miss. All additive.
ALTER TABLE gitlab_links ADD COLUMN gl_author_avatar_url text NOT NULL DEFAULT '';
ALTER TABLE task_gitlab_assignees ADD COLUMN gl_avatar_url text NOT NULL DEFAULT '';
