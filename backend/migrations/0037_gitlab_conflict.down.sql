DROP INDEX IF EXISTS idx_gitlab_writebacks_conflict;
ALTER TABLE gitlab_writebacks DROP COLUMN IF EXISTS resolved_at;
ALTER TABLE gitlab_writebacks DROP COLUMN IF EXISTS resolved_by;
ALTER TABLE gitlab_writebacks DROP COLUMN IF EXISTS resolution;
ALTER TABLE gitlab_writebacks DROP COLUMN IF EXISTS conflict;
ALTER TABLE gitlab_links DROP COLUMN IF EXISTS gl_snapshot;
