-- GitLab linked items → Tessera task relations (task #2591). Additive.
--
-- Loose coupling (see CLAUDE.md): the core `task_relations` table only learns a
-- provider-neutral `source` (user|gitlab) — the same seam already used by
-- task_tags/task_assignees. Everything GitLab-shaped (link_type, iids, project
-- paths, the remote link id) lives in its own table here, so a second integration
-- needs no core change.
--
-- The two-step "raw rows → projection" design buys idempotency for free: a sync
-- upserts gitlab_issue_links and only then resolves each row to a task_relations
-- row. A link whose other endpoint has not been imported yet simply stays
-- unresolved and is picked up by a later run.

ALTER TABLE task_relations ADD COLUMN source text NOT NULL DEFAULT 'user';

-- off | pull (two_way is the reserved next step: pushing Tessera relations back).
ALTER TABLE gitlab_integrations ADD COLUMN relations_sync text NOT NULL DEFAULT 'pull';

CREATE TABLE gitlab_issue_links (
    id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    integration_id       uuid NOT NULL REFERENCES gitlab_integrations(id) ON DELETE CASCADE,
    src_project_path     text NOT NULL,
    src_iid              bigint NOT NULL,
    dst_project_path     text NOT NULL,
    dst_iid              bigint NOT NULL,
    link_type            text NOT NULL,            -- relates_to | blocks | is_blocked_by
    gl_link_id           text NOT NULL DEFAULT '', -- the remote link's id, for a future unlink push
    gl_web_url           text NOT NULL DEFAULT '',
    -- The projected core relation. ON DELETE SET NULL is deliberate: when a user
    -- deletes the relation by hand the raw row survives as unresolved, and the next
    -- sync re-creates it (GitLab remains the source of truth for its own links).
    resolved_relation_id uuid REFERENCES task_relations(id) ON DELETE SET NULL,
    last_seen_at         timestamptz NOT NULL DEFAULT now(),
    created_at           timestamptz NOT NULL DEFAULT now(),
    UNIQUE (integration_id, src_project_path, src_iid, dst_project_path, dst_iid, link_type)
);
CREATE INDEX idx_gitlab_issue_links_integration ON gitlab_issue_links (integration_id);
CREATE INDEX idx_gitlab_issue_links_unresolved
    ON gitlab_issue_links (integration_id) WHERE resolved_relation_id IS NULL;
