-- Встречи v2 (#3034): повтор и длительность (#3052), привязка к этапу (#3053),
-- задачи из чата звонка (#3055).
--
-- conferences:
--   * duration_minutes — сколько длится встреча. NULL — не задано (мгновенная
--     встреча, старые строки): список тогда не пишет длительность.
--   * recurrence — повтор: '' (не повторять), daily, weekdays (по будням),
--     weekly, biweekly (каждые 2 недели), monthly. Текст с CHECK, а не jsonb
--     правил задач (0027): у встречи нет триггеров «по колонке» и подзадач, а
--     пресеты макета — это и есть весь набор; новый пресет — одна строка CHECK.
--     Конференция и так переиспользуемая комната (#2879), поэтому повтор — не
--     копии строк, а «следующий раз» (next_at), который сервер считает на чтении
--     от scheduled_at.
--   * timezone — IANA-пояс, в котором повтор держит время на часах: «по будням в
--     11:00» остаётся 11:00 и после перевода часов. '' — пояс профиля автора не
--     был известен, считаем в UTC.
--   * milestone_id — этап, к которому привязана встреча. SET NULL: удаление этапа
--     не должно удалять встречу, её чат и записи.
--
-- conference_messages:
--   * kind — 'text' (обычная строка) или 'task' (системная строка «создал(а)
--     задачу #N», пишется командой «/» в чате).
--   * task_id — задача такой строки. SET NULL: удалённая задача оставляет строку
--     с текстом, но без ссылки.
--
-- Аддитивная: старые клиенты о колонках не знают; PATCH без новых полей их не
-- трогает (см. UpdateConference).
ALTER TABLE conferences
    ADD COLUMN IF NOT EXISTS duration_minutes integer
        CHECK (duration_minutes IS NULL OR duration_minutes BETWEEN 1 AND 1440),
    ADD COLUMN IF NOT EXISTS recurrence text NOT NULL DEFAULT ''
        CHECK (recurrence IN ('', 'daily', 'weekdays', 'weekly', 'biweekly', 'monthly')),
    ADD COLUMN IF NOT EXISTS timezone text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS milestone_id uuid REFERENCES milestones(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_conferences_milestone
    ON conferences (milestone_id) WHERE milestone_id IS NOT NULL;

ALTER TABLE conference_messages
    ADD COLUMN IF NOT EXISTS kind text NOT NULL DEFAULT 'text'
        CHECK (kind IN ('text', 'task')),
    ADD COLUMN IF NOT EXISTS task_id uuid REFERENCES tasks(id) ON DELETE SET NULL;
