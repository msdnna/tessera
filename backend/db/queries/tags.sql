-- name: CreateTag :one
INSERT INTO tags (workspace_id, name, color)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ListTags :many
SELECT * FROM tags WHERE workspace_id = $1 ORDER BY name;

-- name: GetTag :one
SELECT * FROM tags WHERE id = $1;

-- name: DeleteTag :exec
DELETE FROM tags WHERE id = $1;

-- name: AddTaskTag :exec
INSERT INTO task_tags (task_id, tag_id) VALUES ($1, $2) ON CONFLICT DO NOTHING;

-- name: RemoveTaskTag :exec
DELETE FROM task_tags WHERE task_id = $1 AND tag_id = $2;

-- name: ListTaskTags :many
SELECT t.*
FROM tags t
JOIN task_tags tt ON tt.tag_id = t.id
WHERE tt.task_id = $1
ORDER BY t.name;

-- name: AddTaskAssignee :exec
INSERT INTO task_assignees (task_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING;

-- name: RemoveTaskAssignee :exec
DELETE FROM task_assignees WHERE task_id = $1 AND user_id = $2;

-- name: ListTaskAssignees :many
SELECT u.id, u.email, u.name
FROM users u
JOIN task_assignees ta ON ta.user_id = u.id
WHERE ta.task_id = $1
ORDER BY u.name;
