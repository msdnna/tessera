-- SearchTasks matches a substring of the title or description, or the task's
-- number («2950» / «#2950»), which then sorts first. The row carries what the
-- ⌘K palette prints under the title: status column, board, project, milestone.
-- name: SearchTasks :many
SELECT t.id, t.board_id, b.slug AS board_slug, p.slug AS project_slug, t.number, t.title, t.parent_id, t.completed_at,
       t.priority, t.column_id, b.name AS board_name, p.name AS project_name,
       bc.name AS column_name, bc.name_key AS column_name_key, bc.color AS column_color,
       m.title AS milestone_title
FROM tasks t
JOIN boards b ON b.id = t.board_id
JOIN projects p ON p.id = b.project_id
JOIN board_columns bc ON bc.id = t.column_id
LEFT JOIN milestones m ON m.id = t.milestone_id
WHERE p.workspace_id = $1
  AND t.archived_at IS NULL
  AND (t.title ILIKE '%' || sqlc.arg(q)::text || '%'
       OR t.description ILIKE '%' || sqlc.arg(q)::text || '%'
       OR t.number::text = ltrim(sqlc.arg(q)::text, '#'))
ORDER BY (t.number::text = ltrim(sqlc.arg(q)::text, '#')) DESC, t.number DESC
LIMIT 25;

-- SearchBoards finds boards by their own name or their project's — «Маркетинг»
-- should surface every board of that project.
-- name: SearchBoards :many
SELECT b.id, b.name, b.slug, b.icon, b.color, b.icon_mode, p.slug AS project_slug, p.name AS project_name
FROM boards b
JOIN projects p ON p.id = b.project_id
WHERE p.workspace_id = $1
  AND (b.name ILIKE '%' || sqlc.arg(q)::text || '%' OR p.name ILIKE '%' || sqlc.arg(q)::text || '%')
ORDER BY (b.name ILIKE '%' || sqlc.arg(q)::text || '%') DESC, p.name, b.position
LIMIT 10;

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
