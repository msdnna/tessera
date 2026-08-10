DROP INDEX IF EXISTS idx_notes_body_trgm;
DROP INDEX IF EXISTS idx_notes_title_trgm;
DROP INDEX IF EXISTS idx_tasks_description_trgm;
DROP INDEX IF EXISTS idx_tasks_title_trgm;

-- The extension is deliberately left in place. It is a database-wide object that
-- anything else may have started depending on since; dropping it here would be a
-- wider rollback than this migration's own scope.
