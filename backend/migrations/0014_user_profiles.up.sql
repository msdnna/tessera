-- U1 user-management: local-user profile fields, per-user preferences, and
-- avatar blobs. All additive. GitLab / external identities stay display-only
-- (see 0011) and are NOT touched here — `users.provider` marks the account
-- source so a future OAuth/SSO login can coexist with local accounts.

ALTER TABLE users ADD COLUMN provider       text    NOT NULL DEFAULT 'local';
ALTER TABLE users ADD COLUMN active         boolean NOT NULL DEFAULT true;
ALTER TABLE users ADD COLUMN email_verified boolean NOT NULL DEFAULT false;
-- Legal name, split (display name stays in users.name). All optional.
ALTER TABLE users ADD COLUMN last_name      text    NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN first_name     text    NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN middle_name    text    NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN bio            text    NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN company        text    NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN job_title      text    NOT NULL DEFAULT '';

-- Per-user preferences (1:1). Localizing fields drive date pickers / formats /
-- (later) i18n; personalizing fields hold the appearance moved out of the web
-- client's localStorage (accent + dark + board background).
CREATE TABLE user_preferences (
    user_id          uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    language         text     NOT NULL DEFAULT 'ru',
    timezone         text     NOT NULL DEFAULT '',
    country          text     NOT NULL DEFAULT '',
    time_format      text     NOT NULL DEFAULT '24h',        -- 24h | 12h
    date_format      text     NOT NULL DEFAULT 'dd.MM.yyyy',
    week_start       smallint NOT NULL DEFAULT 1,            -- 1 = Monday (ISO)
    theme            text     NOT NULL DEFAULT 'system',     -- system | light | dark
    accent           text     NOT NULL DEFAULT 'purple',
    board_background text     NOT NULL DEFAULT '',
    updated_at       timestamptz NOT NULL DEFAULT now()
);

-- Avatar blobs kept out of the hot users row; served via GET /api/users/:id/avatar.
CREATE TABLE user_avatars (
    user_id      uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    content_type text  NOT NULL,
    bytes        bytea NOT NULL,
    updated_at   timestamptz NOT NULL DEFAULT now()
);
