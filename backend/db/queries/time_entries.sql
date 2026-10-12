-- name: AddTimeEntry :one
INSERT INTO task_time_entries (task_id, user_id, minutes, note, spent_on)
VALUES ($1, $2, $3, $4, COALESCE(sqlc.narg(spent_on)::date, CURRENT_DATE))
RETURNING *;

-- name: ListTimeEntries :many
SELECT e.id, e.task_id, e.user_id, e.minutes, e.note, e.spent_on, e.created_at,
       COALESCE(u.name, '')::text AS user_name
FROM task_time_entries e
LEFT JOIN users u ON u.id = e.user_id
WHERE e.task_id = $1
ORDER BY e.spent_on DESC, e.created_at DESC;

-- name: GetTimeEntry :one
SELECT * FROM task_time_entries WHERE id = $1 AND task_id = $2;

-- name: DeleteTimeEntry :execrows
DELETE FROM task_time_entries WHERE id = $1 AND task_id = $2;

-- name: TaskSpentMinutes :one
-- «Потрачено» (#2965): the task's own minutes and the whole subtree's — subtasks
-- in the trash (0086) don't count.
WITH RECURSIVE tree AS (
    SELECT t.id FROM tasks t WHERE t.id = $1 AND t.deleted_at IS NULL
    UNION ALL
    SELECT c.id FROM tasks c JOIN tree ON c.parent_id = tree.id WHERE c.deleted_at IS NULL
)
SELECT
    COALESCE((SELECT SUM(o.minutes) FROM task_time_entries o WHERE o.task_id = $1), 0)::bigint AS own,
    COALESCE((SELECT SUM(e.minutes) FROM task_time_entries e JOIN tree ON e.task_id = tree.id), 0)::bigint AS total;
