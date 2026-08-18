-- name: ListUserAcknowledgements :many
SELECT key, ack_at FROM user_acknowledgements
WHERE user_id = $1
ORDER BY ack_at;

-- name: UpsertUserAcknowledgement :one
-- Idempotent: a repeated ack keeps the original ack_at (first-seen wins) and
-- still returns the row, so the caller can treat the endpoint as "mark seen".
INSERT INTO user_acknowledgements (user_id, key)
VALUES ($1, $2)
ON CONFLICT (user_id, key) DO UPDATE SET ack_at = user_acknowledgements.ack_at
RETURNING key, ack_at;
