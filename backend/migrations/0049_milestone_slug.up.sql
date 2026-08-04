-- Give milestones a URL slug so sprint links read /project/<slug>/board/<slug>?milestone=sprint-1
-- instead of exposing a UUID. Scoped per project (like board slugs), so two
-- projects can each have a "Спринт 1". Assigned on create and never changed on
-- rename, keeping shared links stable; existing rows are backfilled at startup.
ALTER TABLE milestones ADD COLUMN slug text NOT NULL DEFAULT '';
CREATE UNIQUE INDEX milestones_project_slug_key ON milestones (project_id, slug) WHERE slug <> '';
