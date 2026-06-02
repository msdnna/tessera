-- name: CreateProject :one
INSERT INTO projects (workspace_id, group_id, name, color, icon, position)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListProjects :many
SELECT * FROM projects WHERE workspace_id = $1 ORDER BY position;

-- name: GetProject :one
SELECT * FROM projects WHERE id = $1;

-- name: MaxProjectPosition :one
SELECT coalesce(max(position), 0)::double precision FROM projects WHERE workspace_id = $1;

-- name: UpdateProject :one
UPDATE projects
SET name = $2, color = $3, icon = $4, group_id = $5, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: MoveProject :one
UPDATE projects
SET group_id = $2, position = $3, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteProject :exec
DELETE FROM projects WHERE id = $1;
