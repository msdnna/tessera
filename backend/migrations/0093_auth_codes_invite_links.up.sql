-- Вход и приглашения v2 (#3035). Всё аддитивно.
--
-- 1. Коды подтверждения почты (#3057) и смена адреса (#3058) живут в той же
--    user_tokens, что и ссылки: kind 'verify_code' | 'email_change'. Код хранится
--    HMAC-хешем с солью в token_hash (сырой код — только в письме), у строки есть
--    счётчик попыток, а у смены адреса — сам новый адрес, который ещё не доказан.
ALTER TABLE user_tokens ADD COLUMN IF NOT EXISTS attempts integer NOT NULL DEFAULT 0;
ALTER TABLE user_tokens ADD COLUMN IF NOT EXISTS email text;
CREATE INDEX IF NOT EXISTS idx_user_tokens_recent ON user_tokens (user_id, created_at);

-- 2. Адресные приглашения: «Отклонить» (#3059) и «Ещё раз / Продлить» (#3061).
--    sent_at — когда письмо ушло последний раз (строка «отправлено 2 дня назад» и
--    пауза между повторами), send_count — сколько раз.
ALTER TABLE workspace_invitations ADD COLUMN IF NOT EXISTS declined_at timestamptz;
ALTER TABLE workspace_invitations ADD COLUMN IF NOT EXISTS sent_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE workspace_invitations ADD COLUMN IF NOT EXISTS send_count integer NOT NULL DEFAULT 1;
UPDATE workspace_invitations SET sent_at = created_at;

-- 3. Многоразовая ссылка-приглашение (#3062): одна живая на пространство, токен
--    только хешем, срок, роль, лимит использований и отзыв. Каждое вступление по
--    ссылке записывается — это и аудит («кто пришёл по какой ссылке»), и защита
--    от повторного расхода одного места одним человеком.
CREATE TABLE IF NOT EXISTS workspace_invite_links (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    token_hash   text NOT NULL UNIQUE,
    role         text NOT NULL DEFAULT 'member',
    max_uses     integer NOT NULL CHECK (max_uses > 0),
    uses         integer NOT NULL DEFAULT 0 CHECK (uses >= 0),
    created_by   uuid REFERENCES users(id) ON DELETE SET NULL,
    expires_at   timestamptz NOT NULL,
    revoked_at   timestamptz,
    revoked_by   uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_invite_links_live
    ON workspace_invite_links (workspace_id) WHERE revoked_at IS NULL;

CREATE TABLE IF NOT EXISTS workspace_invite_link_uses (
    link_id uuid NOT NULL REFERENCES workspace_invite_links(id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    used_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (link_id, user_id)
);
