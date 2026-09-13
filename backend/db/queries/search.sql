-- The search term arrives already escaped for LIKE (handlers.likeEscaper): it is
-- spliced into the pattern, so a `%`, `_` or `\` the user typed would otherwise
-- act as an operator instead of as text. Do not pass a raw query string here.

-- Each LIMIT below is handlers.searchLimit + 1, and the extra row is never shown:
-- the handler trims it off and reports its presence as has_more. Asking for
-- exactly the page size cannot distinguish "this is everything" from "this is the
-- first 25 of four hundred", and the dropdown then presents an arbitrary slice as
-- the whole answer (#2932). Counting instead — a second COUNT(*) per kind — would
-- pay for a full scan to render a number nobody reads. If the constant moves,
-- these three have to move with it; the truncation test pins the pair.

-- name: SearchTasks :many
SELECT t.id, t.board_id, b.slug AS board_slug, p.slug AS project_slug, t.number, t.title, t.parent_id, t.completed_at
FROM tasks t
JOIN boards b ON b.id = t.board_id
JOIN projects p ON p.id = b.project_id
WHERE p.workspace_id = $1
  AND t.archived_at IS NULL
  AND (t.title ILIKE '%' || $2 || '%' OR t.description ILIKE '%' || $2 || '%')
ORDER BY t.number DESC
LIMIT 26;

-- name: SearchNotes :many
SELECT id, title, slug
FROM notes
WHERE workspace_id = $1
  AND (title ILIKE '%' || $2 || '%' OR body ILIKE '%' || $2 || '%')
ORDER BY updated_at DESC
LIMIT 26;

-- SearchDocuments matches on the title only: in D1 `content` is empty, and
-- full-text over the block jsonb is a D2 conversation.
-- name: SearchDocuments :many
SELECT id, title, slug
FROM documents
WHERE workspace_id = $1
  AND title ILIKE '%' || $2 || '%'
ORDER BY updated_at DESC
LIMIT 26;
