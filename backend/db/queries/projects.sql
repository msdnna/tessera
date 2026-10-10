-- name: CreateProject :one
INSERT INTO projects (workspace_id, group_id, name, color, icon, slug, position)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- ListProjects: created_at/id break position ties — see ListColumns
-- (boards.sql); the sidebar orders itself by the same midpoint scheme.
-- name: ListProjects :many
-- The sidebar tree: archived projects (0086) are out of it — they open from the
-- «Архив проектов» list (ListArchivedProjects) only.
SELECT * FROM projects WHERE workspace_id = $1 AND archived_at IS NULL ORDER BY position, created_at, id;

-- name: ListArchivedProjects :many
SELECT * FROM projects WHERE workspace_id = $1 AND archived_at IS NOT NULL ORDER BY archived_at DESC, name;

-- name: ArchiveProject :one
UPDATE projects SET archived_at = now(), updated_at = now()
WHERE id = $1 AND archived_at IS NULL
RETURNING *;

-- name: UnarchiveProject :one
UPDATE projects SET archived_at = NULL, updated_at = now()
WHERE id = $1 AND archived_at IS NOT NULL
RETURNING *;

-- ProjectArchivedForBoard tells a write on a board (or a task on it) whether the
-- board's project is in the archive (0086) — read-only there.
-- name: ProjectArchivedForBoard :one
SELECT (p.archived_at IS NOT NULL)::boolean
FROM boards b JOIN projects p ON p.id = b.project_id
WHERE b.id = $1;

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

-- RebalanceProjectPositions respaces a workspace's projects onto a fresh
-- gap * 1,2,3… ladder, keeping the order they already had. Workspace-scoped for
-- the same reason as the group one: ListProjects orders every project of the
-- workspace by position and the client groups them, so respacing all of them
-- preserves each group's sibling order. MoveProject calls it when the midpoint
-- collapses onto a neighbour.
-- name: RebalanceProjectPositions :exec
UPDATE projects t
SET position = ordered.rn * sqlc.arg(gap)::double precision, updated_at = now()
FROM (
    SELECT id, row_number() OVER (ORDER BY position, created_at, id) AS rn
    FROM projects WHERE workspace_id = sqlc.arg(workspace_id)::uuid
) ordered
WHERE t.id = ordered.id;

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

-- ── Old project addresses (#3012) ─────────────────────────────

-- GetProjectByAlias resolves a project's former address while it still leads
-- somewhere. Callers try GetProjectBySlug first: a live project's slug always
-- wins over an alias.
-- name: GetProjectByAlias :one
SELECT p.* FROM projects p
JOIN project_slug_aliases a ON a.project_id = p.id
WHERE a.slug = sqlc.arg(slug)::text AND a.expires_at > now();

-- UpsertProjectSlugAlias records the address a project is leaving. A slug the
-- project (or another one) left before is re-pointed and its clock restarted.
-- name: UpsertProjectSlugAlias :exec
INSERT INTO project_slug_aliases (slug, project_id, expires_at)
VALUES ($1, $2, $3)
ON CONFLICT (slug) DO UPDATE
SET project_id = EXCLUDED.project_id, expires_at = EXCLUDED.expires_at, created_at = now();

-- DeleteProjectSlugAlias drops the alias on an address a project has just
-- taken: the live slug shadows it anyway, and a stale row would make the
-- address lead elsewhere again the day its new holder moves on.
-- name: DeleteProjectSlugAlias :exec
DELETE FROM project_slug_aliases WHERE slug = $1;

-- name: PurgeExpiredProjectSlugAliases :execrows
DELETE FROM project_slug_aliases WHERE expires_at <= now();

-- ── Counts for the delete confirmations (#3014) ───────────────
-- What a delete takes with it, as its confirmation names it: the boards still
-- on the tree (one in the trash is already gone for the reader) and every task
-- on them — subtasks and archived ones too, since the cascade takes those.

-- name: ProjectFootprint :one
SELECT
    (SELECT count(*) FROM boards b
      WHERE b.project_id = sqlc.arg(project_id)::uuid AND b.deleted_at IS NULL)::bigint AS boards,
    (SELECT count(*) FROM tasks t JOIN boards b ON b.id = t.board_id
      WHERE b.project_id = sqlc.arg(project_id)::uuid AND b.deleted_at IS NULL)::bigint AS tasks;

-- name: WorkspaceFootprint :one
SELECT
    (SELECT count(*) FROM projects p
      WHERE p.workspace_id = sqlc.arg(workspace_id)::uuid)::bigint AS projects,
    (SELECT count(*) FROM boards b JOIN projects p ON p.id = b.project_id
      WHERE p.workspace_id = sqlc.arg(workspace_id)::uuid AND b.deleted_at IS NULL)::bigint AS boards,
    (SELECT count(*) FROM tasks t JOIN boards b ON b.id = t.board_id JOIN projects p ON p.id = b.project_id
      WHERE p.workspace_id = sqlc.arg(workspace_id)::uuid AND b.deleted_at IS NULL)::bigint AS tasks;
