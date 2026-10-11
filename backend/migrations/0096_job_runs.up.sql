-- Background jobs history (#3067): one row per discrete run — a GitLab sync, an
-- on-demand worker pass, an export or an import — kept 30 days for the jobs
-- window. The live state (progress, the full log ring) stays in the process's
-- registry; a row carries what is worth keeping once the run is over: its steps,
-- the last lines of its log, its attempts and its outcome.
--
-- Auto runs that did nothing (an incremental sync with no changes) keep no row,
-- like the GitLab journal's empty runs, or the table would grow by a row per tick.
CREATE TABLE job_runs (
    id             uuid PRIMARY KEY,
    job_key        text        NOT NULL,
    kind           text        NOT NULL,
    type           text        NOT NULL DEFAULT '',
    name           text        NOT NULL DEFAULT '',
    name_key       text        NOT NULL DEFAULT '',
    name_arg       text        NOT NULL DEFAULT '',
    workspace_id   uuid        REFERENCES workspaces(id) ON DELETE SET NULL,
    actor_id       uuid        REFERENCES users(id) ON DELETE SET NULL,
    trigger        text        NOT NULL DEFAULT '',
    status         text        NOT NULL,
    attempt        integer     NOT NULL DEFAULT 1,
    max_attempts   integer     NOT NULL DEFAULT 1,
    progress_done  integer     NOT NULL DEFAULT 0,
    progress_total integer     NOT NULL DEFAULT 0,
    created_count  integer     NOT NULL DEFAULT 0,
    updated_count  integer     NOT NULL DEFAULT 0,
    steps          jsonb       NOT NULL DEFAULT '[]',
    log_tail       jsonb       NOT NULL DEFAULT '[]',
    log_lines      integer     NOT NULL DEFAULT 0,
    log_warns      integer     NOT NULL DEFAULT 0,
    log_errors     integer     NOT NULL DEFAULT 0,
    meta           jsonb       NOT NULL DEFAULT '{}',
    error          text        NOT NULL DEFAULT '',
    queued_at      timestamptz NOT NULL DEFAULT now(),
    started_at     timestamptz,
    finished_at    timestamptz
);

-- The window reads newest first within the retention period, optionally by type.
CREATE INDEX idx_job_runs_queued ON job_runs (queued_at DESC);
CREATE INDEX idx_job_runs_type ON job_runs (type, queued_at DESC);
-- A member reads their own exports and imports.
CREATE INDEX idx_job_runs_actor ON job_runs (actor_id, queued_at DESC);
