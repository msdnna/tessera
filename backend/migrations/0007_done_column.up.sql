-- Phase: configurable "done" column (feature #4).
-- Replaces the hardcoded column-name match ("Готово") with an explicit
-- per-board pointer. NULL means "fall back to the rightmost column".
ALTER TABLE boards
    ADD COLUMN done_column_id uuid REFERENCES board_columns(id) ON DELETE SET NULL;

-- Backfill existing boards: point at their "Готово" column where one exists.
-- Boards without it stay NULL and resolve to the rightmost column at runtime.
UPDATE boards b SET done_column_id = (
    SELECT c.id FROM board_columns c
    WHERE c.board_id = b.id AND c.name = 'Готово'
    ORDER BY c.position DESC
    LIMIT 1
);
