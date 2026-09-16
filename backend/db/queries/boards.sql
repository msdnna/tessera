-- name: CreateBoard :one
INSERT INTO boards (project_id, name, slug, position)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- ListBoards: created_at/id break position ties — see ListColumns below, the
-- midpoint scheme is the same one level up.
-- name: ListBoards :many
SELECT * FROM boards WHERE project_id = $1 ORDER BY position, created_at, id;

-- name: GetBoard :one
SELECT * FROM boards WHERE id = $1;

-- GetBoardBySlugForUser resolves a bare /board/<slug> link (no project in the
-- address) to a board the caller can actually see. Board slugs are unique only
-- per project since 0025, so several projects — and several workspaces — carry
-- the same slug at once; an unscoped `WHERE slug = $1 LIMIT 1` returns whichever
-- row Postgres reaches first, which is regularly a board the caller is not a
-- member of (their own link answers 403) or, when they are a member of both, a
-- different workspace's board with the URL canonicalised to it.
--
-- Candidates are therefore restricted to workspaces the caller belongs to.
-- preferred_workspace (may be NULL) is the client's hint — the workspace it is
-- currently showing — and wins when it holds a match; the oldest board breaks
-- any remaining tie, so the same link always lands on the same board.
-- name: GetBoardBySlugForUser :one
SELECT b.*
FROM boards b
JOIN projects p ON p.id = b.project_id
JOIN memberships m ON m.workspace_id = p.workspace_id AND m.user_id = sqlc.arg('user_id')
WHERE b.slug = sqlc.arg('slug')
ORDER BY (p.workspace_id = sqlc.narg('preferred_workspace')) DESC NULLS LAST, b.created_at, b.id
LIMIT 1;

-- name: GetBoardInProjectBySlug :one
SELECT * FROM boards WHERE project_id = $1 AND slug = $2;

-- name: BoardSlugExistsInProject :one
SELECT EXISTS(SELECT 1 FROM boards WHERE project_id = $1 AND slug = $2);

-- name: BoardsMissingSlug :many
SELECT id, project_id, name FROM boards WHERE slug = '';

-- name: SetBoardSlug :exec
UPDATE boards SET slug = $2 WHERE id = $1;

-- name: MaxBoardPosition :one
SELECT coalesce(max(position), 0)::double precision FROM boards WHERE project_id = $1;

-- name: UpdateBoard :one
UPDATE boards
SET name = $2, position = $3, icon = $4, color = $5, icon_mode = $6, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: SetBoardDoneColumn :one
UPDATE boards
SET done_column_id = $2, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteBoard :exec
DELETE FROM boards WHERE id = $1;

-- CreateColumn takes an optional name_key alongside the display name: the four
-- columns seeded with a new board pass their key ('todo', 'in_progress', …) so
-- clients can caption them in the reader's language, a column the user adds
-- himself passes NULL and is shown verbatim.
-- name: CreateColumn :one
INSERT INTO board_columns (board_id, name, name_key, color, position)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- ListColumns: created_at/id break position ties, exactly as for cards
-- (tasks.sql ListBoardTasks). Two siblings really can share a position — a
-- create reads Max*Position outside the insert's transaction, so two concurrent
-- creates land on the same value, and two reorders into one gap compute the same
-- midpoint. `ORDER BY position` alone then leaves them to the planner: the board
-- swaps them between reads. This also has to match RebalanceColumnPositions,
-- which numbers by (position, created_at, id) and promises to keep "the order
-- they already had" — without the tie-break here that order is the one the
-- reader never saw, so the respace silently swaps two columns for everyone.
-- name: ListColumns :many
SELECT * FROM board_columns WHERE board_id = $1 ORDER BY position, created_at, id;

-- name: GetColumn :one
SELECT * FROM board_columns WHERE id = $1;

-- name: MaxColumnPosition :one
SELECT coalesce(max(position), 0)::double precision FROM board_columns WHERE board_id = $1;

-- UpdateColumn drops name_key when the name actually changes: a column the user
-- renamed must keep that name in every language. Re-saving the same text (a
-- colour edit or a reorder both go through here) is not a rename, so the key
-- survives those.
-- name: UpdateColumn :one
UPDATE board_columns
SET name = $2,
    name_key = CASE WHEN name = $2 THEN name_key ELSE NULL END,
    color = $3, position = $4, updated_at = now()
WHERE id = $1
RETURNING *;

-- RebalanceColumnPositions respaces a board's columns onto a fresh gap * 1,2,3…
-- ladder, keeping the order they already had — the same recovery
-- Rebalance*TaskPositions gives cards, for the list that orders itself the same
-- way. MoveColumn calls it when the midpoint collapses onto a neighbour, which
-- is the only way that slot ever opens up again.
-- (UPDATE … FROM (subquery), not a CTE: sqlc mis-parses UPDATE … FROM with a
-- leading WITH.)
-- name: RebalanceColumnPositions :exec
UPDATE board_columns t
SET position = ordered.rn * sqlc.arg(gap)::double precision, updated_at = now()
FROM (
    SELECT id, row_number() OVER (ORDER BY position, created_at, id) AS rn
    FROM board_columns WHERE board_id = sqlc.arg(board_id)::uuid
) ordered
WHERE t.id = ordered.id;

-- name: DeleteColumn :exec
DELETE FROM board_columns WHERE id = $1;
