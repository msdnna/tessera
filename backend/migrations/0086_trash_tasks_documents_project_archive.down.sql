-- Без deleted_at удалённые строки снова стали бы живыми — откат доводит удаление
-- до конца, как было до 0086 (подзадачи уходят каскадом; у документов parent_id
-- RESTRICT, но одно DELETE проверяет его в конце оператора, а поддерево в корзине
-- лежит целиком).
DELETE FROM tasks WHERE deleted_at IS NOT NULL;
DELETE FROM documents WHERE deleted_at IS NOT NULL;
ALTER TABLE projects DROP COLUMN IF EXISTS archived_at;
DROP INDEX IF EXISTS idx_documents_deleted;
ALTER TABLE documents DROP COLUMN IF EXISTS deleted_by;
ALTER TABLE documents DROP COLUMN IF EXISTS deleted_at;
DROP INDEX IF EXISTS idx_tasks_deleted;
ALTER TABLE tasks DROP COLUMN IF EXISTS deleted_by;
ALTER TABLE tasks DROP COLUMN IF EXISTS deleted_at;
