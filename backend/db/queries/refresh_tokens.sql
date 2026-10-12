-- name: CreateRefreshToken :one
INSERT INTO refresh_tokens (user_id, token_hash, expires_at)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetRefreshToken :one
SELECT * FROM refresh_tokens WHERE token_hash = $1;

-- MediaSessionLive answers "is the session this media cookie names still
-- running, on an account that is still switched on?" — the one lookup
-- middleware.MediaAuth makes per inline image (#2932).
--
-- Both halves in one round trip because both are the same question asked of the
-- same credential, and the route pays for it once per image. The session half is
-- what a password change, a logout and a rotation all already act on; the
-- `active` half is the per-account check this absorbs (it used to be a standalone
-- IsUserActive lookup), kept because deactivation leaves the refresh rows alone
-- on purpose — it is a pause, and the cookie has to come back with the account.
--
-- user_id is matched as well as id so a token signed for one account can never
-- borrow another's session id, however it came by one.
-- name: MediaSessionLive :one
SELECT (u.active AND rt.revoked_at IS NULL AND rt.expires_at > now())::boolean AS live
FROM refresh_tokens rt
JOIN users u ON u.id = rt.user_id
WHERE rt.id = $1 AND rt.user_id = $2;

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
