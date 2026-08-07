-- name: ListWorkspaceCommands :many
SELECT * FROM workspace_commands
WHERE workspace_id = $1
ORDER BY position, key;

-- name: UpsertWorkspaceCommand :one
INSERT INTO workspace_commands (workspace_id, key, description, position)
VALUES ($1, $2, $3, $4)
ON CONFLICT (workspace_id, key) DO UPDATE
SET description = EXCLUDED.description, position = EXCLUDED.position, updated_at = now()
RETURNING *;

-- name: DeleteWorkspaceCommands :exec
DELETE FROM workspace_commands WHERE workspace_id = $1;
