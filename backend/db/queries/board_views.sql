-- Board views (#2970): a named snapshot of the board's toolbar. scope = 'user'
-- is one person's (user_id = them), scope = 'board' is shared with everybody on
-- the board (user_id IS NULL, created_by = whoever made it).

-- ListBoardViewsForUser returns what one reader sees on a board: their own views
-- and the board's shared ones — own first, then shared, each in its own order.
-- name: ListBoardViewsForUser :many
SELECT v.*, COALESCE(u.name, '')::text AS creator_name
FROM board_views v
LEFT JOIN users u ON u.id = v.created_by
WHERE v.board_id = $1 AND (v.scope = 'board' OR v.user_id = $2)
ORDER BY (v.scope = 'board'), v.position, v.name, v.id;

-- name: GetBoardView :one
SELECT * FROM board_views WHERE id = $1;

-- UpsertBoardView creates a personal view or overwrites the user's same-named
-- one — the save the old two-button toolbar (and the Android client) makes.
-- A new row goes to the end of the list; an overwrite keeps its place.
-- name: UpsertBoardView :one
INSERT INTO board_views (board_id, user_id, name, config, scope, position, created_by, updated_at)
VALUES ($1, $2, $3, $4, 'user', $5, $2, now())
ON CONFLICT (board_id, user_id, name) WHERE scope = 'user' DO UPDATE
SET config = EXCLUDED.config, updated_at = now()
RETURNING *;

-- UpsertSharedBoardView is the same for the board's shared list.
-- name: UpsertSharedBoardView :one
INSERT INTO board_views (board_id, user_id, name, config, scope, position, created_by, updated_at)
VALUES ($1, NULL, $2, $3, 'board', $4, $5, now())
ON CONFLICT (board_id, name) WHERE scope = 'board' DO UPDATE
SET config = EXCLUDED.config, updated_at = now()
RETURNING *;

-- CountBoardViews counts one user's saved views on one board — the guard in
-- front of creating another. Scoped per (board, user) like every other read
-- here: the rows are private to their owner, so the bound has to be too, or one
-- member filling their own picker would lock the board's views for everybody.
-- name: CountBoardViews :one
SELECT count(*) FROM board_views WHERE board_id = $1 AND user_id = $2 AND scope = 'user';

-- name: CountSharedBoardViews :one
SELECT count(*) FROM board_views WHERE board_id = $1 AND scope = 'board';

-- BoardViewNameExists answers whether the save about to happen is an overwrite
-- rather than a new row. Only asked once the count is already at the cap, and
-- what makes the cap safe to have: an overwrite must keep working for a user who
-- is at the cap, or saving the loaded view would freeze for exactly the user who
-- uses the feature most.
-- name: BoardViewNameExists :one
SELECT EXISTS (
    SELECT 1 FROM board_views
    WHERE board_id = $1 AND user_id = $2 AND name = $3 AND scope = 'user'
);

-- name: SharedBoardViewNameExists :one
SELECT EXISTS (
    SELECT 1 FROM board_views WHERE board_id = $1 AND name = $2 AND scope = 'board'
);

-- The end of one list — where a new view goes.
-- name: MaxUserBoardViewPosition :one
SELECT COALESCE(max(position), 0)::float8 FROM board_views
WHERE board_id = $1 AND user_id = $2 AND scope = 'user';

-- name: MaxSharedBoardViewPosition :one
SELECT COALESCE(max(position), 0)::float8 FROM board_views
WHERE board_id = $1 AND scope = 'board';

-- UpdateBoardView writes every mutable field at once; the handler merges the
-- partial PATCH over the stored row first.
-- name: UpdateBoardView :one
UPDATE board_views
SET name = $2, config = $3, scope = $4, user_id = $5, position = $6, is_default = $7,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- One star per list: setting it clears the previous one in the same list.
-- name: ClearUserBoardViewDefault :exec
UPDATE board_views SET is_default = false
WHERE board_id = $1 AND user_id = $2 AND scope = 'user' AND is_default;

-- name: ClearSharedBoardViewDefault :exec
UPDATE board_views SET is_default = false
WHERE board_id = $1 AND scope = 'board' AND is_default;

-- name: DeleteBoardView :exec
DELETE FROM board_views WHERE id = $1;
