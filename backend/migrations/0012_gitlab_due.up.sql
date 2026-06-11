-- GitLab due-date sync: take a task's due from the issue or its milestone's End
-- date, configurable per integration, with manual edits winning. Additive.

-- A manually-set Tessera due date wins over GitLab: once the user edits a linked
-- task's due, the sync stops touching it.
ALTER TABLE gitlab_links ADD COLUMN due_overridden boolean NOT NULL DEFAULT false;

-- Where the sync takes a task's due from:
--   issue_milestone (default) = issue due, else the milestone End date
--   issue                     = only the issue's own due
--   milestone                 = only the milestone End date
--   off                       = never sync the due
ALTER TABLE gitlab_integrations ADD COLUMN due_source text NOT NULL DEFAULT 'issue_milestone';
