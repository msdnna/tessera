-- name: CreateNotification :one
INSERT INTO notifications (user_id, workspace_id, task_id, actor_id, kind, text)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListNotifications :many
SELECT
    n.*,
    t.number   AS task_number,
    t.board_id AS task_board_id
FROM notifications n
LEFT JOIN tasks t ON t.id = n.task_id
WHERE n.user_id = $1
ORDER BY n.created_at DESC
LIMIT 50;

-- name: CountUnreadNotifications :one
SELECT count(*) FROM notifications WHERE user_id = $1 AND read_at IS NULL;

-- name: MarkNotificationRead :exec
UPDATE notifications SET read_at = now() WHERE id = $1 AND user_id = $2 AND read_at IS NULL;

-- name: MarkAllNotificationsRead :exec
UPDATE notifications SET read_at = now() WHERE user_id = $1 AND read_at IS NULL;
