-- CreateDocumentTaskLink links a task to a document, optionally to one block in
-- it. Re-linking the same pair refreshes the quote instead of failing: the button
-- is in a panel that may be looking at a stale list, and "already linked" is not
-- an error the user can act on — the state they asked for is the state they get.
-- name: CreateDocumentTaskLink :one
INSERT INTO document_task_links (document_id, task_id, block_id, quote, created_by)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (document_id, task_id, block_id)
DO UPDATE SET quote = EXCLUDED.quote
RETURNING *;

-- ListDocumentTaskLinks returns a document's links with enough of each task to
-- render the row without a second round trip per link. board_id rides along
-- because the panel's job is to navigate to the task, and the board is what the
-- route needs.
-- name: ListDocumentTaskLinks :many
SELECT l.id, l.document_id, l.task_id, l.block_id, l.quote, l.created_by, l.created_at,
       t.title AS task_title, t.board_id AS task_board_id, t.priority AS task_priority,
       t.completed_at AS task_completed_at
FROM document_task_links l
JOIN tasks t ON t.id = l.task_id
WHERE l.document_id = $1
ORDER BY l.created_at;

-- ListTaskDocumentLinks is the same relation read from the task side, for the
-- "Документы" section of the task modal.
-- name: ListTaskDocumentLinks :many
SELECT l.id, l.document_id, l.task_id, l.block_id, l.quote, l.created_by, l.created_at,
       d.title AS document_title, d.icon AS document_icon, d.slug AS document_slug,
       d.workspace_id AS document_workspace_id
FROM document_task_links l
JOIN documents d ON d.id = l.document_id
WHERE l.task_id = $1
ORDER BY l.created_at;

-- name: GetDocumentTaskLink :one
SELECT * FROM document_task_links WHERE id = $1;

-- name: DeleteDocumentTaskLink :exec
DELETE FROM document_task_links WHERE id = $1;
