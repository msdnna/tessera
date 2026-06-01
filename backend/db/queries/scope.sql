-- Scope resolvers: map a nested resource back to its owning workspace so
-- handlers can authorize via workspace membership.

-- name: WorkspaceIDForProject :one
SELECT workspace_id FROM projects WHERE id = $1;

-- name: WorkspaceIDForBoard :one
SELECT p.workspace_id
FROM boards b JOIN projects p ON p.id = b.project_id
WHERE b.id = $1;

-- name: WorkspaceIDForColumn :one
SELECT p.workspace_id
FROM board_columns c
JOIN boards b ON b.id = c.board_id
JOIN projects p ON p.id = b.project_id
WHERE c.id = $1;

-- name: WorkspaceIDForTask :one
SELECT p.workspace_id
FROM tasks t
JOIN boards b ON b.id = t.board_id
JOIN projects p ON p.id = b.project_id
WHERE t.id = $1;
