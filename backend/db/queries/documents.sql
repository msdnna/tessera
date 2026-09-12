-- CreateDocument takes content and preview rather than leaving both at their
-- column defaults, because a document created from a template (D9) is born with
-- a body. Seeding it here instead of following the insert with a content update
-- keeps the new document's history honest: a document that starts as a template
-- has no "empty" state anyone could roll back to, and the version journal (D6)
-- would otherwise record one.
-- name: CreateDocument :one
INSERT INTO documents (workspace_id, project_id, parent_id, author_id, title, slug, icon, position, content, preview)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
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
--
-- UNION, not UNION ALL, and for the same reason as the task and project-group
-- walks: the guard keeps the tree acyclic, but a ring that slips in anyway (two
-- concurrent re-parents racing the check, an import, a manual fix) makes UNION ALL
-- recurse forever, while UNION drops the repeat and stops (#2932).
-- name: DocumentAncestorIDs :many
WITH RECURSIVE up AS (
    SELECT a.id, a.parent_id FROM documents a WHERE a.id = $1
    UNION
    SELECT d.id, d.parent_id FROM documents d JOIN up ON d.id = up.parent_id
)
SELECT id FROM up;

-- DeleteDocumentSubtree removes a document together with everything nested
-- below it. One statement, so the parent_id RESTRICT never sees an intermediate
-- state with a dangling child. UNION for the cycle safety described above — a
-- delete that never returns is the worst place to meet a ring.
-- name: DeleteDocumentSubtree :exec
WITH RECURSIVE down AS (
    SELECT a.id FROM documents a WHERE a.id = $1
    UNION
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

-- ProjectDocumentSlugs lists the addresses a project's documents occupy. A
-- document's slug is unique per workspace, so a transfer has to know them to
-- free the ones the target workspace already uses.
-- name: ProjectDocumentSlugs :many
SELECT id, slug FROM documents WHERE project_id = $1 ORDER BY created_at;

-- DetachDocumentParentsAcrossWorkspaces clears parent_id on every nesting
-- touching the given project whose two ends now sit in different workspaces —
-- the state a project transfer leaves behind, since a document's nesting and
-- its project are independent columns and only one of the pair travels.
-- parentInWorkspace refuses such a pair outright ("parent belongs to another
-- workspace"), so it can only appear when the boundary moves under an existing
-- nesting, and nothing downstream expects it: CountDocumentChildren and
-- DeleteDocumentSubtree walk parent_id with no workspace filter (an undeletable
-- parent with an invisible child, and a ?recursive=true that deletes another
-- workspace's documents), while ListDocuments is per workspace and the tree is
-- assembled from parent_id client-side, so a child whose parent is not in the
-- list renders nowhere at all. Detaching surfaces it at the root of the
-- workspace that now owns it, which is where a document with no parent belongs.
--
-- updated_at is deliberately left alone: it is the optimistic-concurrency token
-- UpdateDocumentContent checks, and bumping it for a change that never touched
-- the body would answer 409 to the next save of whoever has the document open.
-- Returns the number of nestings detached.
-- name: DetachDocumentParentsAcrossWorkspaces :execrows
UPDATE documents d
SET parent_id = NULL
WHERE d.parent_id IS NOT NULL
  AND (
        d.project_id = $1
     OR (SELECT p.project_id FROM documents p WHERE p.id = d.parent_id) = $1
      )
  AND d.workspace_id IS DISTINCT FROM
      (SELECT p.workspace_id FROM documents p WHERE p.id = d.parent_id);
