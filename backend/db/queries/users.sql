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
SELECT id, email, name, is_admin, active, email_verified, created_at, updated_at
FROM users
ORDER BY name;

-- name: SetUserAdmin :exec
UPDATE users SET is_admin = $2, updated_at = now() WHERE id = $1;

-- name: UpdateUserProfile :one
UPDATE users SET
    name        = $2,
    last_name   = $3,
    first_name  = $4,
    middle_name = $5,
    bio         = $6,
    company     = $7,
    job_title   = $8,
    updated_at  = now()
WHERE id = $1
RETURNING *;

-- name: UpdateUserPassword :exec
UPDATE users SET password_hash = $2, updated_at = now() WHERE id = $1;

-- name: SetUserActive :exec
UPDATE users SET active = $2, updated_at = now() WHERE id = $1;

-- name: MarkEmailVerified :exec
UPDATE users SET email_verified = true, updated_at = now() WHERE id = $1;
