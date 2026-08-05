-- Milestones («Этап») — project-scoped planning unit. CRUD + task assignment.
-- GitLab-sourced milestones get a row in gitlab_milestone_links (added later); the
-- presence of that link makes a milestone read-only in Tessera.

-- ListMilestones returns a project's milestones with their GitLab link info (null
-- columns when native), so the UI can tell native from GitLab-sourced.
-- name: ListMilestones :many
SELECT m.*, l.gl_web_url AS gl_url, l.gl_global_id AS gl_global_id
FROM milestones m
LEFT JOIN gitlab_milestone_links l ON l.milestone_id = m.id
WHERE m.project_id = $1
ORDER BY m.position, m.created_at;

-- ListWorkspaceMilestones returns every milestone across a workspace's projects with
-- per-milestone task rollups (total / done / Σ estimate, archived excluded) plus the
-- owning project's name + slug and a board slug to deep-link to. Powers the dedicated
-- «Этапы» roadmap screen (one query, no per-project fan-out).
-- name: ListWorkspaceMilestones :many
SELECT m.id, m.project_id, m.title, m.description, m.start_date, m.due_date,
       m.state, m.position, m.slug, m.created_at, m.updated_at,
       l.gl_web_url AS gl_url, l.gl_global_id AS gl_global_id,
       p.name AS project_name, p.slug AS project_slug,
       COALESCE((SELECT b.slug FROM boards b WHERE b.project_id = p.id
        ORDER BY b.position, b.created_at LIMIT 1), '') AS board_slug,
       COUNT(t.id) AS task_count,
       COUNT(t.id) FILTER (WHERE t.completed_at IS NOT NULL) AS done_count,
       COALESCE(SUM(t.estimate), 0)::float8 AS estimate_sum
FROM milestones m
JOIN projects p ON p.id = m.project_id
LEFT JOIN gitlab_milestone_links l ON l.milestone_id = m.id
LEFT JOIN tasks t ON t.milestone_id = m.id AND t.archived_at IS NULL
WHERE p.workspace_id = $1
GROUP BY m.id, l.gl_web_url, l.gl_global_id, p.name, p.slug, p.id
ORDER BY p.name, m.position, m.created_at;

-- name: GetMilestone :one
SELECT * FROM milestones WHERE id = $1;

-- GetMilestoneInProjectBySlug resolves a ?milestone=<slug> board scope.
-- name: GetMilestoneInProjectBySlug :one
SELECT * FROM milestones WHERE project_id = $1 AND slug = $2;

-- name: MilestoneSlugExistsInProject :one
SELECT EXISTS(SELECT 1 FROM milestones WHERE project_id = $1 AND slug = $2);

-- name: MilestonesMissingSlug :many
SELECT id, project_id, title FROM milestones WHERE slug = '';

-- name: SetMilestoneSlug :exec
UPDATE milestones SET slug = $2 WHERE id = $1;

-- CreateMilestone appends a milestone (position = max+1 within the project).
-- name: CreateMilestone :one
INSERT INTO milestones (project_id, title, description, start_date, due_date, state, slug, position)
VALUES (
    $1, $2, $3, $4, $5, COALESCE(sqlc.narg('state'), 'active'), $6,
    (SELECT COALESCE(MAX(position), 0) + 1 FROM milestones WHERE project_id = $1)
)
RETURNING *;

-- name: UpdateMilestone :one
UPDATE milestones
SET title = $2, description = $3, start_date = $4, due_date = $5, state = $6, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteMilestone :exec
DELETE FROM milestones WHERE id = $1;

-- name: SetTaskMilestone :exec
UPDATE tasks SET milestone_id = $2, updated_at = now() WHERE id = $1;
