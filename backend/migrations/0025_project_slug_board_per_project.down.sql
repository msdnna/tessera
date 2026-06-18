DROP INDEX IF EXISTS boards_project_slug_key;
UPDATE boards SET slug = '';
CREATE UNIQUE INDEX boards_slug_key ON boards (slug) WHERE slug <> '';
DROP INDEX IF EXISTS projects_slug_key;
ALTER TABLE projects DROP COLUMN slug;
