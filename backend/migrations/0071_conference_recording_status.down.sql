DROP INDEX IF EXISTS idx_conference_recordings_egress;
DROP INDEX IF EXISTS idx_conference_recordings_one_active;

ALTER TABLE conference_recordings
    DROP COLUMN IF EXISTS ended_at,
    DROP COLUMN IF EXISTS started_by,
    DROP COLUMN IF EXISTS error,
    DROP COLUMN IF EXISTS status,
    DROP COLUMN IF EXISTS egress_id;
