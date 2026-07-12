-- OAuth provider config + external identities (see migration 0042).

-- name: GetOAuthProvider :one
SELECT * FROM oauth_providers WHERE provider = $1;

-- UpsertOAuthProvider stores the admin-configured OAuth app for a provider.
-- name: UpsertOAuthProvider :one
INSERT INTO oauth_providers (provider, client_id, client_secret_enc, gl_base_url, enabled, org_map, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, now())
ON CONFLICT (provider) DO UPDATE
SET client_id = EXCLUDED.client_id,
    client_secret_enc = EXCLUDED.client_secret_enc,
    gl_base_url = EXCLUDED.gl_base_url,
    enabled = EXCLUDED.enabled,
    org_map = EXCLUDED.org_map,
    updated_at = now()
RETURNING *;

-- name: GetOAuthIdentity :one
SELECT * FROM oauth_identities WHERE provider = $1 AND provider_user_id = $2;

-- UpsertOAuthIdentity records/refreshes a user's external identity.
-- name: UpsertOAuthIdentity :one
INSERT INTO oauth_identities (user_id, provider, provider_user_id, provider_username, provider_email, gl_base_url, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, now())
ON CONFLICT (provider, provider_user_id) DO UPDATE
SET user_id = EXCLUDED.user_id,
    provider_username = EXCLUDED.provider_username,
    provider_email = EXCLUDED.provider_email,
    gl_base_url = EXCLUDED.gl_base_url,
    updated_at = now()
RETURNING *;

-- GetUserIDByOAuthUsername resolves a GitLab username to a Tessera user via a
-- recorded OAuth identity (login-grade link, complements gitlab_credentials).
-- name: GetUserIDByOAuthUsername :one
SELECT user_id FROM oauth_identities WHERE provider = 'gitlab' AND provider_username = $1;

-- CreateOAuthUser provisions a GitLab-authenticated account: no password (bcrypt of
-- "" never matches), email pre-verified (GitLab vouches for it), provider marked.
-- name: CreateOAuthUser :one
INSERT INTO users (email, name, password_hash, is_admin, provider, email_verified)
VALUES ($1, $2, '', $3, $4, true)
RETURNING *;
