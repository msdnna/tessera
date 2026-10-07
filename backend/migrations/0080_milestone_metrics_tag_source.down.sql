-- Откат 0080: дата закрытия этапа, источник тега и индекс событий этапа.
DROP INDEX IF EXISTS idx_task_events_milestone;
ALTER TABLE tags DROP CONSTRAINT IF EXISTS tags_source_check;
ALTER TABLE tags DROP COLUMN IF EXISTS source;
ALTER TABLE milestones DROP COLUMN IF EXISTS closed_at;
