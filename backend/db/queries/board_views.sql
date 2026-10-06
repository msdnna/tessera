-- name: ListBoardViews :many
SELECT * FROM board_views WHERE board_id = $1 AND user_id = $2 ORDER BY name;

-- name: GetBoardView :one
SELECT * FROM board_views WHERE id = $1;

-- UpsertBoardView creates a view or overwrites the user's same-named one.
-- name: UpsertBoardView :one
INSERT INTO board_views (board_id, user_id, name, config, updated_at)
VALUES ($1, $2, $3, $4, now())
ON CONFLICT (board_id, user_id, name) DO UPDATE
SET config = EXCLUDED.config, updated_at = now()
RETURNING *;

-- CountBoardViews counts one user's saved views on one board — the guard in
-- front of creating another. Scoped per (board, user) like every other read
-- here: the rows are private to their owner, so the bound has to be too, or one
-- member filling their own picker would lock the board's views for everybody.
-- name: CountBoardViews :one
SELECT count(*) FROM board_views WHERE board_id = $1 AND user_id = $2;

-- BoardViewNameExists answers whether the save about to happen is an overwrite
-- rather than a new row. Only asked once the count is already at the cap, and
-- what makes the cap safe to have: the board's autosave re-saves the loaded view
-- on every toolbar change, so refusing by count alone would freeze the layout of
-- exactly the user who uses the feature most.
-- name: BoardViewNameExists :one
SELECT EXISTS (
    SELECT 1 FROM board_views WHERE board_id = $1 AND user_id = $2 AND name = $3
);

-- name: DeleteBoardView :exec
DELETE FROM board_views WHERE id = $1;
