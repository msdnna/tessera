ALTER TABLE gitlab_integrations DROP COLUMN IF EXISTS last_synced_at;
ALTER TABLE gitlab_integrations DROP COLUMN IF EXISTS sync_interval_sec;
ALTER TABLE gitlab_integrations DROP COLUMN IF EXISTS owner_user_id;
ALTER TABLE gitlab_links DROP COLUMN IF EXISTS gl_author_name;
ALTER TABLE gitlab_links DROP COLUMN IF EXISTS gl_author;
