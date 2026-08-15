-- name: CreateDocument :one
INSERT INTO documents (workspace_id, project_id, parent_id, author_id, title, slug, icon, position)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- ListDocuments returns the workspace's documents as a flat list; the tree is
-- assembled client-side (same call shape serves Android and MCP). Columns are
-- listed explicitly to leave `content` behind: with D2 that column holds
-- hundreds of KB per document, and SELECT * would turn opening the section into
-- downloading every document in the workspace.
-- name: ListDocuments :many
SELECT id, workspace_id, project_id, parent_id, author_id, title, slug, icon, preview, position, created_at, updated_at
FROM documents
WHERE workspace_id = $1
ORDER BY position, created_at;

-- name: ListDocumentsByProject :many
SELECT id, workspace_id, project_id, parent_id, author_id, title, slug, icon, preview, position, created_at, updated_at
FROM documents
WHERE workspace_id = $1 AND project_id = $2
ORDER BY position, created_at;

-- name: GetDocument :one
SELECT * FROM documents WHERE id = $1;

-- name: GetDocumentBySlug :one
SELECT * FROM documents WHERE workspace_id = $1 AND slug = $2;

-- name: DocumentSlugExists :one
SELECT EXISTS(SELECT 1 FROM documents WHERE workspace_id = $1 AND slug = $2);

-- name: UpdateDocumentMeta :one
UPDATE documents
SET title = $2, icon = $3, parent_id = $4, project_id = $5, position = $6, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: CountDocumentChildren :one
SELECT count(*) FROM documents WHERE parent_id = $1;

-- DocumentAncestorIDs returns the document itself followed by its ancestors, up
-- to the root. Used to reject re-parenting a document into its own subtree,
-- which would detach the branch from the tree and leave it orphaned in the
-- table. The membership test is done in Go: sqlc's analyzer cannot resolve a
-- recursive CTE referenced from inside a subquery, so EXISTS(...) here does not
-- generate.
-- name: DocumentAncestorIDs :many
WITH RECURSIVE up AS (
    SELECT a.id, a.parent_id FROM documents a WHERE a.id = $1
    UNION ALL
    SELECT d.id, d.parent_id FROM documents d JOIN up ON d.id = up.parent_id
)
SELECT id FROM up;

-- DeleteDocumentSubtree removes a document together with everything nested
-- below it. One statement, so the parent_id RESTRICT never sees an intermediate
-- state with a dangling child.
-- name: DeleteDocumentSubtree :exec
WITH RECURSIVE down AS (
    SELECT a.id FROM documents a WHERE a.id = $1
    UNION ALL
    SELECT d.id FROM documents d JOIN down ON d.parent_id = down.id
)
DELETE FROM documents WHERE documents.id IN (SELECT id FROM down);

-- ReassignProjectDocumentsWorkspace re-stamps workspace_id on documents tied to
-- a project, used when the project is transferred between workspaces (mirrors
-- notes). Without it a transferred project's documents keep the old
-- workspace_id, and requireMember authorizes on exactly that column — the
-- documents would stay visible to the team the project left.
-- name: ReassignProjectDocumentsWorkspace :exec
UPDATE documents SET workspace_id = $2 WHERE project_id = $1;

-- name: DocumentsMissingSlug :many
SELECT id, workspace_id, title FROM documents WHERE slug = '';

-- name: SetDocumentSlug :exec
UPDATE documents SET slug = $2 WHERE id = $1;

-- UpdateDocumentContent writes the document body. The updated_at guard is
-- optimistic concurrency: the client sends the timestamp it loaded, and a row
-- that moved on in the meantime matches nothing, so the handler answers 409
-- instead of overwriting an edit it never saw. Per-block merging is D4's job;
-- silently losing the other side's work is not an acceptable stand-in for it.
-- name: UpdateDocumentContent :one
UPDATE documents
SET content = $2, preview = $3, updated_at = now()
WHERE id = $1 AND updated_at = $4
RETURNING *;

-- SetDocumentContent writes the body without the updated_at guard above. Used
-- only by the rollback (D6): a restore is not a client racing another client
-- with a stale copy, it is an explicit "make it look like revision N again", and
-- failing it because someone autosaved a second earlier would make the button
-- work only on idle documents. The state being overwritten is snapshotted first,
-- so the rollback is itself undoable.
-- name: SetDocumentContent :one
UPDATE documents
SET content = $2, preview = $3, updated_at = now()
WHERE id = $1
RETURNING *;
