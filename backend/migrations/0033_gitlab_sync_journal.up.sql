-- GitLab sync journal: a user-visible history of what the integration did, in both
-- directions. Pull runs (GitLab → DB) and push runs (DB → GitLab) each get a run
-- row; every record touched within a run gets an action row carrying a before/after
-- detail for the diff view. Additive — safe on the live DB.
--
-- Loose coupling (see CLAUDE.md): the journal lives entirely in its own tables,
-- written only by the GitLab handlers/workers. The core task/board code is untouched;
-- the `detail` JSONB uses a provider-neutral {fields, tags, comments, ...} shape.

-- One row per worker invocation (a manual/auto pull, or a push-drain cycle).
CREATE TABLE gitlab_sync_runs (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    integration_id uuid NOT NULL REFERENCES gitlab_integrations(id) ON DELETE CASCADE,
    kind           text NOT NULL,                       -- 'pull' | 'push'
    trigger        text NOT NULL DEFAULT 'auto',        -- 'manual' | 'auto'
    actor_id       uuid REFERENCES users(id) ON DELETE SET NULL, -- who triggered (null=system)
    status         text NOT NULL DEFAULT 'ok',          -- 'ok' | 'partial' | 'error'
    created_count  integer NOT NULL DEFAULT 0,
    updated_count  integer NOT NULL DEFAULT 0,
    deleted_count  integer NOT NULL DEFAULT 0,
    action_count   integer NOT NULL DEFAULT 0,
    error          text NOT NULL DEFAULT '',
    started_at     timestamptz NOT NULL DEFAULT now(),
    finished_at    timestamptz
);
CREATE INDEX idx_gitlab_sync_runs_integration ON gitlab_sync_runs (integration_id, started_at DESC);

-- One row per record touched within a run, with the before/after diff in `detail`.
-- task_id is SET NULL on task deletion so the historical entry survives.
CREATE TABLE gitlab_sync_actions (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id      uuid NOT NULL REFERENCES gitlab_sync_runs(id) ON DELETE CASCADE,
    seq         integer NOT NULL,                        -- order within the run
    direction   text NOT NULL,                           -- 'pull' (GL→DB) | 'push' (DB→GL)
    entity_type text NOT NULL,                           -- task|subtask|tag|assignee|comment|state|priority
    op          text NOT NULL,                           -- create|update|delete|push
    task_id     uuid REFERENCES tasks(id) ON DELETE SET NULL,
    gl_iid      bigint,
    summary     text NOT NULL DEFAULT '',
    detail      jsonb NOT NULL DEFAULT '{}'::jsonb,      -- before/after (pull) or payload/result (push)
    status      text NOT NULL DEFAULT 'ok',              -- 'ok' | 'fail'
    error       text NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_gitlab_sync_actions_run ON gitlab_sync_actions (run_id, seq);
