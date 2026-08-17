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

-- name: CreateGitlabIntegration :one
INSERT INTO gitlab_integrations (
    workspace_id, name, project_path, board_id, label_rules, enabled, owner_user_id,
    sync_interval_sec, due_source, start_source, writeback, scope, closed_policy, closed_after,
    relations_sync, full_sync_interval_sec, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, now())
RETURNING *;

-- name: UpdateGitlabIntegration :one
UPDATE gitlab_integrations
SET name = $2, project_path = $3, board_id = $4, label_rules = $5, enabled = $6,
    owner_user_id = $7, sync_interval_sec = $8, due_source = $9, start_source = $10,
    writeback = $11, scope = $12, closed_policy = $13, closed_after = $14,
    relations_sync = $15, full_sync_interval_sec = $16, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteGitlabIntegration :exec
DELETE FROM gitlab_integrations WHERE id = $1;

-- ReassignProjectGitlabWorkspace moves the workspace_id of any GitLab integration
-- bound to a board inside the given project, so the integration follows the project
-- when it is transferred between workspaces (links/members reference integration_id
-- and stay intact).
-- name: ReassignProjectGitlabWorkspace :exec
UPDATE gitlab_integrations SET workspace_id = $2
WHERE board_id IN (SELECT id FROM boards WHERE project_id = $1);

-- name: GetGitlabIntegration :one
SELECT * FROM gitlab_integrations WHERE id = $1;

-- name: ListGitlabIntegrationsByWorkspace :many
SELECT * FROM gitlab_integrations WHERE workspace_id = $1 ORDER BY name, created_at;

-- GetGitlabIntegrationByBoard resolves the binding that mirrors into a given board
-- (task-scoped operations: create-issue, board-scoped sync).
-- name: GetGitlabIntegrationByBoard :one
SELECT * FROM gitlab_integrations WHERE board_id = $1;

-- GetGitlabIntegrationByProject resolves a binding whose target board lives in a
-- project (milestone push is project-scoped). Picks the oldest when several.
-- name: GetGitlabIntegrationByProject :one
SELECT i.* FROM gitlab_integrations i
JOIN boards b ON b.id = i.board_id
WHERE b.project_id = $1
ORDER BY i.created_at
LIMIT 1;

-- MarkGitlabDueOverridden flags a linked task's due as user-set so the sync stops
-- touching it. No-op when the task isn't linked.
-- name: MarkGitlabDueOverridden :exec
UPDATE gitlab_links SET due_overridden = true WHERE task_id = $1;

-- name: MarkGitlabEstimateOverridden :exec
UPDATE gitlab_links SET estimate_overridden = true WHERE task_id = $1;

-- MarkGitlabStartOverridden flags a linked task's start as user-set so the sync
-- stops touching it. No-op when the task isn't linked.
-- name: MarkGitlabStartOverridden :exec
UPDATE gitlab_links SET start_overridden = true WHERE task_id = $1;

-- MarkGitlabSynced stamps the integration's last successful sync time.
-- name: MarkGitlabSynced :exec
UPDATE gitlab_integrations SET last_synced_at = now() WHERE id = $1;

-- MarkGitlabFullSynced stamps both the last sync time and the last FULL sweep time,
-- so the auto worker can tell when the next forced full sweep is due.
-- name: MarkGitlabFullSynced :exec
UPDATE gitlab_integrations SET last_synced_at = now(), last_full_synced_at = now() WHERE id = $1;

-- MarkGitlabMembersSynced stamps when the assignable-member roster was last pulled,
-- so an incremental sync can throttle the (expensive) roster refresh.
-- name: MarkGitlabMembersSynced :exec
UPDATE gitlab_integrations SET members_synced_at = now() WHERE id = $1;

