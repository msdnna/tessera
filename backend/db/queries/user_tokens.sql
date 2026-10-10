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

-- name: DeleteUserTokensOfKinds :exec
DELETE FROM user_tokens WHERE user_id = $1 AND kind = ANY(sqlc.arg(kinds)::text[]);

-- name: CreateUserCode :one
-- A one-time code (#3035): kind 'verify_code' or 'email_change'; email is the
-- address an 'email_change' code is proving.
INSERT INTO user_tokens (user_id, kind, token_hash, expires_at, email)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetLiveUserCode :one
-- The newest code of the kinds that can still be tried. A fresh code supersedes
-- (marks used) the previous ones, so at most one is ever live.
SELECT * FROM user_tokens
WHERE user_id = $1 AND kind = ANY(sqlc.arg(kinds)::text[]) AND used_at IS NULL AND expires_at > now()
ORDER BY created_at DESC
LIMIT 1;

-- name: SpendUserCodeAttempt :one
-- Counts one attempt against a live code *before* it is compared, atomically, so
-- parallel guesses cannot exceed the budget. No row back = no attempts left, or
-- the code was spent / superseded / expired meanwhile.
UPDATE user_tokens SET attempts = attempts + 1
WHERE id = $1 AND used_at IS NULL AND expires_at > now() AND attempts < sqlc.arg(max_attempts)::int
RETURNING attempts;

-- name: SupersedeUserCodes :exec
-- Retires every still-live code of the given kinds: a fresh code replaces the
-- previous one rather than opening a second door.
UPDATE user_tokens SET used_at = now()
WHERE user_id = $1 AND kind = ANY(sqlc.arg(kinds)::text[]) AND used_at IS NULL;

-- name: RecentUserCodes :one
-- Codes issued to the user since a moment, and when the latest of them went out:
-- the pause between resends and the hourly cap.
SELECT count(*)::int AS n,
       COALESCE(min(created_at), 'epoch'::timestamptz)::timestamptz AS first_sent,
       COALESCE(max(created_at), 'epoch'::timestamptz)::timestamptz AS last_sent
FROM user_tokens
WHERE user_id = $1 AND kind IN ('verify_code', 'email_change') AND created_at > sqlc.arg(since)::timestamptz;
