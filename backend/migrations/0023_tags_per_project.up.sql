-- Tags move from workspace scope to project scope: a workspace can hold
-- unrelated projects (e.g. work vs. home renovation) whose tag vocabularies
-- shouldn't leak into each other. workspace_id is kept (denormalised) so the
-- realtime fan-out and membership checks stay cheap; project_id is the real
-- scope and the uniqueness key.
ALTER TABLE tags ADD COLUMN project_id uuid REFERENCES projects(id) ON DELETE CASCADE;

-- Backfill: park every existing tag in a single project per workspace — the
-- "Неолант Тенакс" project where applicable, otherwise the workspace's oldest
-- project. The user recreates the tags they need in the other projects.
UPDATE tags t SET project_id = (
    SELECT p.id FROM projects p
    WHERE p.workspace_id = t.workspace_id
    ORDER BY (p.name = 'Неолант Тенакс') DESC, p.created_at ASC
    LIMIT 1
);

-- Drop tags whose workspace has no project at all (nothing to scope them to).
DELETE FROM tags WHERE project_id IS NULL;

-- Now that tags live in one project, strip the leaked associations: a task keeps
-- a tag only if the tag belongs to the task's own project.
DELETE FROM task_tags tt
USING tasks tk, boards b, tags tg
WHERE tt.task_id = tk.id
  AND tk.board_id = b.id
  AND tt.tag_id = tg.id
  AND b.project_id <> tg.project_id;

ALTER TABLE tags ALTER COLUMN project_id SET NOT NULL;
ALTER TABLE tags DROP CONSTRAINT IF EXISTS tags_workspace_id_name_key;
ALTER TABLE tags ADD CONSTRAINT tags_project_id_name_key UNIQUE (project_id, name);
CREATE INDEX IF NOT EXISTS idx_tags_project ON tags (project_id);
