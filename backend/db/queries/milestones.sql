-- Milestones («Этап») — project-scoped planning unit. CRUD + task assignment.
-- GitLab-sourced milestones get a row in gitlab_milestone_links; since #3010 such a
-- milestone is editable in Tessera when its binding allows writing (gl_writable).

-- ListMilestones returns a project's milestones with their GitLab link info (null
-- columns when native), so the UI can tell native from GitLab-sourced — and, for
-- a GitLab one (#3010), whether it may be written back (gl_writable: binding on
-- and write-back on), whether the last write failed (gl_push_error) and whether
-- a conflict waits in the conflicts window (gl_conflict).
--
-- Each row also carries the project-level rollups the «Проект» window and the
-- sidebar show next to a stage (#2992): task_count / done_count and Σ estimate
-- (total and done), counted the way ListWorkspaceMilestones counts them —
-- archived tasks, tasks in the trash and tasks of deleted boards excluded.
-- name: ListMilestones :many
SELECT m.*, l.gl_web_url AS gl_url, l.gl_global_id AS gl_global_id, l.gl_iid AS gl_iid,
       l.push_error AS gl_push_error, l.push_fields AS gl_push_fields,
       COALESCE(l.conflict <> '{}'::jsonb, false)::bool AS gl_conflict,
       COALESCE(gi.enabled AND COALESCE((gi.writeback->>'enabled')::boolean, false), false)::bool AS gl_writable,
       COUNT(t.id) AS task_count,
       COUNT(t.id) FILTER (WHERE t.completed_at IS NOT NULL) AS done_count,
       COALESCE(SUM(t.estimate), 0)::float8 AS estimate_sum,
       COALESCE(SUM(t.estimate) FILTER (WHERE t.completed_at IS NOT NULL), 0)::float8 AS done_estimate
FROM milestones m
LEFT JOIN gitlab_milestone_links l ON l.milestone_id = m.id
LEFT JOIN gitlab_integrations gi ON gi.id = l.integration_id
LEFT JOIN tasks t ON t.milestone_id = m.id AND t.archived_at IS NULL AND t.deleted_at IS NULL
    AND EXISTS (SELECT 1 FROM boards tb WHERE tb.id = t.board_id AND tb.deleted_at IS NULL)
WHERE m.project_id = $1
GROUP BY m.id, l.milestone_id, gi.id
ORDER BY m.position, m.created_at;

-- ListWorkspaceMilestones returns every milestone across a workspace's projects with
-- per-milestone task rollups (total / done / Σ estimate, archived excluded) plus the
-- owning project's name + slug and a board slug to deep-link to. Powers the dedicated
-- «Этапы» roadmap screen (one query, no per-project fan-out).
-- name: ListWorkspaceMilestones :many
SELECT m.id, m.project_id, m.title, m.description, m.start_date, m.due_date,
       m.state, m.position, m.slug, m.created_at, m.updated_at, m.closed_at,
       l.gl_web_url AS gl_url, l.gl_global_id AS gl_global_id,
       p.name AS project_name, p.slug AS project_slug,
       COALESCE((SELECT b.slug FROM boards b WHERE b.project_id = p.id AND b.deleted_at IS NULL
        ORDER BY b.position, b.created_at LIMIT 1), '') AS board_slug,
       COUNT(t.id) AS task_count,
       COUNT(t.id) FILTER (WHERE t.completed_at IS NOT NULL) AS done_count,
       COALESCE(SUM(t.estimate), 0)::float8 AS estimate_sum
FROM milestones m
JOIN projects p ON p.id = m.project_id
LEFT JOIN gitlab_milestone_links l ON l.milestone_id = m.id
LEFT JOIN tasks t ON t.milestone_id = m.id AND t.archived_at IS NULL AND t.deleted_at IS NULL
    AND EXISTS (SELECT 1 FROM boards tb WHERE tb.id = t.board_id AND tb.deleted_at IS NULL)
WHERE p.workspace_id = $1
GROUP BY m.id, l.gl_web_url, l.gl_global_id, p.name, p.slug, p.id
ORDER BY p.name, m.position, m.created_at;

-- name: GetMilestone :one
SELECT * FROM milestones WHERE id = $1;

-- GetMilestoneInProjectBySlug resolves a ?milestone=<slug> board scope.
-- name: GetMilestoneInProjectBySlug :one
SELECT * FROM milestones WHERE project_id = $1 AND slug = $2;

-- GetMilestoneInProjectByTitle finds the stage a GitLab «лейбл → этап» rule
-- names (#3086), case-insensitively; the oldest wins when titles repeat.
-- name: GetMilestoneInProjectByTitle :one
SELECT * FROM milestones
WHERE project_id = $1 AND lower(title) = lower(sqlc.arg('title')::text)
ORDER BY created_at, id
LIMIT 1;

-- name: MilestoneSlugExistsInProject :one
SELECT EXISTS(SELECT 1 FROM milestones WHERE project_id = $1 AND slug = $2);

-- name: MilestonesMissingSlug :many
SELECT id, project_id, title FROM milestones WHERE slug = '';

-- name: SetMilestoneSlug :exec
UPDATE milestones SET slug = $2 WHERE id = $1;

-- CreateMilestone appends a milestone (position = max+1 within the project).
-- name: CreateMilestone :one
-- A stage born closed (a GitLab milestone first seen closed) is stamped closed now.
INSERT INTO milestones (project_id, title, description, start_date, due_date, state, slug, position, closed_at)
VALUES (
    $1, $2, $3, $4, $5, COALESCE(sqlc.narg('state')::text, 'active'), $6,
    (SELECT COALESCE(MAX(position), 0) + 1 FROM milestones WHERE project_id = $1),
    CASE WHEN sqlc.narg('state')::text = 'closed' THEN now() END
)
RETURNING *;

-- closed_at follows the state (#3027): closing stamps it once — a later edit of
-- a closed stage keeps the day it was closed — and reopening clears it.
-- name: UpdateMilestone :one
UPDATE milestones
SET title = $2, description = $3, start_date = $4, due_date = $5, state = $6, updated_at = now(),
    closed_at = CASE WHEN $6 = 'closed' THEN COALESCE(closed_at, now()) END
WHERE id = $1
RETURNING *;

-- name: DeleteMilestone :exec
DELETE FROM milestones WHERE id = $1;

-- name: SetTaskMilestone :exec
UPDATE tasks SET milestone_id = $2, updated_at = now() WHERE id = $1;

-- ClearMilestonesAcrossWorkspaces unpins every task that, after a project
-- transfer, points at a milestone owned by a different workspace — in either
-- direction: a task of the moved project left pointing at a milestone that
-- stayed, and a task that stayed pointing at one that moved. applyMilestone
-- refuses such a pairing at the door; ListWorkspaceMilestones rolls the
-- milestone up over `tasks t ON t.milestone_id = m.id` with no workspace filter,
-- so a stranded task keeps inflating the count and the Σ estimate of a milestone
-- its own workspace cannot even list. Returns the number of tasks unpinned.
-- name: ClearMilestonesAcrossWorkspaces :execrows
UPDATE tasks SET milestone_id = NULL, updated_at = now()
WHERE milestone_id IS NOT NULL
  AND (
        board_id IN (SELECT b.id FROM boards b WHERE b.project_id = $1)
     OR milestone_id IN (SELECT m.id FROM milestones m WHERE m.project_id = $1)
      )
  AND (SELECT p.workspace_id FROM boards b JOIN projects p ON p.id = b.project_id
       WHERE b.id = tasks.board_id)
      IS DISTINCT FROM
      (SELECT p.workspace_id FROM milestones m JOIN projects p ON p.id = m.project_id
       WHERE m.id = tasks.milestone_id);

-- SubtreeMilestonesOffProject names every task of a subtree (the root included)
-- that is about to lose its milestone to a move onto a board of @project_id,
-- together with the title it loses. The unpinning itself is done by the move —
-- TransferTask, MoveSubtreeToBoard and SetTaskParent each carry the same
-- project_id CASE, because a milestone hangs off a project and a task moves
-- between boards by workspace. This query exists so the change can be JOURNALLED:
-- it must run inside the move's transaction and BEFORE it, since afterwards the
-- task no longer holds the id the title is reachable through.
--
-- The condition is deliberately the same shape as the CASE in those three, so a
-- future change to one is a visible difference here rather than a journal that
-- quietly stops matching what happened. `IS DISTINCT FROM` is what the CASE's
-- `IS NOT DISTINCT FROM` keeps, negated: the join to `milestones` already
-- restricts this to tasks that actually carry one.
--
-- The walk carries ids only and the condition is read back off `tasks`: sqlc
-- resolves a recursive CTE's own columns nowhere but its select list (the same
-- corner ReopenSubtreeLeavingDone and TaskAncestorIDs note).
-- name: SubtreeMilestonesOffProject :many
WITH RECURSIVE down AS (
    SELECT t.id FROM tasks t WHERE t.id = @root_id
    UNION
    SELECT t.id FROM tasks t JOIN down ON t.parent_id = down.id
)
SELECT t.id, m.title, m.id AS milestone_id
FROM tasks t
JOIN milestones m ON m.id = t.milestone_id
WHERE t.id IN (SELECT down.id FROM down)
  AND m.project_id IS DISTINCT FROM @project_id;

-- ── stage page metrics (#2992) ──────────────────────────────────

-- MilestoneTasks lists the tasks a stage holds right now — the population of
-- every metric on the stage page — with what the page shows per task: its
-- column (status), board, dates and estimate. Same exclusions as the rollups:
-- archived tasks and tasks of deleted boards are not in the stage.
-- name: MilestoneTasks :many
SELECT t.id, t.number, t.title, t.parent_id, t.board_id, t.column_id, t.priority,
       t.due_date, t.start_date, t.estimate, t.completed_at, t.created_at,
       c.name AS column_name, c.color AS column_color, c.position AS column_position,
       b.slug AS board_slug, b.name AS board_name
FROM tasks t
JOIN boards b ON b.id = t.board_id AND b.deleted_at IS NULL
JOIN board_columns c ON c.id = t.column_id
WHERE t.milestone_id = $1 AND t.archived_at IS NULL AND t.deleted_at IS NULL
ORDER BY c.position, t.position;

-- MilestoneTaskAssignees: who carries the stage's tasks (for «По исполнителям»).
-- name: MilestoneTaskAssignees :many
SELECT ta.task_id, u.id AS user_id, u.name, u.email
FROM task_assignees ta
JOIN tasks t ON t.id = ta.task_id
JOIN users u ON u.id = ta.user_id
WHERE t.milestone_id = $1 AND t.archived_at IS NULL AND t.deleted_at IS NULL
ORDER BY u.name;

-- MilestoneMembershipEvents returns every journal entry that moved a task into
-- or out of the stage: `milestone` names where the task landed (milestone_id)
-- and, since #2992, where it came from (from_id); `milestone_removed` names the
-- stage it left. Entries written before the ids were journalled carry only the
-- title — those are matched by title on the tasks of the stage's project, the
-- best reading that history allows.
-- name: MilestoneMembershipEvents :many
SELECT e.task_id, e.kind, e.data, e.created_at
FROM task_events e
WHERE e.kind IN ('milestone', 'milestone_removed')
  AND (
        e.data->>'milestone_id' = @milestone_id::text
     OR e.data->>'from_id' = @milestone_id::text
     OR (
            NOT (e.data ? 'milestone_id')
        AND (e.data->>'title' = @title::text OR e.data->>'from_title' = @title::text)
        AND EXISTS (SELECT 1 FROM tasks t JOIN boards b ON b.id = t.board_id
                    WHERE t.id = e.task_id AND b.project_id = @project_id)
        )
      )
ORDER BY e.created_at;

-- PreviousMilestones: the project's other stages closed before @before (the
-- stage's own close, or now while it runs) — the «Темп» comparison. Newest
-- first; done_count is what each finished.
-- name: PreviousMilestones :many
SELECT m.id, m.title, m.slug, m.start_date, m.due_date, m.closed_at,
       COUNT(t.id) FILTER (WHERE t.completed_at IS NOT NULL) AS done_count,
       COUNT(t.id) AS task_count
FROM milestones m
LEFT JOIN tasks t ON t.milestone_id = m.id AND t.archived_at IS NULL AND t.deleted_at IS NULL
    AND EXISTS (SELECT 1 FROM boards tb WHERE tb.id = t.board_id AND tb.deleted_at IS NULL)
WHERE m.project_id = @project_id AND m.id <> @milestone_id AND m.state = 'closed'
  AND COALESCE(m.closed_at, m.due_date, m.updated_at) <= @before::timestamptz
GROUP BY m.id
ORDER BY COALESCE(m.closed_at, m.due_date, m.updated_at) DESC
LIMIT @lim;

-- MilestoneEventTasks reads the tasks the stage's journal names: those still in
-- it and those that left. Archived tasks and tasks of deleted boards come back
-- flagged — the page leaves them out of every figure, as the rollups do.
-- name: MilestoneEventTasks :many
SELECT t.id, t.estimate, t.completed_at, t.created_at,
       (t.archived_at IS NOT NULL OR t.deleted_at IS NOT NULL OR b.deleted_at IS NOT NULL)::boolean AS gone
FROM tasks t
JOIN boards b ON b.id = t.board_id
WHERE t.id = ANY(@ids::uuid[]);
