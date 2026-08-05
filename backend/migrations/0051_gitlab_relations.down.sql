DROP TABLE IF EXISTS gitlab_issue_links;
ALTER TABLE gitlab_integrations DROP COLUMN IF EXISTS relations_sync;
ALTER TABLE task_relations DROP COLUMN IF EXISTS source;
