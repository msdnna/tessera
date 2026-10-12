-- GitLab milestone links (B/M1): map a native milestone to a GitLab project
-- milestone. Presence of a row makes the milestone GitLab-sourced. Since #3010 it
-- can be edited in Tessera too (written back when the binding allows writing),
-- with gl_snapshot as the three-way baseline. Loosely coupled — its own table,
-- separate from `milestones`.

-- name: GetGitlabMilestoneLinkByGID :one
SELECT * FROM gitlab_milestone_links WHERE integration_id = $1 AND gl_global_id = $2;

-- name: GetGitlabMilestoneLink :one
SELECT * FROM gitlab_milestone_links WHERE milestone_id = $1;

-- name: CreateGitlabMilestoneLink :exec
INSERT INTO gitlab_milestone_links (
    milestone_id, integration_id, gl_global_id, gl_iid, gl_numeric_id, gl_web_url, gl_state, title_hash,
    gl_snapshot
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: UpdateGitlabMilestoneLink :exec
UPDATE gitlab_milestone_links
SET gl_iid = $2, gl_numeric_id = $3, gl_web_url = $4, gl_state = $5, title_hash = $6, last_synced_at = now()
WHERE milestone_id = $1;

-- MarkGitlabMilestoneOverridden flags that the user manually changed a linked task's
-- milestone, so the pull won't overwrite it (mirrors due/start/estimate overrides).
-- name: MarkGitlabMilestoneOverridden :exec
UPDATE gitlab_links SET milestone_overridden = true WHERE task_id = $1;

-- ── write-back and conflicts (#3010) ─────────────────────────────

-- SetGitlabMilestoneSnapshot moves the three-way baseline: what GitLab holds
-- after a successful push or pull.
-- name: SetGitlabMilestoneSnapshot :exec
UPDATE gitlab_milestone_links SET gl_snapshot = $2, last_synced_at = now() WHERE milestone_id = $1;

-- SetGitlabMilestonePushResult records how the last write to GitLab went: an
-- empty error clears the «not synced» mark, a non-empty one sets it with the
-- fields that did not make it and the user to retry as.
-- name: SetGitlabMilestonePushResult :exec
UPDATE gitlab_milestone_links
SET push_error = $2, push_fields = $3, push_actor_id = $4
WHERE milestone_id = $1;

-- SetGitlabMilestoneConflict parks (or, with '{}', clears) the fields changed on
-- both sides since the baseline.
-- name: SetGitlabMilestoneConflict :exec
UPDATE gitlab_milestone_links SET conflict = $2 WHERE milestone_id = $1;

-- ListPendingGitlabMilestonePushes names the binding's stages whose last write
-- failed — the sync retries them after its pull.
-- name: ListPendingGitlabMilestonePushes :many
SELECT * FROM gitlab_milestone_links
WHERE integration_id = $1 AND push_error <> ''
ORDER BY last_synced_at;

-- ListOpenMilestoneConflictsByWorkspace feeds the conflicts window: every stage
-- of the workspace with a parked conflict, with what the window shows of it.
-- name: ListOpenMilestoneConflictsByWorkspace :many
SELECT l.milestone_id, l.conflict, l.gl_iid, l.gl_web_url,
       m.title AS milestone_title, m.slug AS milestone_slug,
       p.id AS project_id, p.slug AS project_slug
FROM gitlab_milestone_links l
JOIN milestones m ON m.id = l.milestone_id
JOIN projects p ON p.id = m.project_id
WHERE p.workspace_id = $1 AND l.conflict <> '{}'::jsonb
ORDER BY m.title;
