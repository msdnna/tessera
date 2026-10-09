-- Представления доски v2 (#2970: #2988, #3015, #3016, #3017).
--
-- До этого представление было только личным: (board_id, user_id, name) — ключ,
-- порядок — по имени, «по умолчанию» не было. Теперь:
--   * scope = 'user' — личное (user_id = владелец), scope = 'board' — общее для
--     всех участников доски (user_id IS NULL: общее не принадлежит никому и не
--     должно уходить каскадом вместе с автором, поэтому автор — в created_by);
--   * is_default — звезда: у личных — «открывать у меня», у общих — «открывать у
--     всех» (ставит владелец/админ пространства); личное перекрывает общее;
--   * position — порядок в списке (клавиши 1–9), float8 midpoint, как у колонок.
-- И вид новой доски (#3017): boards.default_layout, выбирается при создании.
--
-- Миграция аддитивная и повторяемая: каждый шаг защищён IF [NOT] EXISTS.

ALTER TABLE board_views ADD COLUMN IF NOT EXISTS scope text NOT NULL DEFAULT 'user';
ALTER TABLE board_views ADD COLUMN IF NOT EXISTS is_default boolean NOT NULL DEFAULT false;
ALTER TABLE board_views ADD COLUMN IF NOT EXISTS position double precision NOT NULL DEFAULT 0;
ALTER TABLE board_views ADD COLUMN IF NOT EXISTS created_by uuid REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE board_views ALTER COLUMN user_id DROP NOT NULL;

-- Ограничения пересоздаются (DROP IF EXISTS + ADD) — так шаг повторяем.
ALTER TABLE board_views DROP CONSTRAINT IF EXISTS board_views_scope_check;
ALTER TABLE board_views ADD CONSTRAINT board_views_scope_check
    CHECK (scope IN ('user', 'board'));
-- Личное всегда чьё-то, общее — ничьё.
ALTER TABLE board_views DROP CONSTRAINT IF EXISTS board_views_owner_check;
ALTER TABLE board_views ADD CONSTRAINT board_views_owner_check
    CHECK ((scope = 'user') = (user_id IS NOT NULL));

-- Существующие строки: автор = владелец, порядок = прежний (по имени).
UPDATE board_views SET created_by = user_id WHERE created_by IS NULL AND user_id IS NOT NULL;
UPDATE board_views v SET position = r.rn * 1024
FROM (
    SELECT id, row_number() OVER (PARTITION BY board_id, user_id ORDER BY name) AS rn
    FROM board_views
) r
WHERE v.id = r.id AND v.position = 0;

-- Уникальность имени — в пределах своего списка: личные одного человека на
-- доске, общие доски. Старое ограничение (board_id, user_id, name) при NULL в
-- user_id общие не различало бы вовсе, поэтому заменено частичными индексами.
ALTER TABLE board_views DROP CONSTRAINT IF EXISTS board_views_board_id_user_id_name_key;
CREATE UNIQUE INDEX IF NOT EXISTS ux_board_views_user_name
    ON board_views (board_id, user_id, name) WHERE scope = 'user';
CREATE UNIQUE INDEX IF NOT EXISTS ux_board_views_board_name
    ON board_views (board_id, name) WHERE scope = 'board';
-- Не больше одной звезды в каждом списке.
CREATE UNIQUE INDEX IF NOT EXISTS ux_board_views_user_default
    ON board_views (board_id, user_id) WHERE scope = 'user' AND is_default;
CREATE UNIQUE INDEX IF NOT EXISTS ux_board_views_board_default
    ON board_views (board_id) WHERE scope = 'board' AND is_default;
CREATE INDEX IF NOT EXISTS idx_board_views_board_shared
    ON board_views (board_id) WHERE scope = 'board';

-- Вид новой доски (#3017): канбан по умолчанию, как было.
ALTER TABLE boards ADD COLUMN IF NOT EXISTS default_layout text NOT NULL DEFAULT 'board';
ALTER TABLE boards DROP CONSTRAINT IF EXISTS boards_default_layout_check;
ALTER TABLE boards ADD CONSTRAINT boards_default_layout_check
    CHECK (default_layout IN ('board', 'list', 'timeline', 'gantt', 'calendar', 'matrix'));
