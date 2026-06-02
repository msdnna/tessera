-- name: LogTaskEvent :one
INSERT INTO task_events (task_id, actor_id, kind, data)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListTaskEvents :many
SELECT e.*, u.name AS actor_name, u.email AS actor_email
FROM task_events e
LEFT JOIN users u ON u.id = e.actor_id
WHERE e.task_id = $1
ORDER BY e.created_at;
