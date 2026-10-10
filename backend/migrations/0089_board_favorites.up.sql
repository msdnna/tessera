-- Избранные доски (#3115): личный список досок пользователя — звезда у крошек
-- доски и раздел «Избранное» вверху дерева сайдбара.
--
-- Строка — «эта доска в избранном у этого человека». Порядок — float8 midpoint
-- (position), как у колонок и проектов: перетаскивание в сайдбаре ставит строку
-- между соседями, не перенумеровывая список. Список читается по пространству
-- (через доску → проект), отдельного workspace_id нет: доска может уехать в
-- другое пространство вместе с проектом, и избранное поедет с ней.
--
-- Удалённая (0078) доска и доска пространства, где человек больше не участник,
-- в выдаче не появляются — фильтр в запросе, а не удаление строки: восстановленная
-- доска и вернувшийся участник снова видят своё избранное. Окончательное удаление
-- доски (воркер trash_purge) и пользователя уносит строки каскадом.
CREATE TABLE IF NOT EXISTS board_favorites (
    user_id    uuid             NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    board_id   uuid             NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
    position   double precision NOT NULL DEFAULT 0,
    created_at timestamptz      NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, board_id)
);

CREATE INDEX IF NOT EXISTS idx_board_favorites_board ON board_favorites (board_id);
