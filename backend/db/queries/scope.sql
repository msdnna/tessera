-- Scope resolvers: map a nested resource back to its owning workspace so
-- handlers can authorize via workspace membership.

-- name: WorkspaceIDForProject :one
SELECT workspace_id FROM projects WHERE id = $1;

-- name: ProjectIDForBoard :one
SELECT project_id FROM boards WHERE id = $1 AND deleted_at IS NULL;

-- name: WorkspaceIDForBoard :one
SELECT p.workspace_id
FROM boards b JOIN projects p ON p.id = b.project_id
WHERE b.id = $1 AND b.deleted_at IS NULL;

-- WorkspaceIDForDeletedBoard / ForDeletedColumn authorize a restore: the plain
-- resolvers above treat a soft-deleted row as missing (0078).
-- name: WorkspaceIDForDeletedBoard :one
SELECT p.workspace_id
FROM boards b JOIN projects p ON p.id = b.project_id
WHERE b.id = $1 AND b.deleted_at IS NOT NULL;

-- name: WorkspaceIDForColumn :one
SELECT p.workspace_id
FROM board_columns c
JOIN boards b ON b.id = c.board_id
JOIN projects p ON p.id = b.project_id
WHERE c.id = $1 AND c.deleted_at IS NULL AND b.deleted_at IS NULL;

-- name: WorkspaceIDForDeletedColumn :one
SELECT p.workspace_id
FROM board_columns c
JOIN boards b ON b.id = c.board_id
JOIN projects p ON p.id = b.project_id
WHERE c.id = $1 AND c.deleted_at IS NOT NULL AND b.deleted_at IS NULL;

-- name: WorkspaceIDForTask :one
SELECT p.workspace_id
FROM tasks t
JOIN boards b ON b.id = t.board_id
JOIN projects p ON p.id = b.project_id
WHERE t.id = $1 AND b.deleted_at IS NULL AND t.deleted_at IS NULL;
