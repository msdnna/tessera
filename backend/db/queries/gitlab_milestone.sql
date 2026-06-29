-- GitLab milestone links (B/M1): map a native milestone to a GitLab project
-- milestone. Presence of a row makes the milestone GitLab-sourced (read-only in
-- Tessera). Loosely coupled — its own table, separate from `milestones`.

-- name: GetGitlabMilestoneLinkByGID :one
SELECT * FROM gitlab_milestone_links WHERE integration_id = $1 AND gl_global_id = $2;

-- name: GetGitlabMilestoneLink :one
SELECT * FROM gitlab_milestone_links WHERE milestone_id = $1;

-- name: CreateGitlabMilestoneLink :exec
INSERT INTO gitlab_milestone_links (
    milestone_id, integration_id, gl_global_id, gl_iid, gl_numeric_id, gl_web_url, gl_state, title_hash
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: UpdateGitlabMilestoneLink :exec
UPDATE gitlab_milestone_links
SET gl_iid = $2, gl_numeric_id = $3, gl_web_url = $4, gl_state = $5, title_hash = $6, last_synced_at = now()
WHERE milestone_id = $1;

-- MarkGitlabMilestoneOverridden flags that the user manually changed a linked task's
-- milestone, so the pull won't overwrite it (mirrors due/start/estimate overrides).
-- name: MarkGitlabMilestoneOverridden :exec
UPDATE gitlab_links SET milestone_overridden = true WHERE task_id = $1;
