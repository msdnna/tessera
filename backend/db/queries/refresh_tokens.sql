-- name: CreateRefreshToken :one
INSERT INTO refresh_tokens (user_id, token_hash, expires_at)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetRefreshToken :one
SELECT * FROM refresh_tokens WHERE token_hash = $1;

-- Rotation revoke: the token stays inside the handler's grace window (see
-- refreshGrace in handlers/auth.go), so a client whose new pair was lost to a
-- dropped connection can retry with the old one.
-- name: RevokeRefreshToken :exec
UPDATE refresh_tokens SET revoked_at = now() WHERE token_hash = $1;

-- Hard revoke for sign-out: pulling expires_at into the past puts the token
-- outside the rotation grace window too, so "log out" means logged out now and
-- not thirty seconds from now.
-- name: ExpireRefreshToken :exec
UPDATE refresh_tokens
SET revoked_at = now(), expires_at = now() - interval '1 second'
WHERE token_hash = $1;

-- Same hard revoke across every session — used after a password reset. Selects
-- on expires_at rather than "revoked_at IS NULL" so tokens sitting in the grace
-- window are killed as well, instead of surviving the reset by half a minute.
-- name: RevokeAllUserTokens :exec
UPDATE refresh_tokens
SET revoked_at = now(), expires_at = now() - interval '1 second'
WHERE user_id = $1 AND expires_at > now();
