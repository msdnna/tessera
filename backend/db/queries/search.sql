-- name: SearchTasks :many
SELECT t.id, t.board_id, b.slug AS board_slug, p.slug AS project_slug, t.number, t.title, t.parent_id, t.completed_at
FROM tasks t
JOIN boards b ON b.id = t.board_id
JOIN projects p ON p.id = b.project_id
WHERE p.workspace_id = $1
  AND t.archived_at IS NULL
  AND (t.title ILIKE '%' || $2 || '%' OR t.description ILIKE '%' || $2 || '%')
ORDER BY t.number DESC
LIMIT 25;

-- name: SearchNotes :many
SELECT id, title, slug
FROM notes
WHERE workspace_id = $1
  AND (title ILIKE '%' || $2 || '%' OR body ILIKE '%' || $2 || '%')
ORDER BY updated_at DESC
LIMIT 25;

-- SearchDocuments matches on the title only: in D1 `content` is empty, and
-- full-text over the block jsonb is a D2 conversation.
-- name: SearchDocuments :many
SELECT id, title, slug
FROM documents
WHERE workspace_id = $1
  AND title ILIKE '%' || $2 || '%'
ORDER BY updated_at DESC
LIMIT 25;
