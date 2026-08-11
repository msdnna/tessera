-- OAuth provider config + external identities (see migration 0042).

-- name: GetOAuthProvider :one
SELECT * FROM oauth_providers WHERE provider = $1;

-- UpsertOAuthProvider stores the admin-configured OAuth app + service token.
-- name: UpsertOAuthProvider :one
INSERT INTO oauth_providers (provider, client_id, client_secret_enc, gl_base_url, enabled, org_map, service_token_enc, sudo_writeback, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
ON CONFLICT (provider) DO UPDATE
SET client_id = EXCLUDED.client_id,
    client_secret_enc = EXCLUDED.client_secret_enc,
    gl_base_url = EXCLUDED.gl_base_url,
    enabled = EXCLUDED.enabled,
    org_map = EXCLUDED.org_map,
    service_token_enc = EXCLUDED.service_token_enc,
    sudo_writeback = EXCLUDED.sudo_writeback,
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

-- GetGitlabUsernameForUser returns a user's GitLab username from their OAuth
-- identity (for attributing issues created under a shared service token).
-- name: GetGitlabUsernameForUser :one
SELECT provider_username FROM oauth_identities WHERE provider = 'gitlab' AND user_id = $1 LIMIT 1;

-- GetGitlabAvatarForUser returns the (already-proxied) GitLab avatar URL for a
-- Tessera user, resolved from the synced project-member roster via their GitLab
-- identity (OAuth login or connected PAT). Lets an OAuth account show its GitLab
-- avatar without a separate download.
-- name: GetGitlabAvatarForUser :one
SELECT m.gl_avatar_url
FROM gitlab_project_members m
WHERE m.gl_avatar_url <> ''
  AND m.gl_username IN (
    SELECT oi.provider_username FROM oauth_identities oi WHERE oi.user_id = $1 AND oi.provider = 'gitlab'
    UNION
    SELECT gc.gl_username FROM gitlab_credentials gc WHERE gc.user_id = $1
  )
LIMIT 1;

-- CreateOAuthUser provisions a GitLab-authenticated account: no password (bcrypt of
-- "" never matches), email pre-verified (GitLab vouches for it), provider marked.
-- name: CreateOAuthUser :one
INSERT INTO users (email, name, password_hash, is_admin, provider, email_verified)
VALUES ($1, $2, '', $3, $4, true)
RETURNING *;
