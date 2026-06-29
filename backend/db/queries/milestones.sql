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

-- name: GetMilestone :one
SELECT * FROM milestones WHERE id = $1;

-- CreateMilestone appends a milestone (position = max+1 within the project).
-- name: CreateMilestone :one
INSERT INTO milestones (project_id, title, description, start_date, due_date, state, position)
VALUES (
    $1, $2, $3, $4, $5, COALESCE(sqlc.narg('state'), 'active'),
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