-- ListAutoSyncIntegrations returns integrations due for unattended sync: enabled,
-- with a positive interval, an owner credential set, and either never synced or
-- past their interval. Used by the background worker.
-- name: ListAutoSyncIntegrations :many
SELECT * FROM gitlab_integrations
WHERE enabled
  AND sync_interval_sec > 0
  AND owner_user_id IS NOT NULL
  AND (last_synced_at IS NULL OR last_synced_at < now() - make_interval(secs => sync_interval_sec));

-- ListDueSyncIntegrations returns integrations due for unattended sync regardless
-- of an owner credential — used when an instance service token drives the sync.
-- name: ListDueSyncIntegrations :many
SELECT * FROM gitlab_integrations
WHERE enabled
  AND sync_interval_sec > 0
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
    gl_author_avatar_url, gl_last_state
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
RETURNING *;

-- name: UpdateGitlabLink :one
UPDATE gitlab_links
SET gl_iid = $2, gl_web_url = $3, gl_updated_at = $4,
    title_hash = $5, desc_hash = $6, labels_hash = $7,
    gl_author = $8, gl_author_name = $9, gl_author_avatar_url = $10,
    gl_last_state = $11,
    last_synced_at = now()
WHERE task_id = $1
RETURNING *;

-- SetGitlabLinkSnapshot stores the last-synced GitLab field state (the conflict
-- baseline). Written alongside the pull link-update and after a successful push, so
-- the next push can tell a clean overwrite from a both-sides-changed conflict.
-- name: SetGitlabLinkSnapshot :exec
UPDATE gitlab_links SET gl_snapshot = $2 WHERE task_id = $1;

-- SyncUpsertTask updates the synced fields of a linked task without touching its
-- position (the user may have reordered it on the board).
-- name: SyncUpdateTask :one
UPDATE tasks
SET title = $2, description = $3, priority = $4, column_id = $5, completed_at = $6, board_id = $7, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: LinkedIidsForIntegration :many
SELECT gl_iid FROM gitlab_links WHERE integration_id = $1;

-- LinkedTasksForIntegration maps every linked task to its GitLab global id, so a
-- full sweep can detect issues deleted in GitLab (a link with no matching issue in
-- the fetch) and archive the orphaned task.
-- name: LinkedTasksForIntegration :many
SELECT task_id, gl_global_id FROM gitlab_links WHERE integration_id = $1;

-- LinkedSyncKeysForIntegration returns the change-detection keys of every linked
-- issue, so an incremental sync can cheaply skip issues whose GitLab updatedAt is
-- unchanged (the 5-minute overlap window re-delivers already-synced issues).
-- title_hash/labels_hash guard the second-precision timestamp: two edits in the same
-- second share an updatedAt, so content hashes break the tie (desc_hash is omitted —
-- the stored one is of the asset-rewritten body, not comparable to the raw issue).
-- name: LinkedSyncKeysForIntegration :many
SELECT gl_global_id, gl_updated_at, title_hash, labels_hash FROM gitlab_links WHERE integration_id = $1;

-- ── sync reconciliation: mixed tags / assignees ────────────
-- Resolve a GitLab username to a Tessera user (via their linked credential).
-- name: GetUserIDByGitlabUsername :one
SELECT user_id FROM gitlab_credentials WHERE gl_username = $1;

-- AddTaskTagSourced returns the number of rows inserted (1 = newly attached, 0 =
-- the tag was already on the task) so the sync journal can record actual additions.
-- name: AddTaskTagSourced :execrows
INSERT INTO task_tags (task_id, tag_id, source) VALUES ($1, $2, $3)
ON CONFLICT (task_id, tag_id) DO NOTHING;

-- DeleteStaleGitlabTaskTags removes gitlab-sourced tags GitLab no longer has and
-- returns their names for the sync journal.
-- name: DeleteStaleGitlabTaskTags :many
WITH deleted AS (
    DELETE FROM task_tags
    WHERE task_id = $1 AND source = 'gitlab' AND NOT (tag_id = ANY($2::uuid[]))
    RETURNING tag_id
)
SELECT t.name FROM deleted d JOIN tags t ON t.id = d.tag_id;

