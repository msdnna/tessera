-- ListWorkspaceTasks returns active tasks across a workspace's boards, with location
-- names and tag/assignee ids — backs "My tasks" / "All tasks" and the home summary
-- (feature #1). By default only top-level tasks; set include_subtasks to also return
-- subtasks (used by the relation picker so subtasks can be linked to each other).
-- name: ListWorkspaceTasks :many
SELECT
    t.*,
    b.name AS board_name,
    p.name AS project_name,
    p.color AS project_color,
    c.name AS column_name,
    c.name_key AS column_name_key,
    c.color AS column_color,
    COALESCE(array_agg(DISTINCT tt.tag_id) FILTER (WHERE tt.tag_id IS NOT NULL), '{}')::uuid[] AS tag_ids,
    COALESCE(array_agg(DISTINCT ta.user_id) FILTER (WHERE ta.user_id IS NOT NULL), '{}')::uuid[] AS assignee_ids
FROM tasks t
JOIN boards b ON b.id = t.board_id
JOIN projects p ON p.id = b.project_id
JOIN board_columns c ON c.id = t.column_id
LEFT JOIN task_tags tt ON tt.task_id = t.id
LEFT JOIN task_assignees ta ON ta.task_id = t.id
WHERE p.workspace_id = @workspace_id
    AND (@include_subtasks::bool OR t.parent_id IS NULL)
    AND t.archived_at IS NULL
GROUP BY t.id, b.name, p.name, p.color, c.name, c.name_key, c.color
ORDER BY t.due_date NULLS LAST, t.created_at DESC;

-- WorkspaceTaskSummary counts the home-screen headline numbers in a single pass over
-- the workspace's tasks. It replaces loading every task row (t.* + 5 joins + two
-- array_agg) into memory just to derive eight integers.
--
-- The day boundaries come in as parameters rather than being derived with date_trunc:
-- the caller buckets a due date in Go, and comparing against caller-supplied midnights
-- keeps that bucketing byte-identical (see WorkspaceSummary). board_columns is not
-- joined — tasks.column_id is a NOT NULL FK, so the join never filtered anything.
-- name: WorkspaceTaskSummary :one
SELECT
    count(*) AS total,
    count(*) FILTER (WHERE t.completed_at IS NOT NULL) AS completed,
    count(*) FILTER (
        WHERE EXISTS (SELECT 1 FROM task_assignees ta WHERE ta.task_id = t.id AND ta.user_id = @user_id)
    ) AS assigned,
    count(*) FILTER (
        WHERE t.completed_at IS NULL AND t.due_date < @day_start::timestamptz
    ) AS overdue,
    count(*) FILTER (
        WHERE t.completed_at IS NULL
          AND t.due_date >= @day_start::timestamptz
          AND t.due_date < @next_day::timestamptz
    ) AS due_today,
    count(*) FILTER (
        WHERE t.completed_at IS NULL
          AND t.due_date >= @day_start::timestamptz
          AND t.due_date < @week_end::timestamptz
    ) AS due_week,
    count(*) FILTER (
        WHERE NOT EXISTS (SELECT 1 FROM task_assignees ta WHERE ta.task_id = t.id)
    ) AS unassigned
FROM tasks t
JOIN boards b ON b.id = t.board_id
JOIN projects p ON p.id = b.project_id
WHERE p.workspace_id = @workspace_id
    AND t.parent_id IS NULL
    AND t.archived_at IS NULL;
