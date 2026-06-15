-- name: CreateUserToken :one
INSERT INTO user_tokens (user_id, kind, token_hash, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetUserToken :one
SELECT * FROM user_tokens WHERE token_hash = $1 AND kind = $2;

-- name: MarkUserTokenUsed :exec
UPDATE user_tokens SET used_at = now() WHERE id = $1;

-- name: DeleteUserTokensOfKind :exec
DELETE FROM user_tokens WHERE user_id = $1 AND kind = $2;
