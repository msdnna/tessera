-- GitLab integration queries (Phase A, pull-only).

-- ── Per-user credential ────────────────────────────────────

-- name: UpsertGitlabCredential :one
INSERT INTO gitlab_credentials (user_id, base_url, token_enc, gl_user_id, gl_username, updated_at)
VALUES ($1, $2, $3, $4, $5, now())
ON CONFLICT (user_id) DO UPDATE
SET base_url = EXCLUDED.base_url,
    token_enc = EXCLUDED.token_enc,
    gl_user_id = EXCLUDED.gl_user_id,
    gl_username = EXCLUDED.gl_username,
    updated_at = now()
RETURNING *;

-- name: GetGitlabCredential :one
SELECT * FROM gitlab_credentials WHERE user_id = $1;

-- name: DeleteGitlabCredential :exec
DELETE FROM gitlab_credentials WHERE user_id = $1;

-- ── Per-workspace integration ──────────────────────────────

-- name: UpsertGitlabIntegration :one
INSERT INTO gitlab_integrations (workspace_id, project_path, board_id, label_rules, enabled, updated_at)
VALUES ($1, $2, $3, $4, $5, now())
ON CONFLICT (workspace_id) DO UPDATE
SET project_path = EXCLUDED.project_path,
    board_id = EXCLUDED.board_id,
    label_rules = EXCLUDED.label_rules,
    enabled = EXCLUDED.enabled,
    updated_at = now()
RETURNING *;

-- name: GetGitlabIntegrationByWorkspace :one
SELECT * FROM gitlab_integrations WHERE workspace_id = $1;

-- ── Task ↔ work item link ──────────────────────────────────

-- name: GetGitlabLinkByGlobalID :one
SELECT * FROM gitlab_links WHERE integration_id = $1 AND gl_global_id = $2;

-- name: CreateGitlabLink :one
INSERT INTO gitlab_links (
    task_id, integration_id, gl_global_id, gl_iid, gl_project_path, gl_web_url,
    gl_updated_at, title_hash, desc_hash, labels_hash
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: UpdateGitlabLink :one
UPDATE gitlab_links
SET gl_iid = $2, gl_web_url = $3, gl_updated_at = $4,
    title_hash = $5, desc_hash = $6, labels_hash = $7, last_synced_at = now()
WHERE task_id = $1
RETURNING *;

-- SyncUpsertTask updates the synced fields of a linked task without touching its
-- position (the user may have reordered it on the board).
-- name: SyncUpdateTask :one
UPDATE tasks
SET title = $2, description = $3, priority = $4, column_id = $5, completed_at = $6, updated_at = now()
WHERE id = $1
RETURNING *;
