-- name: SearchTasks :many
SELECT t.id, t.board_id, t.number, t.title, t.parent_id, t.completed_at
FROM tasks t
JOIN boards b ON b.id = t.board_id
JOIN projects p ON p.id = b.project_id
WHERE p.workspace_id = $1
  AND t.archived_at IS NULL
  AND (t.title ILIKE '%' || $2 || '%' OR t.description ILIKE '%' || $2 || '%')
ORDER BY t.number DESC
LIMIT 25;

-- name: SearchNotes :many
SELECT id, title
FROM notes
WHERE workspace_id = $1
  AND (title ILIKE '%' || $2 || '%' OR body ILIKE '%' || $2 || '%')
ORDER BY updated_at DESC
LIMIT 25;
