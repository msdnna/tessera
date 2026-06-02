-- name: CreateComment :one
INSERT INTO task_comments (task_id, author_id, body)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ListTaskComments :many
SELECT c.*, u.name AS author_name, u.email AS author_email
FROM task_comments c
LEFT JOIN users u ON u.id = c.author_id
WHERE c.task_id = $1
ORDER BY c.created_at;

-- name: GetComment :one
SELECT * FROM task_comments WHERE id = $1;

-- name: UpdateComment :one
UPDATE task_comments SET body = $2, updated_at = now() WHERE id = $1 RETURNING *;

-- name: DeleteComment :exec
DELETE FROM task_comments WHERE id = $1;
