-- name: CreateProjectGroup :one
INSERT INTO project_groups (workspace_id, parent_id, name, position)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListProjectGroups :many
SELECT * FROM project_groups WHERE workspace_id = $1 ORDER BY position;

-- name: GetProjectGroup :one
SELECT * FROM project_groups WHERE id = $1;

-- name: MaxProjectGroupPosition :one
SELECT coalesce(max(position), 0)::double precision FROM project_groups WHERE workspace_id = $1;

-- name: UpdateProjectGroup :one
UPDATE project_groups
SET name = $2, icon = $3, color = $4, icon_mode = $5, updated_at = now()
WHERE id = $1
RETURNING *;

-- ProjectGroupAncestorIDs returns the group itself followed by its ancestors, up
-- to the root. Used to reject re-parenting a group into its own subtree, which
-- would leave the branch in a cycle with no root: the sidebar builds the tree by
-- recursion on parent_id, so a cycle is not a cosmetic oddity but an unbounded
-- one. Mirrors DocumentAncestorIDs — the membership test is done in Go for the
-- same reason (sqlc cannot resolve a recursive CTE inside a subquery).
--
-- UNION rather than UNION ALL: an install that already grew a cycle through the
-- unguarded handler must still get an answer here instead of spinning, and
-- dedup on the row is what stops the walk when it comes back around.
-- name: ProjectGroupAncestorIDs :many
WITH RECURSIVE up AS (
    SELECT a.id, a.parent_id FROM project_groups a WHERE a.id = $1
    UNION
    SELECT g.id, g.parent_id FROM project_groups g JOIN up ON g.id = up.parent_id
)
SELECT id FROM up;

-- name: MoveProjectGroup :one
UPDATE project_groups
SET parent_id = $2, position = $3, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteProjectGroup :exec
DELETE FROM project_groups WHERE id = $1;
