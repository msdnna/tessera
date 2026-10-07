-- Мягкое удаление досок и колонок (#2969, #3002, #3003).
--
-- Модель взаимодействия v2 делает обратимые удаления сразу и предлагает отмену
-- в тосте. Раньше DELETE доски и колонки был окончательным (каскад забирал
-- задачи), и отменять было нечего. Теперь строка получает deleted_at и для всех
-- читателей перестаёт существовать; восстановление снимает отметку, а воркер
-- уборки удаляет окончательно то, что пролежало удалённым дольше срока.
ALTER TABLE boards ADD COLUMN deleted_at timestamptz;
ALTER TABLE board_columns ADD COLUMN deleted_at timestamptz;

-- Уборка ищет только удалённое — частичные индексы держат этот поиск дешёвым и
-- не утяжеляют живые строки.
CREATE INDEX idx_boards_deleted ON boards (deleted_at) WHERE deleted_at IS NOT NULL;
CREATE INDEX idx_board_columns_deleted ON board_columns (deleted_at) WHERE deleted_at IS NOT NULL;

-- Колонка, удалённая как «Готово»: при восстановлении флаг возвращается к ней,
-- а не остаётся на колонке, которую удаление назначило взамен.
ALTER TABLE board_columns ADD COLUMN deleted_was_done boolean NOT NULL DEFAULT false;

-- Удаление колонки больше не удаляет задачи: они переезжают в другую колонку
-- доски. Здесь запомнено, откуда и куда переехала каждая задача, чтобы отмена
-- вернула её на прежнее место. Строки живут, пока колонка лежит удалённой:
-- восстановление их снимает, окончательное удаление колонки — каскадом.
CREATE TABLE column_deletion_moves (
    column_id     uuid NOT NULL REFERENCES board_columns(id) ON DELETE CASCADE,
    task_id       uuid NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    moved_to      uuid NOT NULL,
    from_position double precision NOT NULL,
    PRIMARY KEY (column_id, task_id)
);
CREATE INDEX idx_column_deletion_moves_task ON column_deletion_moves (task_id);
