ALTER TABLE gitlab_links DROP COLUMN IF EXISTS milestone_overridden;
DROP TABLE IF EXISTS gitlab_milestone_links;
