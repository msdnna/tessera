-- Notification channels, routing rules and the delivery outbox (Phase A).

-- ── Channels (per-user) ────────────────────────────────────

-- name: CreateNotificationChannel :one
INSERT INTO notification_channels (user_id, type, label, config, secret_enc, enabled, template)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: ListNotificationChannels :many
SELECT * FROM notification_channels WHERE user_id = $1 ORDER BY created_at;

-- name: GetNotificationChannel :one
SELECT * FROM notification_channels WHERE id = $1 AND user_id = $2;

-- GetNotificationChannelByID fetches a channel without the owner check — used by
-- the delivery worker, which operates off the request path.
-- name: GetNotificationChannelByID :one
SELECT * FROM notification_channels WHERE id = $1;

-- UpdateNotificationChannel updates a channel in place. secret_enc is replaced
-- only when a new secret was supplied (the handler passes the existing value
-- otherwise, so an edit that doesn't touch the secret keeps it).
-- name: UpdateNotificationChannel :one
UPDATE notification_channels
SET label = $3, config = $4, secret_enc = $5, enabled = $6, template = $7, updated_at = now()
WHERE id = $1 AND user_id = $2
RETURNING *;

-- name: SetNotificationChannelVerified :exec
UPDATE notification_channels SET verified = $3, updated_at = now() WHERE id = $1 AND user_id = $2;

-- name: DeleteNotificationChannel :exec
DELETE FROM notification_channels WHERE id = $1 AND user_id = $2;

-- ── Routes (per-user) ──────────────────────────────────────

-- name: CreateNotificationRoute :one
INSERT INTO notification_routes (user_id, position, matcher, channel_ids, options, enabled)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- ListNotificationRoutes returns every route for a user, ordered (the UI shows
-- all of them; the dispatcher filters to enabled ones in code).
-- name: ListNotificationRoutes :many
SELECT * FROM notification_routes WHERE user_id = $1 ORDER BY position, created_at;

-- name: UpdateNotificationRoute :one
UPDATE notification_routes
SET position = $3, matcher = $4, channel_ids = $5, options = $6, enabled = $7, updated_at = now()
WHERE id = $1 AND user_id = $2
RETURNING *;

-- name: DeleteNotificationRoute :exec
DELETE FROM notification_routes WHERE id = $1 AND user_id = $2;

-- ── Notification fetch (for delivery) ──────────────────────

-- name: GetNotification :one
SELECT * FROM notifications WHERE id = $1;

-- ── Delivery outbox ────────────────────────────────────────

-- name: CreateNotificationDelivery :exec
INSERT INTO notification_deliveries (notification_id, channel_id)
VALUES ($1, $2);

-- CreateNotificationDeliveryAt enqueues a delivery that won't be claimed before
-- next_attempt_at — used to defer external delivery past the recipient's quiet
-- hours.
-- name: CreateNotificationDeliveryAt :exec
INSERT INTO notification_deliveries (notification_id, channel_id, next_attempt_at)
VALUES ($1, $2, $3);

-- ClaimPendingDeliveries atomically grabs up to $1 due pending rows, marking them
-- 'sending' and bumping the attempt counter, so concurrent/queued workers never
-- pick the same row (FOR UPDATE SKIP LOCKED).
-- name: ClaimPendingDeliveries :many
UPDATE notification_deliveries
SET status = 'sending', attempts = attempts + 1, updated_at = now()
WHERE id IN (
    SELECT id FROM notification_deliveries
    WHERE status = 'pending' AND next_attempt_at <= now()
    ORDER BY next_attempt_at
    LIMIT $1
    FOR UPDATE SKIP LOCKED
)
RETURNING *;

-- name: MarkDeliverySent :exec
UPDATE notification_deliveries SET status = 'sent', last_error = '', updated_at = now() WHERE id = $1;

-- MarkDeliveryRetry reschedules a transient failure (back to pending, with a
-- backed-off next_attempt_at).
-- name: MarkDeliveryRetry :exec
UPDATE notification_deliveries
SET status = 'pending', last_error = $2, next_attempt_at = $3, updated_at = now()
WHERE id = $1;

-- name: MarkDeliveryFailed :exec
UPDATE notification_deliveries SET status = 'failed', last_error = $2, updated_at = now() WHERE id = $1;
