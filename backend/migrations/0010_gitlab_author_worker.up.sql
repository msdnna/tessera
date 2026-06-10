-- GitLab integration phase A follow-ups: capture the issue author (a GitLab
-- identity that may not exist as a Tessera user) and prepare for an auto-sync
-- worker. All additive.

-- The GitLab issue author, recorded on the link so the UI can attribute a
-- synced task to its GitLab creator by username even when there's no matching
-- Tessera user.
ALTER TABLE gitlab_links ADD COLUMN gl_author      text NOT NULL DEFAULT '';
ALTER TABLE gitlab_links ADD COLUMN gl_author_name text NOT NULL DEFAULT '';

-- Worker prep: which user's credential drives unattended sync, how often, and
-- when it last ran. sync_interval_sec = 0 means manual-only (no auto-sync).
ALTER TABLE gitlab_integrations ADD COLUMN owner_user_id    uuid REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE gitlab_integrations ADD COLUMN sync_interval_sec integer NOT NULL DEFAULT 0;
ALTER TABLE gitlab_integrations ADD COLUMN last_synced_at   timestamptz;
