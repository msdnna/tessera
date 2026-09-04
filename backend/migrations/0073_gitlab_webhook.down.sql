ALTER TABLE gitlab_integrations DROP COLUMN IF EXISTS last_webhook_at;
ALTER TABLE gitlab_integrations DROP COLUMN IF EXISTS webhook_enabled;
ALTER TABLE gitlab_integrations DROP COLUMN IF EXISTS webhook_secret_enc;
