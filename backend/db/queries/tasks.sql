-- name: CreateTask :one
INSERT INTO tasks (
    board_id, column_id, parent_id, title, description, priority, due_date, position, created_by, number
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
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
    COALESCE(array_agg(DISTINCT ta.user_id) FILTER (WHERE ta.user_id IS NOT NULL), '{}')::uuid[] AS assignee_ids,
    COALESCE(array_agg(DISTINCT ga.gl_name) FILTER (WHERE ga.gl_name IS NOT NULL), '{}')::text[] AS gitlab_assignees,
    gl.gl_iid AS gitlab_iid,
    gl.gl_web_url AS gitlab_url,
    gl.gl_author AS gitlab_author,
    gl.gl_author_name AS gitlab_author_name,
    gl.gl_author_avatar_url AS gitlab_author_avatar_url
FROM tasks t
LEFT JOIN task_tags tt ON tt.task_id = t.id
LEFT JOIN task_assignees ta ON ta.task_id = t.id
LEFT JOIN task_gitlab_assignees ga ON ga.task_id = t.id
LEFT JOIN gitlab_links gl ON gl.task_id = t.id
WHERE t.board_id = $1 AND t.parent_id IS NULL AND t.archived_at IS NULL
GROUP BY t.id, gl.gl_iid, gl.gl_web_url, gl.gl_author, gl.gl_author_name, gl.gl_author_avatar_url
ORDER BY t.position;

-- name: ListSubtasks :many
SELECT * FROM tasks WHERE parent_id = $1 ORDER BY position;

-- ListSubtasksWithMeta returns a parent's subtasks with tag/assignee ids, so the
-- task modal can render a kanban-style hover card for each one.
-- name: ListSubtasksWithMeta :many
SELECT
    t.*,
    COALESCE(array_agg(DISTINCT tt.tag_id) FILTER (WHERE tt.tag_id IS NOT NULL), '{}')::uuid[] AS tag_ids,
    COALESCE(array_agg(DISTINCT ta.user_id) FILTER (WHERE ta.user_id IS NOT NULL), '{}')::uuid[] AS assignee_ids
FROM tasks t
LEFT JOIN task_tags tt ON tt.task_id = t.id
LEFT JOIN task_assignees ta ON ta.task_id = t.id
WHERE t.parent_id = $1
GROUP BY t.id
ORDER BY t.position;

-- ListBoardSubtasksWithMeta returns every subtask on a board (parent_id set)
-- with tag/assignee ids, so the kanban can render them under their parents.
-- name: ListBoardSubtasksWithMeta :many
SELECT
    t.*,
    COALESCE(array_agg(DISTINCT tt.tag_id) FILTER (WHERE tt.tag_id IS NOT NULL), '{}')::uuid[] AS tag_ids,
    COALESCE(array_agg(DISTINCT ta.user_id) FILTER (WHERE ta.user_id IS NOT NULL), '{}')::uuid[] AS assignee_ids
FROM tasks t
LEFT JOIN task_tags tt ON tt.task_id = t.id
LEFT JOIN task_assignees ta ON ta.task_id = t.id
WHERE t.board_id = $1 AND t.parent_id IS NOT NULL AND t.archived_at IS NULL
GROUP BY t.id
ORDER BY t.position;

-- ListBoardArchivedWithMeta returns a board's archived tasks: top-level ones
-- and individually-archived subtasks (whose parent is still active). Children
-- archived together with their parent are hidden (they restore with it).
-- name: ListBoardArchivedWithMeta :many
SELECT
    t.*,
    COALESCE(array_agg(DISTINCT tt.tag_id) FILTER (WHERE tt.tag_id IS NOT NULL), '{}')::uuid[] AS tag_ids,
    COALESCE(array_agg(DISTINCT ta.user_id) FILTER (WHERE ta.user_id IS NOT NULL), '{}')::uuid[] AS assignee_ids
FROM tasks t
LEFT JOIN task_tags tt ON tt.task_id = t.id
LEFT JOIN task_assignees ta ON ta.task_id = t.id
WHERE t.board_id = $1
  AND t.archived_at IS NOT NULL
  AND (
    t.parent_id IS NULL
    OR NOT EXISTS (SELECT 1 FROM tasks p WHERE p.id = t.parent_id AND p.archived_at IS NOT NULL)
  )
GROUP BY t.id
ORDER BY t.archived_at DESC;

-- name: ArchiveTask :exec
UPDATE tasks SET archived_at = now(), updated_at = now() WHERE id = $1;

-- name: ArchiveTaskCascade :exec
UPDATE tasks SET archived_at = now(), updated_at = now() WHERE id = $1 OR parent_id = $1;

-- name: RestoreTask :exec
UPDATE tasks SET archived_at = NULL, updated_at = now() WHERE id = $1 OR parent_id = $1;

-- name: TransferTask :one
UPDATE tasks
SET board_id = $2, column_id = $3, parent_id = NULL, position = $4, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: MoveSubtasksToBoard :exec
UPDATE tasks SET board_id = $2, column_id = $3, updated_at = now() WHERE parent_id = $1;

-- name: SetTaskParent :one
UPDATE tasks
SET parent_id = $2, board_id = $3, column_id = $4, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DetachChildren :exec
UPDATE tasks SET parent_id = NULL, updated_at = now() WHERE parent_id = $1;

-- name: MaxTaskPositionInColumn :one
SELECT coalesce(max(position), 0)::double precision
FROM tasks WHERE column_id = $1 AND parent_id IS NULL;

-- name: UpdateTask :one
UPDATE tasks
SET title = $2, description = $3, priority = $4, due_date = $5, completed_at = $6, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UpdateTaskDueDate :exec
UPDATE tasks SET due_date = $2, updated_at = now() WHERE id = $1;

-- name: MoveTask :one
UPDATE tasks
SET column_id = $2, position = $3, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteTask :exec
DELETE FROM tasks WHERE id = $1;
