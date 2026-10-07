-- name: CreateNotification :one
INSERT INTO notifications (user_id, workspace_id, task_id, actor_id, kind, text, payload)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: ListNotifications :many
-- A task on a soft-deleted board (0078) keeps its notification text — that was
-- written at send time — but loses the link: the board would answer 404.
SELECT
    n.*,
    t.number   AS task_number,
    t.board_id AS task_board_id
FROM notifications n
LEFT JOIN boards nb ON nb.id = (SELECT board_id FROM tasks WHERE id = n.task_id) AND nb.deleted_at IS NULL
LEFT JOIN tasks t ON t.id = n.task_id AND nb.id IS NOT NULL
WHERE n.user_id = $1
ORDER BY n.created_at DESC
LIMIT 50;

-- name: CountUnreadNotifications :one
SELECT count(*) FROM notifications WHERE user_id = $1 AND read_at IS NULL;

-- name: MarkNotificationRead :exec
UPDATE notifications SET read_at = now() WHERE id = $1 AND user_id = $2 AND read_at IS NULL;

-- name: MarkAllNotificationsRead :exec
UPDATE notifications SET read_at = now() WHERE user_id = $1 AND read_at IS NULL;
