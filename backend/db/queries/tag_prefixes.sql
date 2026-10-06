-- name: ListTagPrefixes :many
SELECT * FROM tag_prefixes WHERE project_id = $1 ORDER BY prefix;

-- ListWorkspaceTagPrefixes returns prefix names across all projects in a
-- workspace, for cross-project views (Home). Same prefix may repeat per project,
-- and the handler keeps the first row of each — so the order among the repeats
-- is what decides whose wording the whole workspace is shown.
--
-- Ordering by prefix alone left that to physical row order. SetTagPrefixes is a
-- DELETE-then-INSERT over the project's *whole* set, so renaming any one group
-- moved all of that project's rows to the end of the table and handed every
-- prefix it shares with a neighbour the neighbour's wording — a group header on
-- Home renaming itself after an edit that never mentioned it (#2932).
--
-- The tie-break is therefore the project, not the row: the oldest project using
-- the prefix owns the workspace-wide label, which no later edit anywhere can
-- move. id closes the case of two projects created in the same tick.
-- name: ListWorkspaceTagPrefixes :many
SELECT tp.* FROM tag_prefixes tp
JOIN projects p ON p.id = tp.project_id
WHERE tp.workspace_id = $1
ORDER BY tp.prefix, p.created_at, p.id;

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
