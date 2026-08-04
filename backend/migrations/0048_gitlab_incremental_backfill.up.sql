-- Defensive re-assert of the 0046 columns with IF NOT EXISTS.
--
-- Some environments were advanced to schema version 46 by an out-of-repo migration
-- BEFORE 0046 shipped (a "ghost" 46). golang-migrate then treated 0046 as already
-- applied and never ran its ALTERs there, so the columns were missing and every
-- gitlab_integrations read returned 500. This migration re-adds them idempotently:
-- a no-op where 0046 applied cleanly, self-healing where it was skipped.
ALTER TABLE gitlab_integrations ADD COLUMN IF NOT EXISTS last_full_synced_at timestamptz;
ALTER TABLE gitlab_integrations ADD COLUMN IF NOT EXISTS members_synced_at timestamptz;
ALTER TABLE gitlab_integrations ADD COLUMN IF NOT EXISTS full_sync_interval_sec integer NOT NULL DEFAULT 86400;
ALTER TABLE gitlab_sync_runs ADD COLUMN IF NOT EXISTS mode text NOT NULL DEFAULT 'full';
