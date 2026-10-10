-- Favourite boards (#3115): one person's starred boards, in their own order.
-- A row is (user, board); the list is read per workspace through the board's
-- project, and a board that is soft-deleted (0078) or sits in a workspace the
-- user is no longer a member of is filtered out here rather than deleted — a
-- restore, or a re-invite, brings the star back.

-- ListBoardFavorites: the caller's favourites in one workspace, with what the
-- sidebar row draws (the board's icon and colour, the project's name).
-- name: ListBoardFavorites :many
SELECT f.board_id, f.position, f.created_at,
       b.name, b.slug, b.icon, b.color, b.icon_mode, b.project_id,
       p.name AS project_name, p.slug AS project_slug, p.color AS project_color,
       p.workspace_id
FROM board_favorites f
JOIN boards b ON b.id = f.board_id AND b.deleted_at IS NULL
JOIN projects p ON p.id = b.project_id
JOIN memberships m ON m.workspace_id = p.workspace_id AND m.user_id = f.user_id
WHERE f.user_id = sqlc.arg('user_id') AND p.workspace_id = sqlc.arg('workspace_id')
ORDER BY f.position, f.created_at, f.board_id;

-- GetBoardFavoriteRow is one entry of the list above, in the same shape — what
-- add and move answer with.
-- name: GetBoardFavoriteRow :one
SELECT f.board_id, f.position, f.created_at,
       b.name, b.slug, b.icon, b.color, b.icon_mode, b.project_id,
       p.name AS project_name, p.slug AS project_slug, p.color AS project_color,
       p.workspace_id
FROM board_favorites f
JOIN boards b ON b.id = f.board_id AND b.deleted_at IS NULL
JOIN projects p ON p.id = b.project_id
WHERE f.user_id = sqlc.arg('user_id') AND f.board_id = sqlc.arg('board_id');

-- The end of one person's list — where a new favourite goes. Counted across
-- workspaces: the order only has to hold within each workspace's slice of it,
-- and one ladder per person keeps every slice in order at once.
-- name: MaxBoardFavoritePosition :one
SELECT COALESCE(max(position), 0)::float8 FROM board_favorites WHERE user_id = $1;

-- AddBoardFavorite stars a board; starring it again is a no-op that keeps the
-- row's place (the upsert only exists so the call answers with the row).
-- name: AddBoardFavorite :one
INSERT INTO board_favorites (user_id, board_id, position)
VALUES ($1, $2, $3)
ON CONFLICT (user_id, board_id) DO UPDATE SET position = board_favorites.position
RETURNING *;

-- name: DeleteBoardFavorite :execrows
DELETE FROM board_favorites WHERE user_id = $1 AND board_id = $2;

-- name: SetBoardFavoritePosition :exec
UPDATE board_favorites SET position = $3 WHERE user_id = $1 AND board_id = $2;

-- RebalanceBoardFavoritePositions respaces one person's favourites onto a fresh
-- gap * 1,2,3… ladder in the order they already have — the way out of a float8
-- slot that midpoint drops have used up (see positionForReorder).
-- name: RebalanceBoardFavoritePositions :exec
UPDATE board_favorites t
SET position = ordered.rn * sqlc.arg(gap)::double precision
FROM (
    SELECT board_id, row_number() OVER (ORDER BY position, created_at, board_id) AS rn
    FROM board_favorites WHERE user_id = sqlc.arg(user_id)::uuid
) ordered
WHERE t.user_id = sqlc.arg(user_id)::uuid AND t.board_id = ordered.board_id;
