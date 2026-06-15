-- name: CreateInvitation :one
INSERT INTO workspace_invitations (workspace_id, email, role, token_hash, invited_by, expires_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListInvitations :many
SELECT * FROM workspace_invitations
WHERE workspace_id = $1 AND accepted_at IS NULL
ORDER BY created_at DESC;

-- name: GetInvitationByHash :one
SELECT * FROM workspace_invitations WHERE token_hash = $1;

-- name: MarkInvitationAccepted :exec
UPDATE workspace_invitations SET accepted_at = now() WHERE id = $1;

-- name: DeleteInvitation :exec
DELETE FROM workspace_invitations WHERE id = $1;

-- name: ListPendingInvitationsByEmail :many
SELECT * FROM workspace_invitations
WHERE lower(email) = lower($1) AND accepted_at IS NULL AND expires_at > now();
