-- name: CreateNote :one
INSERT INTO notes (workspace_id, project_id, author_id, title, body, slug, task_id, visibility, tags, mentions)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, @tags::text[], @mentions::uuid[])
RETURNING *;

-- ListNotes lists the notes of a workspace the viewer may read: the shared ones
-- and their own private ones (0081). A private note is its author's alone — it
-- never shows up in a colleague's list, search or socket.
-- name: ListNotes :many
SELECT * FROM notes
WHERE workspace_id = @workspace_id
  AND (visibility = 'workspace' OR author_id = @viewer::uuid)
ORDER BY updated_at DESC;

-- InboxNotes is ListNotes for the «Входящие» feed (#2972), capped: the feed is a
-- list of recent records, not an archive.
-- name: InboxNotes :many
SELECT * FROM notes
WHERE workspace_id = @workspace_id
  AND (visibility = 'workspace' OR author_id = @viewer::uuid)
ORDER BY updated_at DESC
LIMIT 300;

-- ListTaskNotes lists the notes tied to a task that the viewer may read.
-- name: ListTaskNotes :many
SELECT * FROM notes
WHERE task_id = @task_id
  AND (visibility = 'workspace' OR author_id = @viewer::uuid)
ORDER BY updated_at DESC;

-- name: NoteSlugExists :one
SELECT EXISTS(SELECT 1 FROM notes WHERE workspace_id = $1 AND slug = $2);

-- name: NotesMissingSlug :many
SELECT id, workspace_id, title FROM notes WHERE slug = '';

-- name: SetNoteSlug :exec
UPDATE notes SET slug = $2 WHERE id = $1;

-- name: GetNote :one
SELECT * FROM notes WHERE id = $1;

-- name: UpdateNote :one
UPDATE notes
SET title = $2, body = $3, task_id = $4, visibility = $5,
    tags = @tags::text[], mentions = @mentions::uuid[], updated_at = now()
WHERE id = $1
RETURNING *;

-- ReassignProjectNotesWorkspace re-stamps workspace_id on notes tied to a project,
-- used when the project is transferred between workspaces.
-- name: ReassignProjectNotesWorkspace :exec
UPDATE notes SET workspace_id = $2 WHERE project_id = $1;

-- name: DeleteNote :exec
DELETE FROM notes WHERE id = $1;

-- ProjectNoteSlugs lists the addresses a project's notes occupy, for the same
-- reason as ProjectDocumentSlugs: notes_ws_slug_key is per workspace and a
-- transfer carries the notes into one that may already use those addresses.
-- name: ProjectNoteSlugs :many
SELECT id, slug FROM notes WHERE project_id = $1 AND slug <> '' ORDER BY created_at;
