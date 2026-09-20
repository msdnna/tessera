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

-- StripPushTokenFromOtherOwners takes the FCM token away from every OTHER user's
-- device channel that names the same physical install: same device_id (anchored
-- to the hardware on Android, so it survives logout and reinstall) or the same
-- token (issued per app install, never per account).
--
-- A phone only ever holds one signed-in account, and FCM delivers to the install
-- rather than to the account: whoever has the phone in their hands sees the
-- notification. So a device channel left over from the previous owner kept
-- pushing their private text — a mention, or a reminder that is literally their
-- own sentence — onto a screen that now belongs to someone else. The client half
-- of this is already closed (logout cancels the locally scheduled reminder
-- alarms, #2932); this is the server half, and it has to live on registration
-- rather than on logout, because a handover need not pass through one at all
-- (no network, or the account changed after a reinstall).
--
-- Only the token goes. The channel row is its owner's own setting, with their
-- label, routing rules and enabled flag hanging off it; if they sign back in on
-- this phone, the next registration puts the token back. Same policy
-- dropPushToken applies to a token FCM has declared dead.
-- name: StripPushTokenFromOtherOwners :execrows
UPDATE notification_channels
SET config = config - 'fcm_token', updated_at = now()
WHERE type = 'device'
  AND user_id <> @user_id
  AND config->>'fcm_token' IS NOT NULL
  AND (config->>'device_id' = @device_id::text
       OR config->>'fcm_token' = @fcm_token::text);

-- StripChannelFromRoutes drops a channel's id out of every routing rule of its
-- owner. notification_routes.channel_ids is a plain uuid[] with no FK, so the
-- channel delete leaves the id behind on its own; the dispatcher then skips the
-- unresolvable id silently, and — worse — the rule's next save fails the
-- ownership check, so the settings screen cannot even toggle it off again.
-- Scoped to the owner, so it can never touch someone else's rules.
-- name: StripChannelFromRoutes :exec
UPDATE notification_routes
SET channel_ids = array_remove(channel_ids, @channel_id::uuid), updated_at = now()
WHERE user_id = @user_id AND @channel_id::uuid = ANY(channel_ids);

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
-- next_attempt_at (used to defer past quiet hours / hold for a digest window). A
-- non-empty digest_group makes it eligible for combining with same-group rows.
-- name: CreateNotificationDeliveryAt :exec
INSERT INTO notification_deliveries (notification_id, channel_id, next_attempt_at, digest_group)
VALUES ($1, $2, $3, $4);

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

-- RequeueStaleDeliveries puts rows left in 'sending' by a process that died
-- between the claim and the settle back into the queue. Nothing else ever moves a
-- row out of 'sending', and ClaimPendingDeliveries only looks at 'pending', so
-- without this a restart mid-drain loses those deliveries for good.
--
-- Called at the top of a drain, which runs under the 'notify_delivery' worker
-- lock: holding it means no other instance is draining this outbox, so every
-- 'sending' row is provably orphaned rather than in flight. attempts is left
-- as-is, so a row whose send keeps killing the process still reaches
-- maxDeliveryAttempts and fails instead of looping forever.
-- name: RequeueStaleDeliveries :execrows
UPDATE notification_deliveries
SET status = 'pending', last_error = 'прервано перезапуском сервиса', updated_at = now()
WHERE status = 'sending';

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

-- GetDeviceChannel finds a user's device channel by its stable device id (stored
-- in config). Used by auto-registration to upsert idempotently.
-- name: GetDeviceChannel :one
SELECT * FROM notification_channels
WHERE user_id = $1 AND type = 'device' AND config->>'device_id' = @device_id::text;
