-- CreateDocumentVersion appends a snapshot. The revision is picked inside the
-- statement rather than read first and incremented in Go: two autosaves from two
-- tabs land here concurrently, and a read-then-write would hand them the same
-- number — which the unique constraint would turn into a failed save of a
-- document the user already typed.
-- name: CreateDocumentVersion :one
INSERT INTO document_versions (document_id, revision, author_id, title, content, preview, label, manual)
VALUES (
    @document_id,
    (SELECT coalesce(max(v.revision), 0) + 1 FROM document_versions v WHERE v.document_id = @document_id),
    @author_id, @title, @content, @preview, @label, @manual
)
RETURNING *;

-- ListDocumentVersions returns the journal, newest first. Content is left out on
-- purpose: a document is up to a megabyte of ProseMirror JSON and the journal
-- shows fifty of them — the panel asks for a body only when a version is opened
-- or compared.
-- name: ListDocumentVersions :many
SELECT v.id, v.document_id, v.revision, v.author_id, v.title, v.preview, v.label, v.manual,
       v.created_at, v.updated_at,
       u.name AS author_name, u.email AS author_email
FROM document_versions v
LEFT JOIN users u ON u.id = v.author_id
WHERE v.document_id = $1
ORDER BY v.revision DESC;

-- name: GetDocumentVersion :one
SELECT * FROM document_versions WHERE id = $1;

-- LatestDocumentVersion is the row the content endpoint checks on every save to
-- decide whether this edit belongs to the open session or starts a new one.
-- name: LatestDocumentVersion :one
SELECT * FROM document_versions WHERE document_id = $1 ORDER BY revision DESC LIMIT 1;

-- ExtendDocumentVersion folds another save of the same editing session into the
-- version that session opened, instead of adding a row per autosave debounce.
-- name: ExtendDocumentVersion :one
UPDATE document_versions
SET content = $2, preview = $3, title = $4, updated_at = now()
WHERE id = $1
RETURNING *;

-- PruneDocumentVersions caps the automatic history of one document, keeping the
-- newest @keep session snapshots. Manual versions are excluded from both the
-- deletion and the count: they are milestones somebody asked to keep, and an
-- afternoon of typing must not push them out of the journal. They are not
-- unbounded either — see PruneManualDocumentVersions below.
-- name: PruneDocumentVersions :exec
DELETE FROM document_versions
WHERE document_versions.document_id = @document_id
  AND document_versions.manual = false
  AND document_versions.id NOT IN (
      SELECT v.id FROM document_versions v
      WHERE v.document_id = @document_id AND v.manual = false
      ORDER BY v.revision DESC
      LIMIT @keep
  );

-- PruneManualDocumentVersions caps the *manual* half of the journal, which had no
-- bound at all: nothing prunes it, there is no endpoint that deletes a version,
-- and three paths write into it — the explicit snapshot button, a rollback (two
-- rows per click: "Перед откатом" and "Откат к версии N") and raising an approval
-- route. Each row is a full copy of the body, so an afternoon of rollbacks is tens
-- of megabytes nobody can reclaim.
--
-- A version a route pinned is never touched: document_approvals.version_id is
-- ON DELETE RESTRICT, so deleting one would not orphan the route — it would fail
-- the whole statement, and with it the snapshot the user just asked for.
-- name: PruneManualDocumentVersions :exec
DELETE FROM document_versions
WHERE document_versions.document_id = @document_id
  AND document_versions.manual = true
  AND NOT EXISTS (
      SELECT 1 FROM document_approvals a WHERE a.version_id = document_versions.id
  )
  AND document_versions.id NOT IN (
      SELECT v.id FROM document_versions v
      WHERE v.document_id = @document_id AND v.manual = true
      ORDER BY v.revision DESC
      LIMIT @keep
  );
