-- name: CreateWorkspace :one
INSERT INTO workspaces (name, owner_id)
VALUES ($1, $2)
RETURNING *;

-- name: GetWorkspace :one
SELECT * FROM workspaces WHERE id = $1;

-- name: NextWorkspaceTaskNumber :one
UPDATE workspaces SET task_counter = task_counter + 1 WHERE id = $1 RETURNING task_counter;

-- name: ListWorkspacesForUser :many
SELECT w.*, m.role AS my_role
FROM workspaces w
JOIN memberships m ON m.workspace_id = w.id
WHERE m.user_id = $1
ORDER BY w.created_at;

-- name: UpdateWorkspace :one
UPDATE workspaces
SET name = $2, updated_at = now()
WHERE id = $1
RETURNING *;

-- SetWorkspaceEstimation stores the workspace-wide default estimation config
-- (NULL clears it back to the built-in default). Provider-neutral; its own
-- endpoint so a name edit never clobbers it.
-- name: SetWorkspaceEstimation :one
UPDATE workspaces
SET estimation = $2, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteWorkspace :exec
DELETE FROM workspaces WHERE id = $1;

-- name: CreateMembership :one
INSERT INTO memberships (workspace_id, user_id, role)
VALUES ($1, $2, $3)
ON CONFLICT (workspace_id, user_id) DO UPDATE SET role = EXCLUDED.role
RETURNING *;

-- name: GetMembershipRole :one
SELECT role FROM memberships WHERE workspace_id = $1 AND user_id = $2;

-- name: GetMembership :one
SELECT * FROM memberships WHERE workspace_id = $1 AND user_id = $2;

-- name: ListMembers :many
-- gl_username is the member's GitLab login, so @-mentions can insert `@login`
-- instead of a display name (a name with spaces resolves to nothing once the
-- comment is pushed to GitLab). Scalar subquery rather than a LEFT JOIN:
-- oauth_identities is unique by (provider, provider_user_id), not by user_id, so
-- a user linked to two GitLab instances would otherwise be listed twice.
SELECT m.user_id, m.role, u.email, u.name,
  COALESCE((
    SELECT oi.provider_username FROM oauth_identities oi
    WHERE oi.user_id = u.id AND oi.provider = 'gitlab'
    ORDER BY oi.created_at, oi.id LIMIT 1
  ), '')::text AS gl_username
FROM memberships m
JOIN users u ON u.id = m.user_id
WHERE m.workspace_id = $1
ORDER BY u.name;

-- name: DeleteMembership :exec
DELETE FROM memberships WHERE workspace_id = $1 AND user_id = $2;

-- name: UpdateMembershipRole :one
UPDATE memberships SET role = $3
WHERE workspace_id = $1 AND user_id = $2
RETURNING *;
