-- Task #2588: done_column_id = NULL now means "this board has NO completing
-- column", not "fall back to the rightmost one". Without that fallback, legacy
-- boards left at NULL (created before 0007, or whose "Готово" column was renamed)
-- would silently stop closing tasks — so pin them to their rightmost column,
-- which is exactly what the runtime fallback resolved to until now.
UPDATE boards b SET done_column_id = (
    SELECT c.id FROM board_columns c
    WHERE c.board_id = b.id
    ORDER BY c.position DESC
    LIMIT 1
)
WHERE b.done_column_id IS NULL;
