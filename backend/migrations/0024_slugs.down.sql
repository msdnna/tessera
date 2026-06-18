DROP INDEX IF EXISTS boards_slug_key;
DROP INDEX IF EXISTS notes_ws_slug_key;
ALTER TABLE boards DROP COLUMN slug;
ALTER TABLE notes  DROP COLUMN slug;
