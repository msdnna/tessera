-- Учёт времени (#2965): «потрачено» рядом с оценкой. Запись трудозатрат — кто,
-- сколько минут, за какой день и (необязательно) что делал. Сумма считается
-- запросом (своё и с подзадачами), отдельного счётчика на задаче нет.
CREATE TABLE IF NOT EXISTS task_time_entries (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id    uuid NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    user_id    uuid REFERENCES users(id) ON DELETE SET NULL,
    minutes    integer NOT NULL CHECK (minutes > 0 AND minutes <= 100000),
    note       text NOT NULL DEFAULT '',
    spent_on   date NOT NULL DEFAULT CURRENT_DATE,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_task_time_entries_task ON task_time_entries (task_id);
