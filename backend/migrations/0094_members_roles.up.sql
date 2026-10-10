-- Участники пространства v2 (#3063): активность, запросы доступа, группы.
--
-- Передача владения (#3072) схемы не требует: владелец — это и
-- workspaces.owner_id, и memberships.role = 'owner'; ручка меняет оба в одной
-- транзакции.
--
-- Активность (#3073) — две отметки, обе обновляются дёшево и с троттлингом в
-- самом UPDATE (строка пишется не чаще раза в минуту):
--   * users.last_seen_at — когда человек был в приложении: выпуск токенов (вход и
--     каждый refresh, раз в ~15 минут работы) и подключение сокета;
--   * memberships.last_active_at — последнее действие в этом пространстве:
--     изменяющий запрос, прошедший проверку членства (lookupMembership).
ALTER TABLE users ADD COLUMN IF NOT EXISTS last_seen_at timestamptz;
ALTER TABLE memberships ADD COLUMN IF NOT EXISTS last_active_at timestamptz;

-- Запрос доступа (#3077): человек открыл ссылку на пространство или доску, в
-- которых он не состоит, и попросил впустить. Решают owner/admin: одобрение
-- создаёт членство (роль member или admin — не owner), отказ закрывает заявку.
-- Одна открытая заявка на (пространство, человека).
CREATE TABLE IF NOT EXISTS workspace_access_requests (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    -- Откуда пришли: доска по ссылке (NULL — ссылка на само пространство).
    board_id     uuid REFERENCES boards(id) ON DELETE SET NULL,
    message      text NOT NULL DEFAULT '',
    status       text NOT NULL DEFAULT 'pending'
                 CHECK (status IN ('pending', 'approved', 'declined')),
    decided_by   uuid REFERENCES users(id) ON DELETE SET NULL,
    decided_at   timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS workspace_access_requests_pending_key
    ON workspace_access_requests (workspace_id, user_id) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS workspace_access_requests_user
    ON workspace_access_requests (user_id, created_at DESC);

-- Группы участников (#3074): «Фронтенд», «Бэкенд»… — именованные наборы
-- участников пространства для назначения задач и упоминаний. Прав не дают:
-- назначение группы раскрывается в её участников, видимость остаётся за
-- членством в пространстве. Участник группы обязан быть участником
-- пространства — чтения соединяются с memberships, так что ушедший из
-- пространства из группы выпадает сам.
CREATE TABLE IF NOT EXISTS member_groups (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name         text NOT NULL,
    color        text NOT NULL DEFAULT '',
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS member_groups_ws_name_key
    ON member_groups (workspace_id, lower(name));

CREATE TABLE IF NOT EXISTS member_group_members (
    group_id uuid NOT NULL REFERENCES member_groups(id) ON DELETE CASCADE,
    user_id  uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    PRIMARY KEY (group_id, user_id)
);
CREATE INDEX IF NOT EXISTS member_group_members_user ON member_group_members (user_id);
