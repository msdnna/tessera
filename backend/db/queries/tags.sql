-- name: CreateTag :one
INSERT INTO tags (workspace_id, project_id, name, color)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListTags :many
SELECT * FROM tags WHERE project_id = $1 ORDER BY name;

-- ListTagsWithCounts is ListTags with the number of live tasks wearing each tag
-- — the «Задачи» column of the project window's tag table (#2990). Archived
-- tasks and tasks of deleted boards are not counted, like everywhere else a
-- figure is shown.
-- name: ListTagsWithCounts :many
SELECT g.*,
       (SELECT COUNT(*) FROM task_tags tt
        JOIN tasks t ON t.id = tt.task_id AND t.archived_at IS NULL
        JOIN boards b ON b.id = t.board_id AND b.deleted_at IS NULL
        WHERE tt.tag_id = g.id) AS task_count
FROM tags g
WHERE g.project_id = $1
ORDER BY g.name;

-- ListWorkspaceTags returns every tag across all projects in a workspace, for
-- read-only views that span projects (Home, cross-project task lists).
-- name: ListWorkspaceTags :many
SELECT * FROM tags WHERE workspace_id = $1 ORDER BY name;

-- name: GetTag :one
SELECT * FROM tags WHERE id = $1;

-- GetProjectTagByName finds a project's tag the way every READER of a tag name
-- compares one: trimmed and case-insensitively. The storage constraint is
-- byte-exact (UNIQUE (project_id, name), migration 0023) while nothing downstream
-- is: `canonPrefix` lowercases a namespace on all three clients, the web picker's
-- `matchTagByName` resolves a typed name case-insensitively, and the MCP
-- `resolveTags` does the same — so «Сложность::высокая» and «сложность::высокая»
-- were two rows that every consumer treats as one name and the board draws as two
-- lanes with the SAME caption (under a narrowed prefix `tagLaneLabel` prints only
-- the value), splitting the killer feature's cards between them.
--
-- Oldest first, so with twins already in the table the answer is stable and the
-- original row is the one everything collapses onto (#2932).
-- name: GetProjectTagByName :one
SELECT * FROM tags
WHERE project_id = $1 AND lower(btrim(name)) = lower(btrim(@name::text))
ORDER BY created_at, id
LIMIT 1;

-- SetTagColor refreshes just the colour. Used by the GitLab sync when the label it
-- is ensuring already exists under a different spelling: the row keeps the name the
-- project gave it, only the colour follows GitLab.
-- name: SetTagColor :one
UPDATE tags SET color = $2 WHERE id = $1 RETURNING *;

-- EnsureTag returns the project tag with this name, creating it (with the given
-- color) if absent. On conflict it refreshes the colour only when a non-empty
-- one is supplied, so the GitLab sync keeps label colours current without wiping
-- a colour set elsewhere. Used by the GitLab sync.
--
-- A tag it creates is marked source = 'gitlab' (#3028); one that already existed
-- keeps the origin it has — a hand-made tag GitLab later starts using stays the
-- project's own.
-- name: EnsureTag :one
INSERT INTO tags (workspace_id, project_id, name, color, source)
VALUES ($1, $2, $3, $4, 'gitlab')
ON CONFLICT (project_id, name) DO UPDATE
SET color = COALESCE(NULLIF(EXCLUDED.color, ''), tags.color)
RETURNING *;

-- name: UpdateTag :one
UPDATE tags SET name = $2, color = $3 WHERE id = $1 RETURNING *;

-- name: DeleteTag :exec
DELETE FROM tags WHERE id = $1;

-- ReassignProjectTagsWorkspace re-stamps the denormalized workspace_id on all of
-- a project's tags. Used when a project is transferred between workspaces.
-- name: ReassignProjectTagsWorkspace :exec
UPDATE tags SET workspace_id = $2 WHERE project_id = $1;

-- DeleteTaskTagsAcrossWorkspaces drops every task↔tag pairing with one end in
-- the given project and the other in a different workspace — the state a project
-- transfer leaves behind. applyTag lets a tag cross *projects* inside one
-- workspace on purpose (the board views rely on it) and refuses one from another
-- workspace, so such a pair can only appear when the boundary moves under an
-- existing pairing; ListTaskTags joins tags straight through task_tags with no
-- workspace filter of its own, so leaving the row in place shows each side the
-- other's tag name and colour. Only the pairing goes — both tags survive, each
-- in its own workspace. Returns the number of pairings dropped.
-- name: DeleteTaskTagsAcrossWorkspaces :execrows
DELETE FROM task_tags tt
USING tasks t, boards b, projects p, tags g
WHERE tt.task_id = t.id
  AND b.id = t.board_id
  AND p.id = b.project_id
  AND g.id = tt.tag_id
  AND (p.id = $1 OR g.project_id = $1)
  AND p.workspace_id <> g.workspace_id;

-- Both return the number of rows they touched (1 = the tag really was attached /
-- detached, 0 = the task already was in that state), so the journal records an
-- actual change rather than a repeated request — the same reason
-- AddTaskTagSourced counts rows for the sync journal.
-- name: AddTaskTag :execrows
INSERT INTO task_tags (task_id, tag_id) VALUES ($1, $2) ON CONFLICT DO NOTHING;

-- name: RemoveTaskTag :execrows
DELETE FROM task_tags WHERE task_id = $1 AND tag_id = $2;

-- name: ListTaskTags :many
SELECT t.*
FROM tags t
JOIN task_tags tt ON tt.tag_id = t.id
WHERE tt.task_id = $1
ORDER BY t.name;

-- name: AddTaskAssignee :exec
INSERT INTO task_assignees (task_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING;

-- name: RemoveTaskAssignee :exec
DELETE FROM task_assignees WHERE task_id = $1 AND user_id = $2;

-- StripNonMemberAssignees removes assignees from a project's tasks who are not
-- members of the given (target) workspace. Used after a project is transferred so
-- no dangling assignments remain. Returns the number of rows removed.
-- name: StripNonMemberAssignees :execrows
DELETE FROM task_assignees ta
USING tasks t, boards b
WHERE ta.task_id = t.id
  AND t.board_id = b.id
  AND b.project_id = $1
  AND NOT EXISTS (
    SELECT 1 FROM memberships m
    WHERE m.workspace_id = $2 AND m.user_id = ta.user_id
  );

-- name: ListTaskAssignees :many
SELECT u.id, u.email, u.name
FROM users u
JOIN task_assignees ta ON ta.user_id = u.id
WHERE ta.task_id = $1
ORDER BY u.name;

-- ── merge and bulk delete (#2990) ──────────────────────────────

-- TagTaskLinks lists who wears a tag and how it got there — what a merge
-- journals and what a bulk delete hands back so the undo can put it back.
-- name: TagTaskLinks :many
SELECT tt.task_id, tt.source FROM task_tags tt WHERE tt.tag_id = $1;

-- MergeTagLinks moves a source tag's tasks onto the target: every task wearing
-- the source gets the target (keeping how the source got there), and returns
-- the tasks that did not have the target yet. The source row is deleted
-- separately (its own task_tags go with it by cascade).
-- name: MergeTagLinks :many
INSERT INTO task_tags (task_id, tag_id, source)
SELECT tt.task_id, @target_id, tt.source FROM task_tags tt WHERE tt.tag_id = @source_id
ON CONFLICT (task_id, tag_id) DO NOTHING
RETURNING task_id;

-- RestoreTag puts a deleted tag back under its old id (the undo of a bulk
-- delete). Nothing is written if the id is taken; a name taken in the meantime
-- fails on the unique index and the caller skips that tag.
-- name: RestoreTag :one
INSERT INTO tags (id, workspace_id, project_id, name, color, source)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (id) DO NOTHING
RETURNING *;

-- RestoreTaskTag re-attaches a restored tag to a task — only to a task of the
-- tag's own workspace that still exists.
-- name: RestoreTaskTag :execrows
INSERT INTO task_tags (task_id, tag_id, source)
SELECT t.id, @tag_id, @source::text
FROM tasks t JOIN boards b ON b.id = t.board_id JOIN projects p ON p.id = b.project_id
WHERE t.id = @task_id AND p.workspace_id = @workspace_id
ON CONFLICT (task_id, tag_id) DO NOTHING;
