-- name: CreateTag :one
INSERT INTO tags (workspace_id, project_id, name, color)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListTags :many
SELECT * FROM tags WHERE project_id = $1 ORDER BY name;

-- ListWorkspaceTags returns every tag across all projects in a workspace, for
-- read-only views that span projects (Home, cross-project task lists).
-- name: ListWorkspaceTags :many
SELECT * FROM tags WHERE workspace_id = $1 ORDER BY name;

-- name: GetTag :one
SELECT * FROM tags WHERE id = $1;

-- EnsureTag returns the project tag with this name, creating it (with the given
-- color) if absent. On conflict it refreshes the colour only when a non-empty
-- one is supplied, so the GitLab sync keeps label colours current without wiping
-- a colour set elsewhere. Used by the GitLab sync.
-- name: EnsureTag :one
INSERT INTO tags (workspace_id, project_id, name, color)
VALUES ($1, $2, $3, $4)
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

-- name: AddTaskTag :exec
INSERT INTO task_tags (task_id, tag_id) VALUES ($1, $2) ON CONFLICT DO NOTHING;

-- name: RemoveTaskTag :exec
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
