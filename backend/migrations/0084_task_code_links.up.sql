-- Merge requests and their pipelines in a task's «Разработка» (#2964).
--
-- task_code_links is the provider-neutral seam: a code change (today a GitLab
-- merge request) that refers to a task, with its review state and the status of
-- its latest CI run. Nothing in the core table names GitLab — `provider` says
-- where the row came from and `source_id` is that provider's binding (for GitLab:
-- gitlab_integrations.id), which the provider package cleans up itself; there is
-- deliberately no FK into a provider table here.
--
-- One row per (task, change): an MR that closes two issues mirrored as two tasks
-- is two rows carrying the same change data. Additive — safe on the live DB.
CREATE TABLE task_code_links (
    task_id           uuid        NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    provider          text        NOT NULL,                       -- gitlab
    source_id         uuid        NOT NULL,                       -- provider binding the change came through
    kind              text        NOT NULL DEFAULT 'merge_request',
    number            bigint      NOT NULL,                       -- per-repository number: the 418 of !418
    external_id       text        NOT NULL DEFAULT '',            -- provider-global id of the change
    repo              text        NOT NULL DEFAULT '',            -- repository path, e.g. group/project
    url               text        NOT NULL DEFAULT '',
    title             text        NOT NULL DEFAULT '',
    state             text        NOT NULL DEFAULT 'open',        -- open | merged | closed
    draft             boolean     NOT NULL DEFAULT false,
    source_branch     text        NOT NULL DEFAULT '',
    target_branch     text        NOT NULL DEFAULT '',
    ci_status         text        NOT NULL DEFAULT '',            -- '' | success | running | failed | canceled | manual
    ci_url            text        NOT NULL DEFAULT '',
    ci_run_id         bigint      NOT NULL DEFAULT 0,             -- newest CI run seen; an older one never overwrites it
    via               text        NOT NULL DEFAULT 'mention',     -- closes | mention | branch: how the change names the task
    remote_updated_at timestamptz,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (task_id, provider, source_id, kind, number)
);

-- A change event names the change, not the task: find every task it is linked to.
CREATE INDEX idx_task_code_links_change ON task_code_links (provider, source_id, kind, number);

-- Cursor of the merge-request poll, separate from the issue pull's last_synced_at:
-- the two are fetched by different queries and one failing must not move the other.
ALTER TABLE gitlab_integrations ADD COLUMN mr_synced_at timestamptz;
