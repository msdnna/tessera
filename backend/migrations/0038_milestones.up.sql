-- Milestones («Этап») — a project-scoped planning unit, 1:1 with GitLab project
-- milestones (the GitLab link lives in its own table, added later). A task has at
-- most one milestone. Native here; the GitLab pull/write-back map onto this in
-- later steps. Additive.

CREATE TABLE milestones (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id  uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    title       text NOT NULL,
    description text NOT NULL DEFAULT '',
    start_date  timestamptz,
    due_date    timestamptz,
    state       text NOT NULL DEFAULT 'active',           -- active | closed
    position    double precision NOT NULL DEFAULT 0,      -- append/explicit ordering
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_milestones_project ON milestones (project_id);

-- A task's milestone (nullable; cleared, not cascaded, when the milestone is deleted).
ALTER TABLE tasks ADD COLUMN milestone_id uuid REFERENCES milestones(id) ON DELETE SET NULL;
CREATE INDEX idx_tasks_milestone ON tasks (milestone_id) WHERE milestone_id IS NOT NULL;
