-- name: CreateTask :one
INSERT INTO tasks (
    board_id, column_id, parent_id, title, description, priority, due_date, position, created_by
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: GetTask :one
SELECT * FROM tasks WHERE id = $1;

-- name: ListTasksByBoard :many
SELECT * FROM tasks WHERE board_id = $1 AND parent_id IS NULL ORDER BY position;

-- ListBoardTasksWithMeta returns top-level board tasks with their tag and
-- assignee ids aggregated, so the kanban can render chips and group by tag
-- without an extra round-trip per card.
-- name: ListBoardTasksWithMeta :many
SELECT
    t.*,
    COALESCE(array_agg(DISTINCT tt.tag_id) FILTER (WHERE tt.tag_id IS NOT NULL), '{}')::uuid[] AS tag_ids,
    COALESCE(array_agg(DISTINCT ta.user_id) FILTER (WHERE ta.user_id IS NOT NULL), '{}')::uuid[] AS assignee_ids
FROM tasks t
LEFT JOIN task_tags tt ON tt.task_id = t.id
LEFT JOIN task_assignees ta ON ta.task_id = t.id
WHERE t.board_id = $1 AND t.parent_id IS NULL
GROUP BY t.id
ORDER BY t.position;

-- name: ListSubtasks :many
SELECT * FROM tasks WHERE parent_id = $1 ORDER BY position;

-- name: MaxTaskPositionInColumn :one
SELECT coalesce(max(position), 0)::double precision
FROM tasks WHERE column_id = $1 AND parent_id IS NULL;

-- name: UpdateTask :one
UPDATE tasks
SET title = $2, description = $3, priority = $4, due_date = $5, completed_at = $6, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: MoveTask :one
UPDATE tasks
SET column_id = $2, position = $3, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteTask :exec
DELETE FROM tasks WHERE id = $1;
