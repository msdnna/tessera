-- Вернуть NOT NULL можно только выбросив внепространственные уведомления:
-- пространство им неоткуда взять (у адресата его и нет). Это лента прочитанного
-- и непрочитанного, не источник истины — потеря ограничена самими строками.
DELETE FROM notifications WHERE workspace_id IS NULL;
ALTER TABLE notifications ALTER COLUMN workspace_id SET NOT NULL;
