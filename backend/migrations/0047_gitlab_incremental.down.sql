ALTER TABLE gitlab_sync_runs DROP COLUMN IF EXISTS mode;
ALTER TABLE gitlab_integrations DROP COLUMN IF EXISTS full_sync_interval_sec;
ALTER TABLE gitlab_integrations DROP COLUMN IF EXISTS members_synced_at;
ALTER TABLE gitlab_integrations DROP COLUMN IF EXISTS last_full_synced_at;
