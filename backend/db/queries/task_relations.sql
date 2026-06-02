-- name: GetTaskByNumber :one
SELECT t.*
FROM tasks t
JOIN boards b ON b.id = t.board_id
JOIN projects p ON p.id = b.project_id
WHERE p.workspace_id = $1 AND t.number = $2;

-- name: AddTaskRelation :one
INSERT INTO task_relations (task_id, related_task_id, kind)
VALUES ($1, $2, $3)
ON CONFLICT (task_id, related_task_id, kind) DO NOTHING
RETURNING *;

-- name: ListTaskRelations :many
SELECT
    r.id, r.task_id, r.related_task_id, r.kind, r.created_at,
    t.number AS related_number,
    t.title  AS related_title,
    t.board_id AS related_board_id,
    t.completed_at AS related_completed_at,
    t.archived_at  AS related_archived_at
FROM task_relations r
JOIN tasks t ON t.id = r.related_task_id
WHERE r.task_id = $1
ORDER BY r.created_at;

-- name: GetTaskRelation :one
SELECT * FROM task_relations WHERE id = $1;

-- name: DeleteTaskRelation :exec
DELETE FROM task_relations WHERE id = $1;
