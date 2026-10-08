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

-- name: HelpFeedbackTotals :many
-- «Статья помогла?» (#3071): a vote is an acknowledgement keyed
-- helpful:<slug>:<lang>:yes|no, so the totals are a count per key. The prefix
-- is matched with a plain LIKE — the key is client-owned and has no index of
-- its own; the admin screen asks for this rarely and the table is small.
SELECT key, count(*)::bigint AS votes
FROM user_acknowledgements
WHERE key LIKE 'helpful:%'
GROUP BY key
ORDER BY key;
