-- name: CreateProject :one
INSERT INTO projects (workspace_id, group_id, name, color, icon, slug, position)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: ListProjects :many
SELECT * FROM projects WHERE workspace_id = $1 ORDER BY position;

-- name: GetProject :one
SELECT * FROM projects WHERE id = $1;

-- name: GetProjectBySlug :one
SELECT * FROM projects WHERE slug = $1;

-- name: ProjectSlugExists :one
SELECT EXISTS(SELECT 1 FROM projects WHERE slug = $1);

-- name: ProjectsMissingSlug :many
SELECT id, name FROM projects WHERE slug = '';

-- name: SetProjectSlug :exec
UPDATE projects SET slug = $2 WHERE id = $1;

-- name: MaxProjectPosition :one
SELECT coalesce(max(position), 0)::double precision FROM projects WHERE workspace_id = $1;

-- name: UpdateProject :one
UPDATE projects
SET name = $2, color = $3, icon = $4, group_id = $5, icon_mode = $6, tree_mode = $7, updated_at = now()
WHERE id = $1
RETURNING *;

-- SetProjectEstimation stores a project's estimation-config override (NULL =
-- inherit the workspace default). Own endpoint, like the workspace one.
-- name: SetProjectEstimation :one
UPDATE projects
SET estimation = $2, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: MoveProject :one
UPDATE projects
SET group_id = $2, position = $3, updated_at = now()
WHERE id = $1
RETURNING *;

-- TransferProject moves a project to another workspace, dropping it at the root
-- (ungrouped) since groups belong to the source workspace. Denormalized
-- workspace_id on tags/tag_prefixes/notes is re-stamped separately in the same tx.
-- name: TransferProject :one
UPDATE projects
SET workspace_id = $2, group_id = NULL, position = $3, updated_at = now()
WHERE id = $1
RETURNING *;

-- CountProjectTasks counts every task on the project's boards — subtasks and
-- archived rows included, since both carry a number and an archived task can be
-- restored into a workspace that has since reused it.
-- name: CountProjectTasks :one
SELECT count(*) FROM tasks t JOIN boards b ON b.id = t.board_id WHERE b.project_id = $1;

-- RenumberProjectTasks re-stamps the project's tasks with a block of numbers
-- reserved from the target workspace (base is the number *before* the block).
-- Ordering by the old number keeps the re-stamp a translation rather than a
-- shuffle: a task that was #7 where it came from still sorts before the one that
-- was #9. created_at/id only break ties for rows predating the number column.
-- name: RenumberProjectTasks :exec
UPDATE tasks SET number = sqlc.arg(base)::bigint + ordered.rn
FROM (
    SELECT t.id, row_number() OVER (ORDER BY t.number NULLS LAST, t.created_at, t.id) AS rn
    FROM tasks t
    JOIN boards b ON b.id = t.board_id
    WHERE b.project_id = $1
) AS ordered
WHERE tasks.id = ordered.id;

-- name: DeleteProject :exec
DELETE FROM projects WHERE id = $1;
