-- GitLab start-date sync: take a task's start from the issue/task creation date
-- or its milestone's Start date, configurable per integration, with manual edits
-- winning. Mirrors the due-date sync (mig 0012). Additive.

-- A manually-set Tessera start date wins over GitLab: once the user edits a linked
-- task's start, the sync stops touching it.
ALTER TABLE gitlab_links ADD COLUMN start_overridden boolean NOT NULL DEFAULT false;

-- Where the sync takes a task's start from:
--   created (default) = the issue/task creation date (always present)
--   milestone         = the milestone Start date
--   off               = never sync the start
ALTER TABLE gitlab_integrations ADD COLUMN start_source text NOT NULL DEFAULT 'created';
