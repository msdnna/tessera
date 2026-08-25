-- Stable key for the auto-created personal workspace (#2800).
--
-- Registration seeds every user a workspace literally named «Личное пространство»,
-- so the name arrived at the client as a finished Russian string and an English
-- UI had nothing to translate. From here the seed also writes `name_key`, and the
-- client renders the caption from its own catalogue. `name` keeps being filled as
-- the fallback for old clients, e-mail templates and DB lookups.
--
-- A user rename clears the key (see UpdateWorkspace): once someone calls their
-- space «Мой хлам», redrawing it as "Personal space" on an English UI would be
-- wrong. That is the line between a default and a chosen name.
ALTER TABLE workspaces ADD COLUMN name_key text;

-- Backfill the existing seeds by exact name match: a workspace someone already
-- renamed does not match and stays keyless, which is the same rule the rename
-- path enforces going forward.
UPDATE workspaces SET name_key = 'personal' WHERE name = 'Личное пространство';
