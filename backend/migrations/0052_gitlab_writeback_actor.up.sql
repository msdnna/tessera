-- Attribute GitLab writes to the acting Tessera user (task #2690).
-- Additive/safe on the live DB.
--
-- Loose coupling (see CLAUDE.md): no GitLab-specific column leaks into the core —
-- the writeback outbox simply remembers who triggered the row so the async worker
-- can pick that user's GitLab credential (personal PAT, else admin `Sudo:` header)
-- instead of always the shared service account. NULL = system/sync or a legacy row.
ALTER TABLE gitlab_writebacks
    ADD COLUMN actor_user_id uuid REFERENCES users(id) ON DELETE SET NULL;

-- Admin toggle for sudo impersonation. When on (and the service token is an admin
-- PAT with scope api+sudo), writes carry a `Sudo: <gitlab-username>` header so
-- GitLab records the real acting user as author. Off by default: enabling it means
-- Tessera holds an admin-scoped instance token, so it's an explicit opt-in for
-- deployments where that concentration of privilege is acceptable (GitLab + Tessera
-- not exposed to the public network).
ALTER TABLE oauth_providers
    ADD COLUMN sudo_writeback boolean NOT NULL DEFAULT false;
