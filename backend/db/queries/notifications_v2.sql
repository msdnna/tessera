-- The facts behind the v2 notifications (#3048): who hears that a stage was
-- closed and how far it got, which meetings are about to start.

-- MilestoneCloseSummary is what «Этап закрыт» says: the project it belongs to
-- and how many of its tasks were done — counted the way the stage page counts
-- them (archived tasks and tasks of deleted boards left out).
-- name: MilestoneCloseSummary :one
SELECT p.name AS project_name, p.slug AS project_slug,
       COUNT(t.id) AS task_count,
       COUNT(t.id) FILTER (WHERE t.completed_at IS NOT NULL) AS done_count
FROM milestones m
JOIN projects p ON p.id = m.project_id
LEFT JOIN tasks t ON t.milestone_id = m.id AND t.archived_at IS NULL AND t.deleted_at IS NULL
    AND EXISTS (SELECT 1 FROM boards tb WHERE tb.id = t.board_id AND tb.deleted_at IS NULL)
WHERE m.id = $1
GROUP BY p.name, p.slug;

-- MilestoneAssignees are the people the stage's tasks are assigned to — the ones
-- a closed stage is news for.
-- name: MilestoneAssignees :many
SELECT DISTINCT ta.user_id
FROM tasks t
JOIN task_assignees ta ON ta.task_id = t.id
JOIN boards b ON b.id = t.board_id AND b.deleted_at IS NULL
WHERE t.milestone_id = $1 AND t.archived_at IS NULL AND t.deleted_at IS NULL;

-- ListConferencesStartingSoon are the planned meetings whose start falls within
-- the lead and whose «встреча через 5 минут» has not gone out for this start
-- yet. Comparing against the start it went out for, rather than a flag, is what
-- re-arms it when the meeting is moved.
-- name: ListConferencesStartingSoon :many
SELECT * FROM conferences
WHERE status = 'scheduled'
  AND scheduled_at IS NOT NULL
  AND scheduled_at > now()
  AND scheduled_at <= now() + make_interval(mins => @lead_minutes::int)
  AND soon_notified_for IS DISTINCT FROM scheduled_at;

-- MarkConferenceSoonNotified stamps the start the reminder went out for. Guarded
-- on that start, so a meeting moved between the read and the stamp stays armed.
-- name: MarkConferenceSoonNotified :execrows
UPDATE conferences SET soon_notified_for = scheduled_at
WHERE id = $1 AND scheduled_at = $2 AND soon_notified_for IS DISTINCT FROM scheduled_at;
