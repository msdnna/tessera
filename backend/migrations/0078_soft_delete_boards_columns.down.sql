-- Без deleted_at удалённые строки снова стали бы живыми — откат доводит
-- удаление до конца (задачи удалённой доски уходят каскадом, как до 0078).
DELETE FROM board_columns WHERE deleted_at IS NOT NULL;
DELETE FROM boards WHERE deleted_at IS NOT NULL;
DROP TABLE IF EXISTS column_deletion_moves;
ALTER TABLE board_columns DROP COLUMN IF EXISTS deleted_was_done;
DROP INDEX IF EXISTS idx_board_columns_deleted;
DROP INDEX IF EXISTS idx_boards_deleted;
ALTER TABLE board_columns DROP COLUMN IF EXISTS deleted_at;
ALTER TABLE boards DROP COLUMN IF EXISTS deleted_at;
