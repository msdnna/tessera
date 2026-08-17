-- name: CreateComment :one
INSERT INTO task_comments (task_id, author_id, body, parent_id)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- ListTaskComments returns the task's comments in thread order: a root, then its
-- replies, then the next root. The response stays a FLAT array (with parent_id)
-- rather than a nested tree — Android and the MCP server render it flat, and this
-- way they keep working unchanged and merely read in a more sensible order.
-- name: ListTaskComments :many
SELECT c.*, u.name AS author_name, u.email AS author_email
FROM task_comments c
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN task_comments p ON p.id = c.parent_id
WHERE c.task_id = $1
ORDER BY COALESCE(p.created_at, c.created_at), COALESCE(p.id, c.id), c.created_at;

-- name: GetComment :one
SELECT * FROM task_comments WHERE id = $1;

-- name: UpdateComment :one
UPDATE task_comments SET body = $2, updated_at = now() WHERE id = $1 RETURNING *;

-- name: DeleteComment :exec
DELETE FROM task_comments WHERE id = $1;

-- ListThreadReplies returns the replies under a root, oldest first. Used to pick
-- the successor when the root is deleted and to collect thread participants.
-- name: ListThreadReplies :many
SELECT * FROM task_comments WHERE parent_id = $1 ORDER BY created_at;

-- PromoteReplyToRoot detaches a reply so it can take over a deleted root.
-- name: PromoteReplyToRoot :exec
UPDATE task_comments SET parent_id = NULL, updated_at = now() WHERE id = $1;

-- ReparentReplies moves the remaining replies of a deleted root onto its successor.
-- name: ReparentReplies :exec
UPDATE task_comments SET parent_id = sqlc.arg(to_root), updated_at = now()
WHERE parent_id = sqlc.arg(from_root) AND id <> sqlc.arg(to_root);

-- ListThreadParticipants returns the Tessera users who wrote in a thread (the
-- root's author plus everyone who replied). GitLab-sourced comments have a NULL
-- author_id and drop out. Used to notify a thread when someone replies in it:
-- the root's author is often not a task participant and would otherwise never
-- learn that a branch was started under their comment.
-- name: ListThreadParticipants :many
SELECT DISTINCT c.author_id FROM task_comments c
WHERE c.author_id IS NOT NULL AND (c.id = $1 OR c.parent_id = $1);
