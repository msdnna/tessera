-- name: CreateAttachment :one
INSERT INTO task_attachments (task_id, uploader_id, filename, content_type, size, storage_path)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListTaskAttachments :many
SELECT a.*, u.name AS uploader_name
FROM task_attachments a
LEFT JOIN users u ON u.id = a.uploader_id
WHERE a.task_id = $1
ORDER BY a.created_at;

-- name: GetAttachment :one
SELECT * FROM task_attachments WHERE id = $1;

-- name: DeleteAttachment :exec
DELETE FROM task_attachments WHERE id = $1;
