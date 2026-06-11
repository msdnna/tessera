-- GitLab sync fidelity: external GitLab users (assignees/comment authors that
-- aren't Tessera users) and provenance markers so the pull can reconcile its own
-- tags/assignees without clobbering manually-applied ones. All additive.

-- External GitLab assignees on a task (display-only; a GitLab user with no
-- Tessera account). Assignees whose GitLab username maps to a Tessera user go
-- into task_assignees instead.
CREATE TABLE task_gitlab_assignees (
    task_id     uuid NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    gl_username text NOT NULL,
    gl_name     text NOT NULL DEFAULT '',
    PRIMARY KEY (task_id, gl_username)
);

-- Comments mirrored from GitLab notes. gl_note_id is the source note's global id
-- (unique → idempotent upsert); gl_author_* denormalise the GitLab author since
-- it may not be a Tessera user (author_id stays null then).
ALTER TABLE task_comments ADD COLUMN gl_note_id      text;
ALTER TABLE task_comments ADD COLUMN gl_author_login text NOT NULL DEFAULT '';
ALTER TABLE task_comments ADD COLUMN gl_author_name  text NOT NULL DEFAULT '';
CREATE UNIQUE INDEX idx_task_comments_gl_note ON task_comments (gl_note_id) WHERE gl_note_id IS NOT NULL;

-- Provenance for the sync's mixed reconciliation: 'user' = applied in Tessera
-- (never touched by sync), 'gitlab' = applied by the GitLab sync (reconciled to
-- match the source each run).
ALTER TABLE task_tags ADD COLUMN source text NOT NULL DEFAULT 'user';
ALTER TABLE task_assignees ADD COLUMN source text NOT NULL DEFAULT 'user';
