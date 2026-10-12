-- name: ListTaskTemplates :many
SELECT * FROM task_templates WHERE workspace_id = $1 ORDER BY name;

-- name: GetTaskTemplate :one
SELECT * FROM task_templates WHERE id = $1;

-- name: CreateTaskTemplate :one
INSERT INTO task_templates (workspace_id, name, title, description, tags, priority, subtasks, created_by)
VALUES ($1, $2, $3, $4, @tags::text[], $5, @subtasks::text[], $6)
RETURNING *;

-- name: UpdateTaskTemplate :one
UPDATE task_templates
SET name = $2, title = $3, description = $4, tags = @tags::text[], priority = $5,
    subtasks = @subtasks::text[], updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteTaskTemplate :exec
DELETE FROM task_templates WHERE id = $1;

-- name: ListMacros :many
-- The viewer's macros of a workspace: the shared ones and their own.
SELECT * FROM workspace_macros
WHERE workspace_id = $1 AND (owner_id IS NULL OR owner_id = @viewer::uuid)
ORDER BY position, created_at;

-- name: GetMacro :one
SELECT * FROM workspace_macros WHERE id = $1;

-- name: CreateMacro :one
INSERT INTO workspace_macros (workspace_id, owner_id, name, trigger, action, params, keys, category, enabled, position, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9,
        COALESCE((SELECT MAX(position) FROM workspace_macros WHERE workspace_id = $1), 0) + 1, $10)
RETURNING *;

-- name: UpdateMacro :one
UPDATE workspace_macros
SET owner_id = $2, name = $3, trigger = $4, action = $5, params = $6, keys = $7,
    category = $8, enabled = $9, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteMacro :exec
DELETE FROM workspace_macros WHERE id = $1;
