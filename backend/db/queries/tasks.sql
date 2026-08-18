-- name: CreateTask :one
INSERT INTO tasks (
    board_id, column_id, parent_id, title, description, priority, due_date, start_date, estimate, position, created_by, number
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING *;

-- name: GetTask :one
SELECT * FROM tasks WHERE id = $1;

-- name: ListTasksByBoard :many
SELECT * FROM tasks WHERE board_id = $1 AND parent_id IS NULL ORDER BY position;

-- ListBoardTasksWithMeta returns top-level board tasks with their tag and
-- assignee ids aggregated, so the kanban can render chips and group by tag
-- without an extra round-trip per card.
--
-- The three M:N sets are gathered in LATERAL subqueries rather than joined. Joining
-- them produced a row per *combination* — a task with 4 tags, 3 assignees and 2 GitLab
-- assignees expanded to 24 rows, 23 of which array_agg(DISTINCT …) threw away — and
-- this is the board's hot query. Each LATERAL is an index scan on the table's
-- (task_id, …) primary key, and the output is one row per task. gitlab_links stays a
-- plain LEFT JOIN: task_id is its primary key, so it is 1:1 and never multiplied rows.
-- DISTINCT is kept inside the aggregates so the arrays are ordered and deduplicated
-- exactly as before (gl_name and gl_username are each sorted on their own value).
-- name: ListBoardTasksWithMeta :many
SELECT
    t.*,
    tg.tag_ids,
    asg.assignee_ids,
    ga.gitlab_assignees,
    ga.gitlab_assignee_logins,
    gl.gl_iid AS gitlab_iid,
    gl.gl_web_url AS gitlab_url,
    gl.gl_author AS gitlab_author,
    gl.gl_author_name AS gitlab_author_name,
    gl.gl_author_avatar_url AS gitlab_author_avatar_url
FROM tasks t
LEFT JOIN LATERAL (
    SELECT COALESCE(array_agg(DISTINCT tt.tag_id), '{}')::uuid[] AS tag_ids
    FROM task_tags tt WHERE tt.task_id = t.id
) tg ON true
LEFT JOIN LATERAL (
    SELECT COALESCE(array_agg(DISTINCT ta.user_id), '{}')::uuid[] AS assignee_ids
    FROM task_assignees ta WHERE ta.task_id = t.id
) asg ON true
LEFT JOIN LATERAL (
    SELECT
        COALESCE(array_agg(DISTINCT tga.gl_name), '{}')::text[] AS gitlab_assignees,
        COALESCE(array_agg(DISTINCT tga.gl_username), '{}')::text[] AS gitlab_assignee_logins
    FROM task_gitlab_assignees tga WHERE tga.task_id = t.id
) ga ON true
LEFT JOIN gitlab_links gl ON gl.task_id = t.id
WHERE t.board_id = @board_id AND t.parent_id IS NULL
  -- Active board (archived_at IS NULL) or the read-only archive view (IS NOT NULL).
  AND ((@archived::boolean AND t.archived_at IS NOT NULL) OR (NOT @archived::boolean AND t.archived_at IS NULL))
  AND (
    (NOT @backlog::boolean AND sqlc.narg('milestone_id')::uuid IS NULL)             -- all (no scope)
    OR (@backlog::boolean AND t.milestone_id IS NULL)                               -- backlog (no milestone)
    OR (sqlc.narg('milestone_id')::uuid IS NOT NULL AND t.milestone_id = sqlc.narg('milestone_id')) -- one milestone
  )
-- Newest-archived first in the archive; board position otherwise.
ORDER BY CASE WHEN @archived::boolean THEN t.archived_at END DESC NULLS LAST, t.position;

-- name: ListSubtasks :many
SELECT * FROM tasks WHERE parent_id = $1 ORDER BY position;

-- ListSubtasksWithMeta returns a parent's subtasks with tag/assignee ids, so the
-- task modal can render a kanban-style hover card for each one.
--
-- The three gl_* columns are the subtask's GitLab hierarchy state (#2592), which the
-- subtasks tab renders per row: gl_iid non-null means the subtask has its own issue,
-- and an EMPTY gl_parent_global_id on top of that is precisely "created and linked,
-- but GitLab did not accept it into the hierarchy" — the state that offers a retry.
-- Joined here rather than fetched per row because the tab renders the whole list at
-- once; gitlab_links is keyed by task_id, so this cannot multiply rows.
-- name: ListSubtasksWithMeta :many
SELECT
    t.*,
    COALESCE(array_agg(DISTINCT tt.tag_id) FILTER (WHERE tt.tag_id IS NOT NULL), '{}')::uuid[] AS tag_ids,
    COALESCE(array_agg(DISTINCT ta.user_id) FILTER (WHERE ta.user_id IS NOT NULL), '{}')::uuid[] AS assignee_ids,
    gl.gl_iid,
    gl.gl_web_url,
    gl.gl_parent_global_id
FROM tasks t
LEFT JOIN task_tags tt ON tt.task_id = t.id
LEFT JOIN task_assignees ta ON ta.task_id = t.id
LEFT JOIN gitlab_links gl ON gl.task_id = t.id
WHERE t.parent_id = $1
GROUP BY t.id, gl.task_id
ORDER BY t.position;

-- ListBoardSubtasksWithMeta returns every subtask on a board (parent_id set)
-- with tag/assignee ids, so the kanban can render them under their parents.
-- gitlab_assignee_logins mirrors the top-level query so the composer's
-- "gl:<login>" assignee filter matches subtasks too.
--
-- LATERAL for the same reason as ListBoardTasksWithMeta: this fires on the same
-- board render, so joining the three M:N sets multiplied rows here too.
-- name: ListBoardSubtasksWithMeta :many
SELECT
    t.*,
    tg.tag_ids,
    asg.assignee_ids,
    ga.gitlab_assignee_logins
FROM tasks t
LEFT JOIN LATERAL (
    SELECT COALESCE(array_agg(DISTINCT tt.tag_id), '{}')::uuid[] AS tag_ids
    FROM task_tags tt WHERE tt.task_id = t.id
) tg ON true
LEFT JOIN LATERAL (
    SELECT COALESCE(array_agg(DISTINCT ta.user_id), '{}')::uuid[] AS assignee_ids
    FROM task_assignees ta WHERE ta.task_id = t.id
) asg ON true
LEFT JOIN LATERAL (
    SELECT COALESCE(array_agg(DISTINCT tga.gl_username), '{}')::text[] AS gitlab_assignee_logins
    FROM task_gitlab_assignees tga WHERE tga.task_id = t.id
) ga ON true
WHERE t.board_id = $1 AND t.parent_id IS NOT NULL AND t.archived_at IS NULL
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

-- ArchiveTaskIfActive/RestoreTaskIfArchived are idempotent single-task variants for
-- the GitLab sync (closed_policy=archive_closed_sprints): no-op + no updated_at churn
-- when the task is already in the target state.
-- name: ArchiveTaskIfActive :exec
UPDATE tasks SET archived_at = now(), updated_at = now() WHERE id = $1 AND archived_at IS NULL;

-- name: RestoreTaskIfArchived :exec
UPDATE tasks SET archived_at = NULL, updated_at = now() WHERE id = $1 AND archived_at IS NOT NULL;

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

-- UpdateTask writes a task's editable fields, optionally under an optimistic
-- lock: pass expected_updated_at with the updated_at the client read, and the
-- write only lands if nobody touched the row since — no row comes back
-- otherwise, so check and write stay in one statement with no gap to race in.
-- NULL (the default) means no check, which is how every internal caller writes.
-- The comparison is truncated to milliseconds because clients round-trip the
-- timestamp through JS Date, which drops the microseconds Postgres stores.
-- name: UpdateTask :one
UPDATE tasks
SET title = $2, description = $3, priority = $4, due_date = $5, completed_at = $6,
    recurrence = $7, start_date = $8, estimate = $9, updated_at = now()
WHERE id = $1
  AND (sqlc.narg('expected_updated_at')::timestamptz IS NULL
       OR date_trunc('milliseconds', updated_at)
          = date_trunc('milliseconds', sqlc.narg('expected_updated_at')::timestamptz))
RETURNING *;

-- ReopenSubtasks clears the completion of a recurring task's direct subtasks when
-- the parent recurs, so a checklist starts fresh for the next occurrence.
-- name: ReopenSubtasks :exec
UPDATE tasks SET completed_at = NULL, updated_at = now()
WHERE parent_id = $1 AND completed_at IS NOT NULL;

-- ListScheduleRecurDue returns active tasks whose recurrence fires on a schedule
-- and whose due date has passed — the schedule worker advances each one.
-- name: ListScheduleRecurDue :many
SELECT * FROM tasks
WHERE archived_at IS NULL
  AND completed_at IS NULL
  AND due_date IS NOT NULL
  AND due_date <= now()
  AND recurrence->>'trigger' = 'schedule'
ORDER BY due_date;

-- name: UpdateTaskDueDate :exec
UPDATE tasks SET due_date = $2, updated_at = now() WHERE id = $1;

-- name: UpdateTaskStartDate :exec
UPDATE tasks SET start_date = $2, updated_at = now() WHERE id = $1;

-- name: UpdateTaskEstimate :exec
UPDATE tasks SET estimate = $2, updated_at = now() WHERE id = $1;

-- SetTaskTitle / SetTaskDescription apply a single resolved field from a write-back
-- conflict resolution (theirs/manual) without disturbing the other columns.
-- name: SetTaskTitle :exec
UPDATE tasks SET title = $2, updated_at = now() WHERE id = $1;

-- name: SetTaskDescription :exec
UPDATE tasks SET description = $2, updated_at = now() WHERE id = $1;

-- name: SetTaskPriority :exec
UPDATE tasks SET priority = $2, updated_at = now() WHERE id = $1;

-- SetTaskColumnCompleted applies a resolved state conflict: move the task to a
-- column and set/clear its completion (used when accepting GitLab's open/closed).
-- name: SetTaskColumnCompleted :exec
UPDATE tasks SET column_id = $2, completed_at = $3, updated_at = now() WHERE id = $1;

-- name: MoveTask :one
UPDATE tasks
SET column_id = $2, position = $3, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteTask :exec
DELETE FROM tasks WHERE id = $1;

-- SetTaskDueNotify sets a task's per-task due-notification overrides (NULL = inherit
-- the user default). Used by the card's due popover.
-- name: SetTaskDueNotify :one
UPDATE tasks
SET due_lead_minutes = $2, due_repeat_minutes = $3, due_notify_enabled = $4, updated_at = now()
WHERE id = $1
RETURNING *;

-- SetTaskEisenhower pins a task to an Eisenhower-matrix quadrant (NULL = derive the
-- quadrant automatically from priority + due-date). Used by the matrix view's drag.
-- name: SetTaskEisenhower :one
UPDATE tasks
SET eisenhower_quadrant = $2, updated_at = now()
WHERE id = $1
RETURNING *;
