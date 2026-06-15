-- U2: account lifecycle. Email-verification + password-reset tokens, and
-- workspace invitations by email (invitee may not have an account yet). Raw
-- tokens are emailed; only their SHA-256 hash is stored (like refresh tokens).
-- All additive.

CREATE TABLE user_tokens (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind       text NOT NULL,          -- 'verify' | 'reset'
    token_hash text NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    used_at    timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_user_tokens_user ON user_tokens (user_id, kind);

CREATE TABLE workspace_invitations (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    email        text NOT NULL,
    role         text NOT NULL DEFAULT 'member',
    token_hash   text NOT NULL UNIQUE,
    invited_by   uuid REFERENCES users(id) ON DELETE SET NULL,
    expires_at   timestamptz NOT NULL,
    accepted_at  timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_ws_invitations_ws ON workspace_invitations (workspace_id);
CREATE INDEX idx_ws_invitations_email ON workspace_invitations (lower(email));
