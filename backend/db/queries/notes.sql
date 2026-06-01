-- name: CreateNote :one
INSERT INTO notes (workspace_id, project_id, author_id, title, body)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListNotes :many
SELECT * FROM notes WHERE workspace_id = $1 ORDER BY updated_at DESC;

-- name: GetNote :one
SELECT * FROM notes WHERE id = $1;

-- name: UpdateNote :one
UPDATE notes
SET title = $2, body = $3, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteNote :exec
DELETE FROM notes WHERE id = $1;
