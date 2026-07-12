-- Instance-wide GitLab service credential (OAuth era): an admin-configured service
-- account PAT that drives ALL sync + write operations, decoupling the integration
-- from any individual user's personal token. Lives on the same oauth_providers row
-- as the GitLab connection (base_url + OAuth app), encrypted with the AES-GCM sealer.
-- Additive/safe. When empty, the legacy per-user PAT path still applies.
ALTER TABLE oauth_providers ADD COLUMN service_token_enc text NOT NULL DEFAULT '';
