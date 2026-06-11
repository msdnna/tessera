-- name: CreateTag :one
INSERT INTO tags (workspace_id, name, color)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ListTags :many
SELECT * FROM tags WHERE workspace_id = $1 ORDER BY name;

-- name: GetTag :one
SELECT * FROM tags WHERE id = $1;

-- EnsureTag returns the workspace tag with this name, creating it (with the
-- given color) if absent. On conflict it refreshes the colour only when a
-- non-empty one is supplied, so the GitLab sync keeps label colours current
-- without wiping a colour set elsewhere. Used by the GitLab sync.
-- name: EnsureTag :one
INSERT INTO tags (workspace_id, name, color)
VALUES ($1, $2, $3)
ON CONFLICT (workspace_id, name) DO UPDATE
SET color = COALESCE(NULLIF(EXCLUDED.color, ''), tags.color)
RETURNING *;

-- name: UpdateTag :one
UPDATE tags SET name = $2, color = $3 WHERE id = $1 RETURNING *;

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
