DROP TABLE IF EXISTS workspace_invite_link_uses;
DROP TABLE IF EXISTS workspace_invite_links;
ALTER TABLE workspace_invitations DROP COLUMN IF EXISTS send_count;
ALTER TABLE workspace_invitations DROP COLUMN IF EXISTS sent_at;
ALTER TABLE workspace_invitations DROP COLUMN IF EXISTS declined_at;
DROP INDEX IF EXISTS idx_user_tokens_recent;
-- Codes and pending address changes have no meaning without these columns.
DELETE FROM user_tokens WHERE kind IN ('verify_code', 'email_change');
ALTER TABLE user_tokens DROP COLUMN IF EXISTS email;
ALTER TABLE user_tokens DROP COLUMN IF EXISTS attempts;
