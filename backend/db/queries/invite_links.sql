-- name: GetLiveInviteLink :one
SELECT * FROM workspace_invite_links WHERE workspace_id = $1 AND revoked_at IS NULL;

-- name: GetInviteLinkByHash :one
SELECT * FROM workspace_invite_links WHERE token_hash = $1;

-- name: RevokeInviteLinks :execrows
UPDATE workspace_invite_links SET revoked_at = now(), revoked_by = $2
WHERE workspace_id = $1 AND revoked_at IS NULL;

-- name: CreateInviteLink :one
INSERT INTO workspace_invite_links (workspace_id, token_hash, role, max_uses, created_by, expires_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ConsumeInviteLink :one
-- Takes one place on the link, atomically: concurrent joins cannot overrun the
-- limit, and a revoked or expired link lets nobody in.
UPDATE workspace_invite_links SET uses = uses + 1
WHERE id = $1 AND revoked_at IS NULL AND expires_at > now() AND uses < max_uses
RETURNING *;

-- name: RecordInviteLinkUse :exec
INSERT INTO workspace_invite_link_uses (link_id, user_id) VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: InvitePreviewCounts :one
-- What the invitation page tells a newcomer about the workspace (#3059): how
-- many projects and people are behind the door — counts, never names.
SELECT
    (SELECT count(*) FROM projects p WHERE p.workspace_id = $1)::int AS projects,
    (SELECT count(*) FROM memberships m JOIN users u ON u.id = m.user_id
      WHERE m.workspace_id = $1 AND u.active)::int AS members;
