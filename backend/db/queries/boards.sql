-- name: CreateBoard :one
INSERT INTO boards (project_id, name, slug, position, default_layout)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- ListBoards: created_at/id break position ties — see ListColumns below, the
-- midpoint scheme is the same one level up.
-- name: ListBoards :many
SELECT * FROM boards WHERE project_id = $1 AND deleted_at IS NULL ORDER BY position, created_at, id;

-- A soft-deleted board (0078) answers as missing to every reader: GetBoard is
-- the gate most board-scoped endpoints pass through. GetBoardAny is for the few
-- paths that deal with the deleted row itself (restore, purge).
-- name: GetBoard :one
SELECT * FROM boards WHERE id = $1 AND deleted_at IS NULL;

-- name: GetBoardAny :one
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
WHERE b.slug = sqlc.arg('slug') AND b.deleted_at IS NULL
ORDER BY (p.workspace_id = sqlc.narg('preferred_workspace')) DESC NULLS LAST, b.created_at, b.id
LIMIT 1;

-- name: GetBoardInProjectBySlug :one
SELECT * FROM boards WHERE project_id = $1 AND slug = $2 AND deleted_at IS NULL;

-- BoardSlugExistsInProject deliberately counts soft-deleted boards: the slug
-- stays reserved while the board can still be restored, so the restore never
-- collides with a newer board that took the same address.
-- name: BoardSlugExistsInProject :one
SELECT EXISTS(SELECT 1 FROM boards WHERE project_id = $1 AND slug = $2);

-- name: BoardsMissingSlug :many
SELECT id, project_id, name FROM boards WHERE slug = '';

-- name: SetBoardSlug :exec
UPDATE boards SET slug = $2 WHERE id = $1;

-- name: MaxBoardPosition :one
SELECT coalesce(max(position), 0)::double precision FROM boards WHERE project_id = $1 AND deleted_at IS NULL;

-- name: UpdateBoard :one
UPDATE boards
SET name = $2, position = $3, icon = $4, color = $5, icon_mode = $6, default_layout = $7,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: SetBoardDoneColumn :one
UPDATE boards
SET done_column_id = $2, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteBoard :exec
DELETE FROM boards WHERE id = $1;

-- SoftDeleteBoard hides a board from every reader (0078); RestoreBoard brings it
-- back while the purge worker hasn't taken it yet. Both answer no row when the
-- board is already in the state asked for, so a double click can't flip it twice.
-- name: SoftDeleteBoard :one
UPDATE boards SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL RETURNING *;

-- name: RestoreBoard :one
UPDATE boards SET deleted_at = NULL, updated_at = now()
WHERE id = $1 AND deleted_at IS NOT NULL
RETURNING *;

-- PurgeDeletedBoards finishes deletes older than the cutoff. The task ids come
-- back first (DeletedBoardTaskIDs) so the caller can sweep their uploads after
-- the cascade, exactly like the immediate delete used to.
-- name: DeletedBoardTaskIDs :many
SELECT t.id FROM tasks t JOIN boards b ON b.id = t.board_id
WHERE b.deleted_at IS NOT NULL AND b.deleted_at < sqlc.arg(cutoff)::timestamptz;

-- name: PurgeDeletedBoards :execrows
DELETE FROM boards WHERE deleted_at IS NOT NULL AND deleted_at < sqlc.arg(cutoff)::timestamptz;

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
SELECT * FROM board_columns WHERE board_id = $1 AND deleted_at IS NULL ORDER BY position, created_at, id;

-- GetColumn hides soft-deleted columns (0078): nothing may move a card into one
-- or edit it. GetColumnAny serves restore.
-- name: GetColumn :one
SELECT * FROM board_columns WHERE id = $1 AND deleted_at IS NULL;

-- name: GetColumnAny :one
SELECT * FROM board_columns WHERE id = $1;

-- name: MaxColumnPosition :one
SELECT coalesce(max(position), 0)::double precision FROM board_columns WHERE board_id = $1 AND deleted_at IS NULL;

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
    FROM board_columns WHERE board_id = sqlc.arg(board_id)::uuid AND deleted_at IS NULL
) ordered
WHERE t.id = ordered.id;

-- name: DeleteColumn :exec
DELETE FROM board_columns WHERE id = $1;

-- Soft delete of a column (0078). The cards it held don't go with it any more:
-- RecordColumnMoves notes where each one stood, MoveColumnTasks appends them —
-- in their old order — to the end of the target column, and RestoreColumnTasks
-- puts back the ones still standing where the delete left them.
-- name: SoftDeleteColumn :one
UPDATE board_columns SET deleted_at = now(), deleted_was_done = sqlc.arg(was_done)::boolean
WHERE id = sqlc.arg(id)::uuid AND deleted_at IS NULL
RETURNING *;

-- name: RecordColumnMoves :execrows
INSERT INTO column_deletion_moves (column_id, task_id, moved_to, from_position)
SELECT t.column_id, t.id, sqlc.arg(moved_to)::uuid, t.position
FROM tasks t WHERE t.column_id = sqlc.arg(column_id)::uuid;

-- Only top-level cards are renumbered onto the target's ladder: a subtask's
-- position orders it among its siblings under the parent, not in the column, so
-- MoveColumnSubtasks changes nothing but the column.
-- (UPDATE … FROM (subquery), not a CTE: sqlc mis-parses UPDATE … FROM with a
-- leading WITH — see RebalanceColumnPositions.)
-- name: MoveColumnTasks :execrows
UPDATE tasks t
SET column_id = sqlc.arg(target)::uuid,
    position = sqlc.arg(base)::double precision + ordered.rn * sqlc.arg(gap)::double precision,
    updated_at = now()
FROM (
    SELECT id, row_number() OVER (ORDER BY position, created_at, id) AS rn
    FROM tasks WHERE column_id = sqlc.arg(source)::uuid AND parent_id IS NULL
) ordered
WHERE t.id = ordered.id;

-- name: MoveColumnSubtasks :execrows
UPDATE tasks SET column_id = sqlc.arg(target)::uuid, updated_at = now()
WHERE column_id = sqlc.arg(source)::uuid AND parent_id IS NOT NULL;

-- name: RestoreColumn :one
UPDATE board_columns SET deleted_at = NULL, deleted_was_done = false, updated_at = now()
WHERE id = $1 AND deleted_at IS NOT NULL
RETURNING *;

-- name: RestoreColumnTasks :execrows
UPDATE tasks t
SET column_id = m.column_id, position = m.from_position, updated_at = now()
FROM column_deletion_moves m
WHERE m.column_id = sqlc.arg(column_id)::uuid AND t.id = m.task_id AND t.column_id = m.moved_to;

-- name: ClearColumnMoves :exec
DELETE FROM column_deletion_moves WHERE column_id = $1;

-- name: PurgeDeletedColumns :execrows
DELETE FROM board_columns WHERE deleted_at IS NOT NULL AND deleted_at < sqlc.arg(cutoff)::timestamptz;

-- name: GetLiveBoardWorkspace :one
-- Presence on a board (#3116): the workspace a live board belongs to — the
-- socket may only announce itself on a board of a workspace in its scope.
SELECT p.workspace_id
FROM boards b
JOIN projects p ON p.id = b.project_id
WHERE b.id = $1 AND b.deleted_at IS NULL;
