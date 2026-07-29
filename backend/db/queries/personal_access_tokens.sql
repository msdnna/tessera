-- name: CreatePAT :one
INSERT INTO personal_access_tokens (user_id, name, token_hash, last_four, expires_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetPATByHash :one
SELECT * FROM personal_access_tokens WHERE token_hash = $1;

-- name: ListPATsByUser :many
SELECT id, user_id, name, last_four, expires_at, revoked_at, last_used_at, created_at
FROM personal_access_tokens
WHERE user_id = $1
ORDER BY created_at DESC;

-- name: RevokePAT :exec
UPDATE personal_access_tokens
SET revoked_at = now()
WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL;

-- name: TouchPATLastUsed :exec
UPDATE personal_access_tokens SET last_used_at = now() WHERE id = $1;
