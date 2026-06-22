DROP INDEX IF EXISTS idx_gitlab_writebacks_pending_task;
DROP INDEX IF EXISTS idx_gitlab_writebacks_due;
DROP TABLE IF EXISTS gitlab_writebacks;
ALTER TABLE gitlab_links DROP COLUMN IF EXISTS gl_last_state;
ALTER TABLE gitlab_integrations DROP COLUMN IF EXISTS writeback;
