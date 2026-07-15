-- name: ListTagPrefixes :many
SELECT * FROM tag_prefixes WHERE project_id = $1 ORDER BY prefix;

-- ListWorkspaceTagPrefixes returns prefix names across all projects in a
-- workspace, for cross-project views (Home). Same prefix may repeat per project.
-- name: ListWorkspaceTagPrefixes :many
SELECT * FROM tag_prefixes WHERE workspace_id = $1 ORDER BY prefix;

-- name: UpsertTagPrefix :one
INSERT INTO tag_prefixes (project_id, workspace_id, prefix, label)
VALUES ($1, $2, $3, $4)
ON CONFLICT (project_id, prefix) DO UPDATE
SET label = EXCLUDED.label, updated_at = now()
RETURNING *;

-- name: DeleteTagPrefixesForProject :exec
DELETE FROM tag_prefixes WHERE project_id = $1;

-- ReassignProjectTagPrefixesWorkspace re-stamps workspace_id on a project's tag
-- prefixes, used when the project is transferred between workspaces.
-- name: ReassignProjectTagPrefixesWorkspace :exec
UPDATE tag_prefixes SET workspace_id = $2 WHERE project_id = $1;
