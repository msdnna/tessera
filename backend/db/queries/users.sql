-- name: CreateUser :one
INSERT INTO users (email, name, password_hash, is_admin)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = $1;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;

-- name: CountUsers :one
SELECT count(*) FROM users;

-- name: ListUsers :many
SELECT id, email, name, is_admin, created_at, updated_at
FROM users
ORDER BY name;
