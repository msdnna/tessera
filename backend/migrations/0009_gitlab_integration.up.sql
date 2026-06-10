-- GitLab integration (Phase A, pull-only): mirror GitLab work items assigned to
-- a user from a self-hosted GitLab into a Tessera board. Additive — safe on the
-- live DB.

-- Per-user GitLab credential. The Personal Access Token is stored encrypted
-- (AES-256-GCM, base64 in token_enc) together with the GitLab identity it
-- authenticates as (resolved at connect time, used to query "assigned to me").
CREATE TABLE gitlab_credentials (
    user_id     uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    base_url    text NOT NULL,             -- self-hosted GitLab root, e.g. https://gl.example.com
    token_enc   text NOT NULL,             -- AES-256-GCM(PAT), base64
    gl_user_id  bigint NOT NULL DEFAULT 0, -- numeric GitLab user id
    gl_username text NOT NULL DEFAULT '',  -- used as the assignee filter
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

-- Per-workspace integration config: which GitLab project mirrors into which
-- board, and the label rule engine (status->column, priority->priority,
-- others->tags). One integration per workspace for the pull-only slice.
CREATE TABLE gitlab_integrations (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    project_path text NOT NULL,            -- GitLab project full path, e.g. group/project
    board_id     uuid NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
    label_rules  jsonb NOT NULL DEFAULT '{}'::jsonb,
    enabled      boolean NOT NULL DEFAULT true,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id)
);

-- Mapping between a Tessera task and its GitLab work item. The *_hash columns
-- snapshot the last synced field values; unused in the pull-only slice but
-- present so a future two-way sync can detect real diffs and ignore its own
-- echoes without a schema change.
CREATE TABLE gitlab_links (
    task_id         uuid PRIMARY KEY REFERENCES tasks(id) ON DELETE CASCADE,
    integration_id  uuid NOT NULL REFERENCES gitlab_integrations(id) ON DELETE CASCADE,
    gl_global_id    text NOT NULL,         -- GraphQL global id, e.g. gid://gitlab/Issue/123
    gl_iid          bigint NOT NULL,       -- per-project iid (#N)
    gl_project_path text NOT NULL,
    gl_web_url      text NOT NULL DEFAULT '',
    gl_updated_at   timestamptz,           -- remote updatedAt at last sync
    title_hash      text NOT NULL DEFAULT '',
    desc_hash       text NOT NULL DEFAULT '',
    labels_hash     text NOT NULL DEFAULT '',
    last_synced_at  timestamptz NOT NULL DEFAULT now(),
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (integration_id, gl_global_id)
);
CREATE INDEX idx_gitlab_links_integration ON gitlab_links (integration_id);
