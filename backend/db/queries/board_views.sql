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

-- name: DeleteBoardView :exec
DELETE FROM board_views WHERE id = $1;
