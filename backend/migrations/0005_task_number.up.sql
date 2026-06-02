-- Per-workspace sequential task numbers (a reference tracker-style #N).
ALTER TABLE workspaces ADD COLUMN task_counter bigint NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN number bigint;

-- Backfill existing tasks: number per workspace by creation order.
WITH ordered AS (
    SELECT t.id,
           p.workspace_id AS ws,
           row_number() OVER (PARTITION BY p.workspace_id ORDER BY t.created_at, t.id) AS rn
    FROM tasks t
    JOIN boards b ON b.id = t.board_id
    JOIN projects p ON p.id = b.project_id
)
UPDATE tasks SET number = ordered.rn FROM ordered WHERE tasks.id = ordered.id;

-- Seed each workspace counter to its current max task number.
WITH maxn AS (
    SELECT p.workspace_id AS ws, COALESCE(max(t.number), 0) AS m
    FROM tasks t
    JOIN boards b ON b.id = t.board_id
    JOIN projects p ON p.id = b.project_id
    GROUP BY p.workspace_id
)
UPDATE workspaces SET task_counter = maxn.m FROM maxn WHERE workspaces.id = maxn.ws;

CREATE INDEX idx_tasks_number ON tasks (board_id, number);
