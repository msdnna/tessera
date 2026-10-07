-- Откат 0079: общих представлений до неё не было — они удаляются (владельца у
-- них нет, вернуть ключ (board_id, user_id, name) с NULL нельзя).
ALTER TABLE boards DROP CONSTRAINT IF EXISTS boards_default_layout_check;
ALTER TABLE boards DROP COLUMN IF EXISTS default_layout;

DELETE FROM board_views WHERE scope = 'board' OR user_id IS NULL;
DROP INDEX IF EXISTS idx_board_views_board_shared;
DROP INDEX IF EXISTS ux_board_views_board_default;
DROP INDEX IF EXISTS ux_board_views_user_default;
DROP INDEX IF EXISTS ux_board_views_board_name;
DROP INDEX IF EXISTS ux_board_views_user_name;
ALTER TABLE board_views DROP CONSTRAINT IF EXISTS board_views_board_id_user_id_name_key;
ALTER TABLE board_views ADD CONSTRAINT board_views_board_id_user_id_name_key
    UNIQUE (board_id, user_id, name);
ALTER TABLE board_views DROP CONSTRAINT IF EXISTS board_views_owner_check;
ALTER TABLE board_views DROP CONSTRAINT IF EXISTS board_views_scope_check;
ALTER TABLE board_views ALTER COLUMN user_id SET NOT NULL;
ALTER TABLE board_views DROP COLUMN IF EXISTS created_by;
ALTER TABLE board_views DROP COLUMN IF EXISTS position;
ALTER TABLE board_views DROP COLUMN IF EXISTS is_default;
ALTER TABLE board_views DROP COLUMN IF EXISTS scope;
