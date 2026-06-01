-- name: CreateProjectGroup :one
INSERT INTO project_groups (workspace_id, name, position)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ListProjectGroups :many
SELECT * FROM project_groups WHERE workspace_id = $1 ORDER BY position;

-- name: GetProjectGroup :one
SELECT * FROM project_groups WHERE id = $1;

-- name: MaxProjectGroupPosition :one
SELECT coalesce(max(position), 0)::double precision FROM project_groups WHERE workspace_id = $1;

-- name: UpdateProjectGroup :one
UPDATE project_groups
SET name = $2, position = $3, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteProjectGroup :exec
DELETE FROM project_groups WHERE id = $1;
