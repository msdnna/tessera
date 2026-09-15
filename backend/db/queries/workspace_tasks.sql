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
--
-- A due that lands exactly on UTC midnight is not an instant at all: it is the
-- codebase's date-only value (a GitLab issue due, `parseDate` in internal/gitlab,
-- or a milestone's). Both clients read one in UTC in every zone — `isDateOnly` in
-- frontend/src/utils/format.js, `isUtcMidnight` behind `dueDayKey` in Dates.kt —
-- and so does the server elsewhere (`taskDateStr`, handlers/gitlab_conflict.go).
-- Comparing it as an instant against the caller's midnight instead slid it to the
-- previous day for every reader WEST of UTC: the tile said «Просрочено» while the
-- row right under it, bucketed by the client, read «Сегодня» and stayed black.
-- Hence `due_day`: a bare date for those values, compared against the caller's
-- own calendar day (@today_day), with everything else still compared as instants.
--
-- Every count here is the size of a list the client can open: each number is a
-- clickable card on the home screen that switches the list below it. So `assigned`
-- excludes completed tasks for the same reason `overdue` does — both clients' "Мои"
-- list is open tasks only (HomeView.vue, HomeUiState.visibleTasks), and a card that
-- claims eight while the list under it shows five is worse than no card.
-- name: WorkspaceTaskSummary :one
WITH scoped AS (
    SELECT
        t.id,
        t.completed_at,
        t.due_date,
        CASE WHEN t.due_date = date_trunc('day', t.due_date AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'
             THEN (t.due_date AT TIME ZONE 'UTC')::date
             ELSE NULL::date END AS due_day
    FROM tasks t
    JOIN boards b ON b.id = t.board_id
    JOIN projects p ON p.id = b.project_id
    WHERE p.workspace_id = @workspace_id
        AND t.parent_id IS NULL
        AND t.archived_at IS NULL
)
SELECT
    count(*) AS total,
    count(*) FILTER (WHERE t.completed_at IS NOT NULL) AS completed,
    count(*) FILTER (
        WHERE t.completed_at IS NULL
          AND EXISTS (SELECT 1 FROM task_assignees ta WHERE ta.task_id = t.id AND ta.user_id = @user_id)
    ) AS assigned,
    count(*) FILTER (
        WHERE t.completed_at IS NULL
          AND CASE WHEN t.due_day IS NOT NULL
                   THEN t.due_day < @today_day::date
                   ELSE t.due_date < @day_start::timestamptz END
    ) AS overdue,
    count(*) FILTER (
        WHERE t.completed_at IS NULL
          AND CASE WHEN t.due_day IS NOT NULL
                   THEN t.due_day = @today_day::date
                   ELSE t.due_date >= @day_start::timestamptz
                    AND t.due_date < @next_day::timestamptz END
    ) AS due_today,
    count(*) FILTER (
        WHERE t.completed_at IS NULL
          AND CASE WHEN t.due_day IS NOT NULL
                   THEN t.due_day >= @today_day::date
                    AND t.due_day < @week_end_day::date
                   ELSE t.due_date >= @day_start::timestamptz
                    AND t.due_date < @week_end::timestamptz END
    ) AS due_week,
    count(*) FILTER (
        WHERE NOT EXISTS (SELECT 1 FROM task_assignees ta WHERE ta.task_id = t.id)
    ) AS unassigned
FROM scoped t;
