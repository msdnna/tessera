-- name: CreateDocumentTemplate :one
INSERT INTO document_templates (workspace_id, author_id, title, description, icon, content, preview)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- ListDocumentTemplates returns the workspace gallery. Content is left behind
-- for the same reason as in ListDocuments: a template is a whole document body,
-- and the gallery shows all of them at once — the preview column exists so this
-- list stays cheap.
-- name: ListDocumentTemplates :many
SELECT t.id, t.workspace_id, t.author_id, t.title, t.description, t.icon, t.preview,
       t.created_at, t.updated_at,
       u.name AS author_name
FROM document_templates t
LEFT JOIN users u ON u.id = t.author_id
WHERE t.workspace_id = $1
ORDER BY t.title, t.created_at;

-- name: GetDocumentTemplate :one
SELECT * FROM document_templates WHERE id = $1;

-- name: UpdateDocumentTemplateMeta :one
UPDATE document_templates
SET title = $2, description = $3, icon = $4, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteDocumentTemplate :exec
DELETE FROM document_templates WHERE id = $1;

-- name: CountDocumentTemplates :one
SELECT count(*) FROM document_templates WHERE workspace_id = $1;
