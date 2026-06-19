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
INSERT INTO gitlab_integrations (workspace_id, project_path, board_id, label_rules, enabled, owner_user_id, sync_interval_sec, due_source, start_source, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now())
ON CONFLICT (workspace_id) DO UPDATE
SET project_path = EXCLUDED.project_path,
    board_id = EXCLUDED.board_id,
    label_rules = EXCLUDED.label_rules,
    enabled = EXCLUDED.enabled,
    owner_user_id = EXCLUDED.owner_user_id,
    sync_interval_sec = EXCLUDED.sync_interval_sec,
    due_source = EXCLUDED.due_source,
    start_source = EXCLUDED.start_source,
    updated_at = now()
RETURNING *;

-- MarkGitlabDueOverridden flags a linked task's due as user-set so the sync stops
-- touching it. No-op when the task isn't linked.
-- name: MarkGitlabDueOverridden :exec
UPDATE gitlab_links SET due_overridden = true WHERE task_id = $1;

-- MarkGitlabStartOverridden flags a linked task's start as user-set so the sync
-- stops touching it. No-op when the task isn't linked.
-- name: MarkGitlabStartOverridden :exec
UPDATE gitlab_links SET start_overridden = true WHERE task_id = $1;

-- name: GetGitlabIntegrationByWorkspace :one
SELECT * FROM gitlab_integrations WHERE workspace_id = $1;

-- MarkGitlabSynced stamps the integration's last successful sync time.
-- name: MarkGitlabSynced :exec
UPDATE gitlab_integrations SET last_synced_at = now() WHERE id = $1;

-- ListAutoSyncIntegrations returns integrations due for unattended sync: enabled,
-- with a positive interval, an owner credential set, and either never synced or
-- past their interval. Used by the background worker.
-- name: ListAutoSyncIntegrations :many
SELECT * FROM gitlab_integrations
WHERE enabled
  AND sync_interval_sec > 0
  AND owner_user_id IS NOT NULL
  AND (last_synced_at IS NULL OR last_synced_at < now() - make_interval(secs => sync_interval_sec));

-- ── Task ↔ work item link ──────────────────────────────────

-- name: GetGitlabLinkByGlobalID :one
SELECT * FROM gitlab_links WHERE integration_id = $1 AND gl_global_id = $2;

-- name: GetGitlabLinkByTask :one
SELECT * FROM gitlab_links WHERE task_id = $1;

-- name: CreateGitlabLink :one
INSERT INTO gitlab_links (
    task_id, integration_id, gl_global_id, gl_iid, gl_project_path, gl_web_url,
    gl_updated_at, title_hash, desc_hash, labels_hash, gl_author, gl_author_name,
    gl_author_avatar_url
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
RETURNING *;

-- name: UpdateGitlabLink :one
UPDATE gitlab_links
SET gl_iid = $2, gl_web_url = $3, gl_updated_at = $4,
    title_hash = $5, desc_hash = $6, labels_hash = $7,
    gl_author = $8, gl_author_name = $9, gl_author_avatar_url = $10,
    last_synced_at = now()
WHERE task_id = $1
RETURNING *;

-- SyncUpsertTask updates the synced fields of a linked task without touching its
-- position (the user may have reordered it on the board).
-- name: SyncUpdateTask :one
UPDATE tasks
SET title = $2, description = $3, priority = $4, column_id = $5, completed_at = $6, board_id = $7, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: LinkedIidsForIntegration :many
SELECT gl_iid FROM gitlab_links WHERE integration_id = $1;

-- ── sync reconciliation: mixed tags / assignees ────────────
-- Resolve a GitLab username to a Tessera user (via their linked credential).
-- name: GetUserIDByGitlabUsername :one
SELECT user_id FROM gitlab_credentials WHERE gl_username = $1;

-- name: AddTaskTagSourced :exec
INSERT INTO task_tags (task_id, tag_id, source) VALUES ($1, $2, $3)
ON CONFLICT (task_id, tag_id) DO NOTHING;

-- name: DeleteStaleGitlabTaskTags :exec
DELETE FROM task_tags
WHERE task_id = $1 AND source = 'gitlab' AND NOT (tag_id = ANY($2::uuid[]));

-- name: AddTaskAssigneeSourced :exec
INSERT INTO task_assignees (task_id, user_id, source) VALUES ($1, $2, $3)
ON CONFLICT (task_id, user_id) DO NOTHING;

-- name: DeleteStaleGitlabAssignees :exec
DELETE FROM task_assignees
WHERE task_id = $1 AND source = 'gitlab' AND NOT (user_id = ANY($2::uuid[]));

-- ── external GitLab assignees (no Tessera account) ─────────
-- name: DeleteTaskGitlabAssignees :exec
DELETE FROM task_gitlab_assignees WHERE task_id = $1;

-- name: AddTaskGitlabAssignee :exec
INSERT INTO task_gitlab_assignees (task_id, gl_username, gl_name, gl_avatar_url) VALUES ($1, $2, $3, $4)
ON CONFLICT (task_id, gl_username) DO UPDATE SET gl_name = EXCLUDED.gl_name, gl_avatar_url = EXCLUDED.gl_avatar_url;

-- name: ListTaskGitlabAssignees :many
SELECT gl_username, gl_name, gl_avatar_url FROM task_gitlab_assignees WHERE task_id = $1 ORDER BY gl_name;

-- ── synced comments (idempotent by GitLab note id) ─────────
-- name: UpsertGitlabComment :exec
INSERT INTO task_comments (task_id, author_id, body, gl_note_id, gl_author_login, gl_author_name, gl_author_avatar_url, created_at, updated_at)
VALUES ($1, NULL, $2, $3, $4, $5, $6, $7, $7)
ON CONFLICT (gl_note_id) WHERE gl_note_id IS NOT NULL
DO UPDATE SET body = EXCLUDED.body, gl_author_name = EXCLUDED.gl_author_name, gl_author_avatar_url = EXCLUDED.gl_author_avatar_url, updated_at = now();
