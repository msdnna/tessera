-- GitLab milestone mapping (B/M1): link a native Tessera milestone to a GitLab
-- project milestone, and remember when the user overrode a task's milestone so the
-- pull doesn't clobber it. Loose coupling: the GitLab specifics live here, the core
-- `milestones`/`tasks` tables stay provider-neutral. Additive.

CREATE TABLE gitlab_milestone_links (
    milestone_id   uuid PRIMARY KEY REFERENCES milestones(id) ON DELETE CASCADE,
    integration_id uuid NOT NULL REFERENCES gitlab_integrations(id) ON DELETE CASCADE,
    gl_global_id   text NOT NULL,                  -- gid://gitlab/Milestone/<n>
    gl_iid         bigint,                          -- nullable (older GitLab has none)
    gl_numeric_id  bigint NOT NULL,                 -- for REST PUT /issues/:iid?milestone_id=
    gl_web_url     text NOT NULL DEFAULT '',
    gl_state       text NOT NULL DEFAULT '',        -- active | closed
    title_hash     text NOT NULL DEFAULT '',
    last_synced_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (integration_id, gl_global_id)
);

-- Presence of a link row marks a milestone as GitLab-sourced (read-only in Tessera).
ALTER TABLE gitlab_links ADD COLUMN milestone_overridden boolean NOT NULL DEFAULT false;
