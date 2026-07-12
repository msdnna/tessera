-- GitLab OAuth ("Login with GitLab") + a canonical GL↔Tessera identity link.
-- Loosely coupled (see CLAUDE.md): provider config and identities live in their
-- own tables; the core auth path only grows a callback that reuses issue(). A
-- GitLab-provisioned account keeps password_hash = '' (bcrypt of "" never matches,
-- so password login is impossible) instead of a nullable column — additive/safe.

-- Admin-configured OAuth application(s). One row per provider (currently 'gitlab').
-- client_secret is encrypted at rest with the same AES-256-GCM sealer as PATs.
-- org_map maps a GitLab group full-path to a Tessera workspace + role assignment,
-- AWX social-auth style:
--   { "<gl-group-path>": { "workspace_id": "<uuid>", "admins": ["v.sokolov"], "users": true } }
CREATE TABLE oauth_providers (
    provider          text PRIMARY KEY,               -- 'gitlab'
    client_id         text NOT NULL DEFAULT '',
    client_secret_enc text NOT NULL DEFAULT '',        -- AES-256-GCM(client secret), base64
    gl_base_url       text NOT NULL DEFAULT '',
    enabled           boolean NOT NULL DEFAULT false,
    org_map           jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);

-- Canonical link between a Tessera user and their external (GitLab) identity.
-- Used both for login (find/attach the account) and for sync assignee attribution
-- (map a GitLab assignee → Tessera user). provider_user_id is GitLab's numeric id
-- (stable across username changes).
CREATE TABLE oauth_identities (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider         text NOT NULL,                    -- 'gitlab'
    provider_user_id text NOT NULL,                    -- numeric GitLab user id (as text)
    provider_username text NOT NULL DEFAULT '',
    provider_email   text NOT NULL DEFAULT '',
    gl_base_url      text NOT NULL DEFAULT '',
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider, provider_user_id)
);
CREATE INDEX idx_oauth_identities_user ON oauth_identities (user_id);
CREATE INDEX idx_oauth_identities_username ON oauth_identities (provider, provider_username);
