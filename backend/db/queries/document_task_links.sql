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

-- CountDocumentTaskLinks and DocumentTaskLinkExists are the pair the cap needs:
-- how many links this document already carries, and whether the one being asked
-- for is among them. Two statements rather than one because they answer
-- different questions — the count decides whether the cap applies, the existence
-- check decides whether this particular call would add a row or refresh one that
-- is already there (see maxDocTaskLinks).
-- name: CountDocumentTaskLinks :one
SELECT count(*) FROM document_task_links WHERE document_id = $1;

-- name: DocumentTaskLinkExists :one
SELECT EXISTS (
    SELECT 1 FROM document_task_links
    WHERE document_id = @document_id AND task_id = @task_id AND block_id = @block_id
);

-- ListDocumentTaskLinks returns a document's links with enough of each task to
-- render the row without a second round trip per link. board_id rides along
-- because the panel's job is to navigate to the task, and the board is what the
-- route needs.
-- name: ListDocumentTaskLinks :many
SELECT l.id, l.document_id, l.task_id, l.block_id, l.quote, l.created_by, l.created_at,
       t.title AS task_title, t.board_id AS task_board_id, t.priority AS task_priority,
       t.completed_at AS task_completed_at
FROM document_task_links l
JOIN tasks t ON t.id = l.task_id AND t.deleted_at IS NULL
JOIN boards b ON b.id = t.board_id AND b.deleted_at IS NULL
WHERE l.document_id = $1
ORDER BY l.created_at;

-- ListTaskDocumentLinks is the same relation read from the task side, for the
-- "Документы" section of the task modal.
-- name: ListTaskDocumentLinks :many
SELECT l.id, l.document_id, l.task_id, l.block_id, l.quote, l.created_by, l.created_at,
       d.title AS document_title, d.icon AS document_icon, d.slug AS document_slug,
       d.workspace_id AS document_workspace_id
FROM document_task_links l
-- A document in the trash (0086) keeps its links — they come back with it — but
-- is out of the task's «Документы» meanwhile.
JOIN documents d ON d.id = l.document_id AND d.deleted_at IS NULL
WHERE l.task_id = $1
ORDER BY l.created_at;

-- MoveDocumentTaskLinks re-points every link of one task at another and returns
-- the documents that have to be told. Used when a repeating task's create_new
-- branch builds the next occurrence: the link names the artefact the routine is
-- governed by, and it belongs to whichever copy is live.
--
-- The links are moved rather than copied for what the reads do with them. Both
-- panels are unfiltered lists (ListDocumentTaskLinks joins tasks and hands
-- completed_at over for the styling alone), so a copy would leave the document
-- naming one more finished task every time the chore came round — the reschedule
-- branch edits the task in place and shows exactly one link on the live
-- occurrence, which is the shape the duplicate has to match. No conflict is
-- possible on the way: the destination is a task created moments ago, so it holds
-- no (document_id, task_id, block_id) of its own.
-- name: MoveDocumentTaskLinks :many
UPDATE document_task_links SET task_id = @to_task_id
WHERE task_id = @from_task_id
RETURNING document_id;

-- name: GetDocumentTaskLink :one
SELECT * FROM document_task_links WHERE id = $1;

-- name: DeleteDocumentTaskLink :exec
DELETE FROM document_task_links WHERE id = $1;

-- DeleteDocumentLinksAcrossWorkspaces drops every document↔task link touching
-- the given project whose two ends now sit in different workspaces — the state a
-- project transfer leaves behind when a document travels with its project while
-- the task it points at belongs to a project that stayed, or the reverse.
-- CreateDocumentTaskLink refuses such a pair outright, so it can only appear when
-- the boundary moves under an existing link; neither read re-checks it
-- (ListDocumentTaskLinks joins tasks for task_title, ListTaskDocumentLinks joins
-- documents for title/slug, both unfiltered), so a row left in place keeps
-- handing each side the other's titles. Returns the number of links dropped.
-- name: DeleteDocumentLinksAcrossWorkspaces :execrows
DELETE FROM document_task_links l
WHERE (
        l.task_id IN (SELECT t.id FROM tasks t JOIN boards b ON b.id = t.board_id WHERE b.project_id = $1)
     OR l.document_id IN (SELECT d.id FROM documents d WHERE d.project_id = $1)
      )
  AND (SELECT p.workspace_id FROM tasks t JOIN boards b ON b.id = t.board_id
       JOIN projects p ON p.id = b.project_id WHERE t.id = l.task_id)
      IS DISTINCT FROM
      (SELECT d.workspace_id FROM documents d WHERE d.id = l.document_id);
