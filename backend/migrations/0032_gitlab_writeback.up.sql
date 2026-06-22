-- GitLab write-back (phase B): push Tessera-side changes back to the linked
-- GitLab issue. Additive — safe on the live DB.
--
-- Loose coupling (see CLAUDE.md): write-back config lives in its own `writeback`
-- JSONB on gitlab_integrations (separate from the pull-direction `label_rules`),
-- and the queue is its own table — the core task handlers only call a thin enqueue
-- helper. Delivery is async, mirroring notification_deliveries (claim/backoff/retry).

-- Per-integration write-back config (provider-neutral shape, defaults all-off so
-- write-back is strictly opt-in):
--   {enabled, push_state, push_priority, push_comments, push_title_desc,
--    column_label_bindings: {<columnName>: <S: label>}}
ALTER TABLE gitlab_integrations ADD COLUMN writeback jsonb NOT NULL DEFAULT '{}'::jsonb;

-- Last GitLab issue state we observed/pushed ("opened"|"closed"), the baseline for
-- the state loop-guard: never push a state that already matches GitLab. Written by
-- the pull (from issue.state) and after a successful state push.
ALTER TABLE gitlab_links ADD COLUMN gl_last_state text NOT NULL DEFAULT '';

-- Outbox queue: one row per (task, change_kind) we decided to push. The background
-- worker claims due pending rows, calls the GitLab REST API, and either marks them
-- sent or reschedules with backoff (failed after the attempt cap). Decouples the
-- push from the request path (async, retry, survives restart).
CREATE TABLE gitlab_writebacks (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id         uuid NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    integration_id  uuid NOT NULL REFERENCES gitlab_integrations(id) ON DELETE CASCADE,
    change_kind     text NOT NULL,                      -- 'state' | 'priority' | 'comment'
    payload         jsonb NOT NULL DEFAULT '{}'::jsonb, -- kind-specific (e.g. {comment_id}, {old_priority,new_priority})
    status          text NOT NULL DEFAULT 'pending',    -- pending | sending | sent | failed
    attempts        integer NOT NULL DEFAULT 0,
    last_error      text NOT NULL DEFAULT '',
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_gitlab_writebacks_due ON gitlab_writebacks (next_attempt_at) WHERE status = 'pending';
-- Supports coalescing a burst of edits to the same task+kind into one pending row.
CREATE INDEX idx_gitlab_writebacks_pending_task ON gitlab_writebacks (task_id, change_kind) WHERE status = 'pending';
