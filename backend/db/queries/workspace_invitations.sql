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

-- name: DeclineInvitation :execrows
-- «Отклонить» (#3059). Only a live invitation can be declined: a spent, expired
-- or already declined one has nothing left to refuse.
UPDATE workspace_invitations SET declined_at = now()
WHERE id = $1 AND accepted_at IS NULL AND declined_at IS NULL AND expires_at > now();

-- name: ReissueInvitation :one
-- «Ещё раз» / «Продлить» (#3061): a new token (only the hash is stored, so a
-- resend cannot repeat the old link), a fresh lifetime, and a declined
-- invitation becomes a live one again. Scoped by workspace like DeleteInvitation.
UPDATE workspace_invitations
SET token_hash = $3, expires_at = $4, sent_at = now(), send_count = send_count + 1, declined_at = NULL
WHERE id = $1 AND workspace_id = $2 AND accepted_at IS NULL
RETURNING *;

-- name: GetInvitationInWorkspace :one
SELECT * FROM workspace_invitations WHERE id = $1 AND workspace_id = $2;

-- name: DeleteInvitation :execrows
-- Scoped by workspace: the caller's right to revoke was checked against the
-- workspace in the URL, so the invitation has to belong to that same one.
DELETE FROM workspace_invitations WHERE id = $1 AND workspace_id = $2;

-- name: ListPendingInvitationsByEmail :many
-- A declined invitation stays declined: proving the mailbox later must not
-- quietly accept what its owner turned down.
SELECT * FROM workspace_invitations
WHERE lower(email) = lower($1) AND accepted_at IS NULL AND declined_at IS NULL AND expires_at > now();

-- name: ListAllInvitations :many
-- The admin panel's «Приглашения» (#3079): every invitation not yet accepted,
-- across all workspaces, with where it leads and who sent it.
SELECT i.id, i.workspace_id, w.name AS workspace_name, i.email, i.role, i.created_at,
       i.expires_at, i.sent_at, i.send_count, i.declined_at,
       COALESCE(u.name, '')::text AS invited_by_name
FROM workspace_invitations i
JOIN workspaces w ON w.id = i.workspace_id
LEFT JOIN users u ON u.id = i.invited_by
WHERE i.accepted_at IS NULL
ORDER BY i.sent_at DESC;

-- name: DeleteInvitationAny :execrows
-- Instance admin revoking from «Приглашения» (#3079): not scoped by workspace —
-- the global admin flag is the right being checked. A spent one stays (it is
-- history of a membership, not an open door).
DELETE FROM workspace_invitations WHERE id = $1 AND accepted_at IS NULL;