-- name: AddTaskAssigneeSourced :exec
INSERT INTO task_assignees (task_id, user_id, source) VALUES ($1, $2, $3)
ON CONFLICT (task_id, user_id) DO NOTHING;

-- name: DeleteStaleGitlabAssignees :exec
DELETE FROM task_assignees
WHERE task_id = $1 AND source = 'gitlab' AND NOT (user_id = ANY($2::uuid[]));

-- ── external GitLab assignees (no Tessera account) ─────────
-- Only the sync-made set is rebuilt each run; user-pinned (source='user') survive.
-- name: DeleteGitlabSourcedAssignees :exec
DELETE FROM task_gitlab_assignees WHERE task_id = $1 AND source = 'gitlab';

-- Sync upsert: never downgrades an existing user-pinned row's source.
-- name: UpsertGitlabSourcedAssignee :exec
INSERT INTO task_gitlab_assignees (task_id, gl_username, gl_name, gl_avatar_url, source) VALUES ($1, $2, $3, $4, 'gitlab')
ON CONFLICT (task_id, gl_username) DO UPDATE SET gl_name = EXCLUDED.gl_name, gl_avatar_url = EXCLUDED.gl_avatar_url;

-- User pin (from the assignee picker): forces source='user' so the sync won't drop it.
-- name: PinGitlabAssignee :exec
INSERT INTO task_gitlab_assignees (task_id, gl_username, gl_name, gl_avatar_url, source) VALUES ($1, $2, $3, $4, 'user')
ON CONFLICT (task_id, gl_username) DO UPDATE SET source = 'user', gl_name = EXCLUDED.gl_name, gl_avatar_url = EXCLUDED.gl_avatar_url;

-- name: RemoveGitlabAssignee :exec
DELETE FROM task_gitlab_assignees WHERE task_id = $1 AND gl_username = $2;

-- name: ListTaskGitlabAssignees :many
SELECT gl_username, gl_name, gl_avatar_url, source FROM task_gitlab_assignees WHERE task_id = $1 ORDER BY gl_name;

-- ── GitLab project members (assignable from Tessera) ───────
-- name: UpsertGitlabProjectMember :exec
INSERT INTO gitlab_project_members (integration_id, gl_user_id, gl_username, gl_name, gl_avatar_url, access_level, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, now())
ON CONFLICT (integration_id, gl_user_id) DO UPDATE SET
    gl_username = EXCLUDED.gl_username, gl_name = EXCLUDED.gl_name,
    gl_avatar_url = EXCLUDED.gl_avatar_url, access_level = EXCLUDED.access_level, updated_at = now();

-- name: DeleteStaleGitlabProjectMembers :exec
DELETE FROM gitlab_project_members WHERE integration_id = $1 AND NOT (gl_user_id = ANY($2::bigint[]));

-- ListGitlabProjectMembersByWorkspace returns the assignable GitLab roster, each
-- annotated with the Tessera user it maps to (via OAuth identity or connected PAT),
-- so the UI can dedup members that already have a Tessera account.
-- name: ListGitlabProjectMembersByWorkspace :many
SELECT DISTINCT ON (m.gl_user_id)
  m.gl_user_id, m.gl_username, m.gl_name, m.gl_avatar_url, m.access_level,
  oi.user_id AS tessera_user_id,
  gc.user_id AS tessera_user_id_pat
FROM gitlab_project_members m
JOIN gitlab_integrations i ON i.id = m.integration_id
LEFT JOIN oauth_identities oi ON oi.provider = 'gitlab' AND oi.provider_username = m.gl_username
LEFT JOIN gitlab_credentials gc ON gc.gl_username = m.gl_username
WHERE i.workspace_id = $1
ORDER BY m.gl_user_id, m.gl_name;

-- name: GetGitlabMemberIDByUsername :one
SELECT gl_user_id FROM gitlab_project_members WHERE integration_id = $1 AND gl_username = $2;

