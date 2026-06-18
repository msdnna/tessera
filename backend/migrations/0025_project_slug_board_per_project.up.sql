-- Nest board URLs under the project (/project/<slug>/board/<slug>): projects get
-- a globally-unique slug, and board slugs become unique *per project* instead of
-- globally — so two projects can each have a board "Общие задачи" →
-- obshchie-zadachi with no confusing global -2/-3 suffix.
ALTER TABLE projects ADD COLUMN slug text NOT NULL DEFAULT '';
CREATE UNIQUE INDEX projects_slug_key ON projects (slug) WHERE slug <> '';

-- Re-scope board slug uniqueness from global to per-project, and clear existing
-- board slugs so they're regenerated per-project at startup (dropping the global
-- suffixes assigned under the old scheme).
DROP INDEX IF EXISTS boards_slug_key;
UPDATE boards SET slug = '';
CREATE UNIQUE INDEX boards_project_slug_key ON boards (project_id, slug) WHERE slug <> '';
