-- Conference chat (#2864, subtask #2873).

-- name: CreateConferenceMessage :one
INSERT INTO conference_messages (conference_id, user_id, body)
VALUES ($1, $2, $3)
RETURNING *;

-- ListConferenceMessages pages *backwards* from a cursor: a chat is read from
-- the bottom, so the rows worth fetching first are the newest ones. The handler
-- reverses the page into reading order — doing it in SQL would need a subquery
-- for no gain.
--
-- The cursor is (created_at, id) rather than created_at alone. Two messages sent
-- in the same millisecond are ordinary in a call where everyone is typing at
-- once, and a timestamp-only cursor would either skip one of them or serve it
-- twice forever.
-- name: ListConferenceMessages :many
SELECT m.*, u.name AS user_name, u.email AS user_email
FROM conference_messages m
LEFT JOIN users u ON u.id = m.user_id
WHERE m.conference_id = $1
  AND (
    sqlc.narg('before_at')::timestamptz IS NULL
    OR (m.created_at, m.id) < (sqlc.narg('before_at')::timestamptz, sqlc.narg('before_id')::uuid)
  )
ORDER BY m.created_at DESC, m.id DESC
LIMIT $2;

-- name: GetConferenceMessage :one
SELECT * FROM conference_messages WHERE id = $1;

-- name: DeleteConferenceMessage :exec
DELETE FROM conference_messages WHERE id = $1;

-- name: CreateConferenceMessageAttachment :one
INSERT INTO conference_message_attachments (message_id, uploader_id, filename, content_type, size, storage_path)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- ListConferenceMessageAttachmentsFor loads the attachments of a whole page of
-- messages in one statement. Asking per message would turn a fifty-message page
-- into fifty-one round trips, and the page is what the chat rail draws.
-- name: ListConferenceMessageAttachmentsFor :many
SELECT * FROM conference_message_attachments
WHERE message_id = ANY(sqlc.arg('message_ids')::uuid[])
ORDER BY created_at, id;

-- name: GetConferenceMessageAttachment :one
SELECT * FROM conference_message_attachments WHERE id = $1;

-- ConferenceForMessageAttachment resolves the conference an attachment belongs
-- to, so the download route can authorize by workspace membership without
-- fetching the message and then the conference separately.
-- name: ConferenceForMessageAttachment :one
SELECT c.*
FROM conference_message_attachments a
JOIN conference_messages m ON m.id = a.message_id
JOIN conferences c ON c.id = m.conference_id
WHERE a.id = $1;

-- name: ListConferenceMessageAttachmentPaths :many
SELECT storage_path FROM conference_message_attachments WHERE message_id = $1;
