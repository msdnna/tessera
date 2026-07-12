DROP INDEX IF EXISTS gitlab_integrations_ws_project_key;
ALTER TABLE gitlab_integrations DROP CONSTRAINT IF EXISTS gitlab_integrations_board_id_key;
ALTER TABLE gitlab_integrations DROP COLUMN IF EXISTS closed_after;
ALTER TABLE gitlab_integrations DROP COLUMN IF EXISTS closed_policy;
ALTER TABLE gitlab_integrations DROP COLUMN IF EXISTS scope;
ALTER TABLE gitlab_integrations DROP COLUMN IF EXISTS name;
-- NOTE: only reversible if at most one integration per workspace remains.
ALTER TABLE gitlab_integrations ADD CONSTRAINT gitlab_integrations_workspace_id_key UNIQUE (workspace_id);
