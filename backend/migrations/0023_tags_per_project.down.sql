-- Revert tag scope to per-workspace. Data lost in the up migration (pruned
-- leaked associations, dropped tags) is not restored — only the schema.
ALTER TABLE tags DROP CONSTRAINT IF EXISTS tags_project_id_name_key;
DROP INDEX IF EXISTS idx_tags_project;
ALTER TABLE tags DROP COLUMN project_id;
ALTER TABLE tags ADD CONSTRAINT tags_workspace_id_name_key UNIQUE (workspace_id, name);
