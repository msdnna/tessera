-- GitLab connections v2 (#3065): several projects on one board, an OAuth
-- connection instead of a PAT, the token's expiry with a paused sync, and an
-- import filter by label. GitLab-only tables — the core is untouched.

-- #3082: several GitLab projects may mirror into one board. A card still belongs
-- to exactly one binding (gitlab_links is keyed by integration), so two bindings
-- never fight over a card; the same project twice in a workspace stays refused
-- (gitlab_integrations_ws_project_key).
ALTER TABLE gitlab_integrations DROP CONSTRAINT gitlab_integrations_board_id_key;
CREATE INDEX idx_gitlab_integrations_board ON gitlab_integrations (board_id);

-- #3085: import only issues carrying every one of these labels (GitLab's own
-- labelName filter). Empty = no filter.
ALTER TABLE gitlab_integrations ADD COLUMN import_labels text[] NOT NULL DEFAULT '{}';

-- #3084: GitLab refused the credential (401) — the unattended sync sits out until
-- someone reconnects or a manual sync gets through. pause_reason: 'unauthorized'.
ALTER TABLE gitlab_integrations ADD COLUMN paused_at timestamptz;
ALTER TABLE gitlab_integrations ADD COLUMN pause_reason text NOT NULL DEFAULT '';

-- #3084: when the personal access token expires (GitLab's
-- /personal_access_tokens/self), NULL = never or unknown.
ALTER TABLE gitlab_credentials ADD COLUMN token_expires_at timestamptz;
ALTER TABLE oauth_providers ADD COLUMN service_token_expires_at timestamptz;

-- #3083: a connection made through GitLab OAuth. token_enc then holds the access
-- token (encrypted like a PAT), refresh_token_enc the refresh token, and
-- access_expires_at when the access token has to be refreshed.
ALTER TABLE gitlab_credentials ADD COLUMN auth_kind text NOT NULL DEFAULT 'pat'
    CHECK (auth_kind IN ('pat', 'oauth'));
ALTER TABLE gitlab_credentials ADD COLUMN refresh_token_enc text NOT NULL DEFAULT '';
ALTER TABLE gitlab_credentials ADD COLUMN access_expires_at timestamptz;
