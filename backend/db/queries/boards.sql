-- name: CreateBoard :one
INSERT INTO boards (project_id, name, position)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ListBoards :many
SELECT * FROM boards WHERE project_id = $1 ORDER BY position;

-- name: GetBoard :one
SELECT * FROM boards WHERE id = $1;

-- name: MaxBoardPosition :one
SELECT coalesce(max(position), 0)::double precision FROM boards WHERE project_id = $1;

-- name: UpdateBoard :one
UPDATE boards
SET name = $2, position = $3, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteBoard :exec
DELETE FROM boards WHERE id = $1;

-- name: CreateColumn :one
INSERT INTO board_columns (board_id, name, color, position)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListColumns :many
SELECT * FROM board_columns WHERE board_id = $1 ORDER BY position;

-- name: GetColumn :one
SELECT * FROM board_columns WHERE id = $1;

-- name: MaxColumnPosition :one
SELECT coalesce(max(position), 0)::double precision FROM board_columns WHERE board_id = $1;

-- name: UpdateColumn :one
UPDATE board_columns
SET name = $2, color = $3, position = $4, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteColumn :exec
DELETE FROM board_columns WHERE id = $1;
