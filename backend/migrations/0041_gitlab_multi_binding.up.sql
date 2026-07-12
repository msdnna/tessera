-- GitLab multi-binding: allow several GitLab project → board bindings per
-- workspace (was one-per-workspace). The schema is already keyed by
-- integration_id throughout (links, milestone_links, writebacks, sync_runs,
-- project_members), so this only relaxes the workspace uniqueness and adds
-- per-binding config. Additive/safe on the live DB.

-- Drop the one-integration-per-workspace unique.
ALTER TABLE gitlab_integrations DROP CONSTRAINT gitlab_integrations_workspace_id_key;

-- Human-readable label for the binding (shown in the config list / journal).
ALTER TABLE gitlab_integrations ADD COLUMN name text NOT NULL DEFAULT '';

-- Import scope: which issues the pull fetches.
--   assigned (default, legacy) = only the credential owner's assigned issues
--   all                        = every issue in the GitLab project
ALTER TABLE gitlab_integrations ADD COLUMN scope text NOT NULL DEFAULT 'assigned';

-- Closed-issue policy (consumed in phase C, stored here so the config is stable):
--   all (default, legacy)   = import all closed issues into the board (Done column)
--   archive_closed_sprints  = closed issues of a closed milestone go to the archive;
--                             closed issues of an open/no milestone go to Done
--   period                  = only import closed issues updated at/after closed_after
ALTER TABLE gitlab_integrations ADD COLUMN closed_policy text NOT NULL DEFAULT 'all';
ALTER TABLE gitlab_integrations ADD COLUMN closed_after  timestamptz;

-- A board mirrors at most one GitLab project (prevents two bindings fighting over
-- the same board's cards).
ALTER TABLE gitlab_integrations ADD CONSTRAINT gitlab_integrations_board_id_key UNIQUE (board_id);

-- No duplicate binding of the same GitLab project inside one workspace.
CREATE UNIQUE INDEX gitlab_integrations_ws_project_key
    ON gitlab_integrations (workspace_id, project_path);
