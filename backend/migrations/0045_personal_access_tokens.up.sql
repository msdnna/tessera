-- Personal Access Tokens: long-lived, revocable bearer credentials for
-- headless clients (MCP server, CI, scripts). Only the SHA-256 hash is stored,
-- mirroring refresh_tokens; the plaintext is shown once at creation.
CREATE TABLE personal_access_tokens (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name         text NOT NULL,
    token_hash   text NOT NULL UNIQUE,     -- SHA-256 hex of the plaintext token
    last_four    text NOT NULL,            -- display hint (never the full token)
    expires_at   timestamptz,             -- NULL = never expires
    revoked_at   timestamptz,
    last_used_at timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX personal_access_tokens_hash_idx ON personal_access_tokens (token_hash);
CREATE INDEX personal_access_tokens_user_idx ON personal_access_tokens (user_id);
