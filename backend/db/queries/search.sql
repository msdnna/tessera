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

-- A task is also addressable by its number, the way it is written everywhere else
-- in the product (`#2932` on a card, `?task=2932` in a URL, the board's own search
-- box). @number carries the digits the handler read out of the query, or 0 when
-- there were none — 0 is not a task number (they start at 1), so the branch costs
-- a comparison and never widens a text search by accident.
--
-- The exact number wins the ordering. A search for `2932` also matches every task
-- whose description mentions it, and those are the rows with the higher numbers:
-- by `number DESC` alone the task actually named #2932 sorts below its own
-- mentions, and past the page cut it is not in the answer at all (#2932).
--
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
WHERE p.workspace_id = @workspace_id
  AND t.archived_at IS NULL
  AND (t.title ILIKE '%' || @term || '%' OR t.description ILIKE '%' || @term || '%'
       OR t.number = @number::bigint)
ORDER BY (t.number = @number::bigint) DESC, t.number DESC
LIMIT 26;

-- SearchBoards finds boards by their own name or their project's — «Маркетинг»
-- should surface every board of that project.
-- name: SearchBoards :many
SELECT b.id, b.name, b.slug, b.icon, b.color, b.icon_mode, p.slug AS project_slug, p.name AS project_name
FROM boards b
JOIN projects p ON p.id = b.project_id
WHERE p.workspace_id = @workspace_id
  AND (b.name ILIKE '%' || @term || '%' OR p.name ILIKE '%' || @term || '%')
ORDER BY (b.name ILIKE '%' || @term || '%') DESC, p.name, b.position
LIMIT 10;

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
