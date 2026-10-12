-- Команды-макросы палитры ⌘K и шаблоны задач (#3069). Отдельно от словаря «/»
-- (workspace_commands, 0050) — тот остаётся подсказками комментариев.
--
-- task_templates: заготовка задачи пространства — заголовок, описание (Markdown,
-- в т.ч. чекбоксы), теги по имени (сопоставляются с тегами проекта при создании),
-- приоритет, подзадачи (заголовки).
CREATE TABLE IF NOT EXISTS task_templates (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name         text NOT NULL,
    title        text NOT NULL DEFAULT '',
    description  text NOT NULL DEFAULT '',
    tags         text[] NOT NULL DEFAULT '{}',
    priority     integer NOT NULL DEFAULT 0,
    subtasks     text[] NOT NULL DEFAULT '{}',
    created_by   uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_task_templates_ws ON task_templates (workspace_id);

-- workspace_macros: исполняемая команда. owner_id NULL — общая (правят owner/admin),
-- иначе личная. action — что делает (create_task | open_link | create_document |
-- run_pipeline), params — его параметры; trigger — вызов в палитре («bug» → >bug),
-- keys — клавиша из каталога web (`Alt+KeyB`), category — группа в окне.
CREATE TABLE IF NOT EXISTS workspace_macros (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    owner_id     uuid REFERENCES users(id) ON DELETE CASCADE,
    name         text NOT NULL,
    trigger      text NOT NULL,
    action       text NOT NULL CHECK (action IN ('create_task', 'open_link', 'create_document', 'run_pipeline')),
    params       jsonb NOT NULL DEFAULT '{}',
    keys         text NOT NULL DEFAULT '',
    category     text NOT NULL DEFAULT '',
    enabled      boolean NOT NULL DEFAULT true,
    position     double precision NOT NULL DEFAULT 0,
    created_by   uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_workspace_macros_ws ON workspace_macros (workspace_id);
