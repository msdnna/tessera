ALTER TABLE oauth_providers DROP COLUMN IF EXISTS sudo_writeback;
ALTER TABLE gitlab_writebacks DROP COLUMN IF EXISTS actor_user_id;
