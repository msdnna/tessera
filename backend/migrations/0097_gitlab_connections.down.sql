ALTER TABLE gitlab_credentials DROP COLUMN access_expires_at;
ALTER TABLE gitlab_credentials DROP COLUMN refresh_token_enc;
ALTER TABLE gitlab_credentials DROP COLUMN auth_kind;
ALTER TABLE oauth_providers DROP COLUMN service_token_expires_at;
ALTER TABLE gitlab_credentials DROP COLUMN token_expires_at;
ALTER TABLE gitlab_integrations DROP COLUMN pause_reason;
ALTER TABLE gitlab_integrations DROP COLUMN paused_at;
ALTER TABLE gitlab_integrations DROP COLUMN import_labels;

-- Back to one binding per board: of several on one board, the oldest stays (the
-- others go with their links, by cascade).
DELETE FROM gitlab_integrations a
USING gitlab_integrations b
WHERE a.board_id = b.board_id AND (a.created_at, a.id) > (b.created_at, b.id);
DROP INDEX IF EXISTS idx_gitlab_integrations_board;
ALTER TABLE gitlab_integrations ADD CONSTRAINT gitlab_integrations_board_id_key UNIQUE (board_id);
