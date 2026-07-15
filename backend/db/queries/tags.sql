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
