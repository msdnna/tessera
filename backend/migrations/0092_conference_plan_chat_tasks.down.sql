ALTER TABLE conference_messages DROP COLUMN IF EXISTS task_id;
ALTER TABLE conference_messages DROP COLUMN IF EXISTS kind;
DROP INDEX IF EXISTS idx_conferences_milestone;
ALTER TABLE conferences DROP COLUMN IF EXISTS milestone_id;
ALTER TABLE conferences DROP COLUMN IF EXISTS timezone;
ALTER TABLE conferences DROP COLUMN IF EXISTS recurrence;
ALTER TABLE conferences DROP COLUMN IF EXISTS duration_minutes;
