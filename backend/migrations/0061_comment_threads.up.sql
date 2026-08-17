-- Comment threads: a reply points at the root comment of its thread, and the
-- thread is mirrored to the GitLab discussion it belongs to.
--
-- ON DELETE SET NULL, not CASCADE: deleting a root must not silently erase the
-- replies other people wrote under it. The handler promotes the oldest reply to
-- root instead; the FK is only here so a dangling parent_id cannot exist.
ALTER TABLE task_comments ADD COLUMN parent_id uuid REFERENCES task_comments(id) ON DELETE SET NULL;
ALTER TABLE task_comments ADD COLUMN gl_discussion_id text NOT NULL DEFAULT '';

CREATE INDEX idx_task_comments_parent ON task_comments (parent_id) WHERE parent_id IS NOT NULL;
CREATE INDEX idx_task_comments_discussion ON task_comments (task_id, gl_discussion_id) WHERE gl_discussion_id <> '';