-- ── synced comments (idempotent by GitLab note id) ─────────
-- ClaimPushedUserComment links a GitLab note back to the user's own comment that
-- produced it, instead of importing it as a duplicate. When a comment is posted
-- from Tessera the note gid is tagged asynchronously (SetCommentGlNoteID); if a
-- pull races that worker, the note would otherwise be inserted as a new
-- gitlab-sourced comment. This claims the most recent still-unlinked local user
-- comment (author_id set, gl_note_id NULL) with the same body on the task, so the
-- next pull dedups by gid. Returns the claimed comment id (no rows → not ours).
-- name: ClaimPushedUserComment :one
UPDATE task_comments
SET gl_note_id = $2, gl_discussion_id = $4, updated_at = now()
WHERE id = (
    SELECT tc.id FROM task_comments tc
    WHERE tc.task_id = $1
      AND tc.gl_note_id IS NULL
      AND tc.author_id IS NOT NULL
      AND tc.body = $3
    ORDER BY tc.created_at DESC
    LIMIT 1
)
RETURNING id;

-- UpsertGitlabComment returns whether the row was freshly inserted (xmax = 0) so
-- the sync journal can count new comments rather than re-synced ones. The
-- conflict branch also refreshes parent_id/gl_discussion_id: that is how already
-- imported flat comments acquire their thread on the next pull, so no data
-- migration is needed.
-- name: UpsertGitlabComment :one
INSERT INTO task_comments (task_id, author_id, body, gl_note_id, gl_author_login, gl_author_name, gl_author_avatar_url, created_at, updated_at, parent_id, gl_discussion_id)
VALUES ($1, NULL, $2, $3, $4, $5, $6, $7, $7, $8, $9)
ON CONFLICT (gl_note_id) WHERE gl_note_id IS NOT NULL
DO UPDATE SET body = EXCLUDED.body, gl_author_name = EXCLUDED.gl_author_name, gl_author_avatar_url = EXCLUDED.gl_author_avatar_url, parent_id = EXCLUDED.parent_id, gl_discussion_id = EXCLUDED.gl_discussion_id, updated_at = now()
RETURNING (xmax = 0) AS inserted;

-- GetCommentIDByGlNoteID resolves an already-imported GitLab note back to its
-- Tessera comment, so a reply arriving in a later pull than its root can still
-- be attached to the right thread.
-- name: GetCommentIDByGlNoteID :one
SELECT id FROM task_comments WHERE task_id = $1 AND gl_note_id = $2;

-- ── mirrored assets (Tessera upload → GitLab upload store) ──
-- The map is read in both directions: outbound to skip re-uploading an asset that is
-- already in the project's store, inbound (rewriteAssets) to turn our own mirrored
-- URL back into the Tessera one, so a description round-trips byte-for-byte and
-- title_desc conflict detection doesn't see a permanent divergence. See migration 0062.

-- name: GetGitlabUpload :one
SELECT * FROM gitlab_uploads
WHERE integration_id = $1 AND source_key = $2;

-- name: UpsertGitlabUpload :one
INSERT INTO gitlab_uploads (integration_id, source_key, gl_url, gl_markdown)
VALUES ($1, $2, $3, $4)
ON CONFLICT (integration_id, source_key) DO UPDATE
SET gl_url = EXCLUDED.gl_url, gl_markdown = EXCLUDED.gl_markdown
RETURNING *;

-- GetGitlabUploadSourceByURL resolves a GitLab upload URL back to the Tessera source
-- it was mirrored from, scoped to the workspace (rewriteAssets knows the workspace,
-- not which of its bindings performed the upload).
-- name: GetGitlabUploadSourceByURL :one
SELECT u.source_key FROM gitlab_uploads u
JOIN gitlab_integrations i ON i.id = u.integration_id
WHERE i.workspace_id = $1 AND u.gl_url = $2
LIMIT 1;
