-- name: CreateNotification :one
INSERT INTO notifications (user_id, workspace_id, task_id, actor_id, kind, text, payload)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: ListNotifications :many
-- A task on a soft-deleted board (0078) keeps its notification text — that was
-- written at send time — but loses the link: the board would answer 404. The
-- same for a task in the trash (0086). (The «deleted» notification itself has
-- no task_id: msgDeleted carries the board in its payload.)
SELECT
    n.*,
    t.number   AS task_number,
    t.board_id AS task_board_id,
    -- The whole title, for the row's chip (the payload inlines only a short one).
    t.title    AS task_title,
    -- The reader's answer to a conference invitation (#3051) — see
    -- ListNotificationsPage.
    cp.rsvp    AS rsvp
FROM notifications n
LEFT JOIN boards nb ON nb.id = (SELECT board_id FROM tasks WHERE id = n.task_id AND deleted_at IS NULL) AND nb.deleted_at IS NULL
LEFT JOIN tasks t ON t.id = n.task_id AND nb.id IS NOT NULL AND t.deleted_at IS NULL
LEFT JOIN conference_participants cp ON n.kind = 'conference_invite'
     AND cp.user_id = n.user_id AND cp.conference_id::text = n.payload->>'conference_id'
WHERE n.user_id = $1
ORDER BY n.created_at DESC, n.id DESC
LIMIT 50;

-- ListNotificationsPage is the page «Все уведомления» (#3049): the same rows as
-- ListNotifications, read backwards from a keyset cursor (created_at, id) and
-- narrowed by the feed's filter on the server — the popover's client-side
-- reading of 50 rows cannot say what lies past them.
--
-- The groups mirror the client's notificationGroup (utils/notificationFeed.js):
--   unread   — read_at IS NULL;
--   mentions — kind 'mention' (a task, a document, a meeting chat);
--   assigned — kind 'assigned';
--   gitlab   — an integration's sync report or conflict.
-- `rsvp` is the reader's own answer to a conference invitation (#3051), NULL
-- for every other row and for an invitation to a call that is gone.
-- name: ListNotificationsPage :many
SELECT
    n.*,
    t.number   AS task_number,
    t.board_id AS task_board_id,
    -- The whole title, for the row's chip (the payload inlines only a short one).
    t.title    AS task_title,
    cp.rsvp    AS rsvp
FROM notifications n
LEFT JOIN boards nb ON nb.id = (SELECT board_id FROM tasks WHERE id = n.task_id AND deleted_at IS NULL) AND nb.deleted_at IS NULL
LEFT JOIN tasks t ON t.id = n.task_id AND nb.id IS NOT NULL AND t.deleted_at IS NULL
LEFT JOIN conference_participants cp ON n.kind = 'conference_invite'
     AND cp.user_id = n.user_id AND cp.conference_id::text = n.payload->>'conference_id'
WHERE n.user_id = @user_id
  AND (sqlc.narg('before_at')::timestamptz IS NULL
       OR (n.created_at, n.id) < (sqlc.narg('before_at')::timestamptz, sqlc.narg('before_id')::uuid))
  AND (CASE @filter::text
         WHEN 'unread'   THEN n.read_at IS NULL
         WHEN 'mentions' THEN n.kind = 'mention'
         WHEN 'assigned' THEN n.kind = 'assigned'
         WHEN 'gitlab'   THEN n.kind IN ('integration_sync', 'integration_conflict')
         ELSE true
       END)
ORDER BY n.created_at DESC, n.id DESC
LIMIT @lim;

-- CountUnreadNotificationsByGroup counts the unread rows per filter of the feed,
-- for the counters on its tabs — the whole backlog, not the loaded page.
-- name: CountUnreadNotificationsByGroup :one
SELECT
    count(*)                                                         AS total,
    count(*) FILTER (WHERE kind = 'mention')                         AS mentions,
    count(*) FILTER (WHERE kind = 'assigned')                        AS assigned,
    count(*) FILTER (WHERE kind IN ('integration_sync', 'integration_conflict')) AS gitlab
FROM notifications
WHERE user_id = $1 AND read_at IS NULL;

-- name: MarkNotificationRead :exec
UPDATE notifications SET read_at = now() WHERE id = $1 AND user_id = $2 AND read_at IS NULL;

-- MarkConferenceInviteRead closes the reader's invitations to one call once
-- they answered it (#3051), wherever they answered — the feed, the meeting's
-- page or another device.
-- name: MarkConferenceInviteRead :exec
UPDATE notifications SET read_at = now()
WHERE user_id = @user_id AND kind = 'conference_invite' AND read_at IS NULL
  AND payload->>'conference_id' = @conference_id::text;

-- name: MarkAllNotificationsRead :exec
UPDATE notifications SET read_at = now() WHERE user_id = $1 AND read_at IS NULL;
