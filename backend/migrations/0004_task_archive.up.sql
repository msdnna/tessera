-- Soft-delete / archive for tasks. archived_at NULL = active.
ALTER TABLE tasks ADD COLUMN archived_at timestamptz;
CREATE INDEX idx_tasks_archived ON tasks (board_id, archived_at);
