-- Incremental GitLab sync: pull only the issues that changed since the last run
-- (server-side updatedAfter), keeping a full sweep as an explicit option. Track the
-- last full sweep and member-roster refresh so the auto worker can throttle them,
-- and record whether a run was a full or incremental pull for the journal.

ALTER TABLE gitlab_integrations ADD COLUMN last_full_synced_at timestamptz;
ALTER TABLE gitlab_integrations ADD COLUMN members_synced_at timestamptz;
-- How often the auto worker forces a full sweep (catches deletes/drift an
-- incremental pull can't see). 0 disables the forced full sweep. Default 24h.
ALTER TABLE gitlab_integrations ADD COLUMN full_sync_interval_sec integer NOT NULL DEFAULT 86400;

-- 'full' | 'incremental' — how the run fetched issues.
ALTER TABLE gitlab_sync_runs ADD COLUMN mode text NOT NULL DEFAULT 'full';
