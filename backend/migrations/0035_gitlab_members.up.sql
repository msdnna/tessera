-- Phase B+ step 3: GitLab project members (assignable from Tessera) + writable
-- external assignees. Lets a GitLab project member with no Tessera account be
-- assigned from Tessera and pushed back to the issue. Loosely coupled (own table).

-- Project members fetched from GitLab on each sync, per integration. gl_user_id is
-- the numeric GitLab user id (needed for assignee write-back via assignee_ids[]).
CREATE TABLE gitlab_project_members (
    integration_id uuid   NOT NULL REFERENCES gitlab_integrations(id) ON DELETE CASCADE,
    gl_user_id     bigint NOT NULL,
    gl_username    text   NOT NULL,
    gl_name        text   NOT NULL DEFAULT '',
    gl_avatar_url  text   NOT NULL DEFAULT '',
    access_level   int    NOT NULL DEFAULT 0,
    updated_at     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (integration_id, gl_user_id)
);
CREATE INDEX idx_gitlab_members_username ON gitlab_project_members (integration_id, gl_username);

-- Provenance on external GL assignees so a user-pinned GL member survives the sync
-- rebuild (mirrors task_tags/task_assignees.source). Existing rows are sync-made.
ALTER TABLE task_gitlab_assignees ADD COLUMN source text NOT NULL DEFAULT 'gitlab';
