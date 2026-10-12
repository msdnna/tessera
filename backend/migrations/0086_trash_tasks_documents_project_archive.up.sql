-- Корзина задач и документов, архив проектов (#3001: #3004, #3005, #3006, #3120).
--
-- Модель взаимодействия v2 удаляет сразу и предлагает отмену в тосте, а удалённое
-- можно вернуть из корзины ещё 30 дней. Доски и колонки так уже работают (0078,
-- срок — сутки); здесь то же для задач и документов. Удалённая строка получает
-- deleted_at и для всех читателей перестаёт существовать; воркер trash_purge
-- удаляет окончательно то, что пролежало в корзине дольше срока.
--
-- Поддерево (подзадачи, вложенные документы) уходит в корзину одним UPDATE с
-- одной отметкой времени: по ней восстановление отличает «ушло вместе с корнем»
-- от «лежало в корзине раньше» — тот же приём, что у архива задач (0004).
ALTER TABLE tasks ADD COLUMN deleted_at timestamptz;
ALTER TABLE tasks ADD COLUMN deleted_by uuid REFERENCES users(id) ON DELETE SET NULL;
CREATE INDEX idx_tasks_deleted ON tasks (board_id, deleted_at) WHERE deleted_at IS NOT NULL;

ALTER TABLE documents ADD COLUMN deleted_at timestamptz;
ALTER TABLE documents ADD COLUMN deleted_by uuid REFERENCES users(id) ON DELETE SET NULL;
CREATE INDEX idx_documents_deleted ON documents (workspace_id, deleted_at) WHERE deleted_at IS NOT NULL;

-- Архив проекта — не корзина: обратим и без срока. Архивный проект пропадает из
-- дерева, поиска, «Моих задач» и сканеров уведомлений, но открывается на
-- просмотр из раздела «Архив проектов»; его задачи — только для чтения.
ALTER TABLE projects ADD COLUMN archived_at timestamptz;
