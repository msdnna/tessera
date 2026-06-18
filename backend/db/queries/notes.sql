-- name: CreateNote :one
INSERT INTO notes (workspace_id, project_id, author_id, title, body, slug)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListNotes :many
SELECT * FROM notes WHERE workspace_id = $1 ORDER BY updated_at DESC;

-- name: NoteSlugExists :one
SELECT EXISTS(SELECT 1 FROM notes WHERE workspace_id = $1 AND slug = $2);

-- name: NotesMissingSlug :many
SELECT id, workspace_id, title FROM notes WHERE slug = '';

-- name: SetNoteSlug :exec
UPDATE notes SET slug = $2 WHERE id = $1;

-- name: GetNote :one
SELECT * FROM notes WHERE id = $1;

-- name: UpdateNote :one
UPDATE notes
SET title = $2, body = $3, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteNote :exec
DELETE FROM notes WHERE id = $1;
