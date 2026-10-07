-- «Входящие» (#2972: #2995, #3037, #3038, #3039, #3041): заметки и напоминания
-- одной лентой. Две таблицы остаются (решение #3036), меняются их поля.
--
--   * notes.task_id — заметка, привязанная к задаче (раздел «Напоминания и
--     заметки» в модалке задачи). Удаление задачи заметку не уносит.
--   * notes.visibility — private (видна только автору) | workspace (всему
--     пространству). Новые записи из захвата — private; существующие заметки
--     были общими и такими остаются (DEFAULT 'workspace').
--   * reminders.body — текст напоминания (Markdown); message остаётся заголовком.
--   * reminders.done_at — когда отметили выполненным: «Выполнено сегодня»
--     держится в ленте зачёркнутым до полуночи. Поле done не трогаем — его
--     читают сканер доставки и Android; done_at ставится и снимается вместе с ним.
--   * reminders.snoozed_from — исходное время отложенного напоминания (чип
--     «с 12:00»); снимается, когда время меняют не через «Отложить».
--   * reminders.workspace_id — пространство, в ленте которого лежит
--     напоминание. Захват ставит текущее. NULL — «вне пространств»: такие
--     (старые, без задачи) видны в ленте любого пространства.
--     Бэкфилл: напоминание с задачей получает пространство задачи.
--
-- Аддитивная и повторяемая: шаги под IF [NOT] EXISTS, бэкфилл трогает только
-- пустые значения.

ALTER TABLE notes ADD COLUMN IF NOT EXISTS task_id uuid REFERENCES tasks(id) ON DELETE SET NULL;
ALTER TABLE notes ADD COLUMN IF NOT EXISTS visibility text NOT NULL DEFAULT 'workspace';
ALTER TABLE notes DROP CONSTRAINT IF EXISTS notes_visibility_check;
ALTER TABLE notes ADD CONSTRAINT notes_visibility_check CHECK (visibility IN ('private', 'workspace'));
CREATE INDEX IF NOT EXISTS idx_notes_task ON notes (task_id) WHERE task_id IS NOT NULL;

ALTER TABLE reminders ADD COLUMN IF NOT EXISTS body text NOT NULL DEFAULT '';
ALTER TABLE reminders ADD COLUMN IF NOT EXISTS done_at timestamptz;
ALTER TABLE reminders ADD COLUMN IF NOT EXISTS snoozed_from timestamptz;
ALTER TABLE reminders ADD COLUMN IF NOT EXISTS workspace_id uuid REFERENCES workspaces(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_reminders_task ON reminders (task_id) WHERE task_id IS NOT NULL;

UPDATE reminders r SET workspace_id = p.workspace_id
FROM tasks t
JOIN boards b ON b.id = t.board_id
JOIN projects p ON p.id = b.project_id
WHERE r.task_id = t.id AND r.workspace_id IS NULL;
