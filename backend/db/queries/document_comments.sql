-- name: CreateDocumentComment :one
INSERT INTO document_comments (document_id, block_id, parent_id, author_id, body, quote)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- ListDocumentComments returns every comment on a document, roots and replies
-- alike, oldest first. The client assembles the threads: it has to group them by
-- block anyway to place them next to the text, and a second round trip per
-- thread would make opening a discussed document a request storm.
-- name: ListDocumentComments :many
SELECT c.*, u.name AS author_name, u.email AS author_email
FROM document_comments c
LEFT JOIN users u ON u.id = c.author_id
WHERE c.document_id = $1
ORDER BY c.created_at;

-- name: GetDocumentComment :one
SELECT * FROM document_comments WHERE id = $1;

-- name: UpdateDocumentCommentBody :one
UPDATE document_comments SET body = $2, updated_at = now() WHERE id = $1 RETURNING *;

-- SetDocumentCommentResolved marks a thread done, or reopens it. The CHECK on
-- the table keeps this from ever landing on a reply.
-- name: SetDocumentCommentResolved :one
UPDATE document_comments
SET resolved_at = $2, resolved_by = $3, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteDocumentComment :exec
DELETE FROM document_comments WHERE id = $1;

-- name: CountDocumentCommentReplies :one
SELECT count(*) FROM document_comments WHERE parent_id = $1;
