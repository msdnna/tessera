ALTER TABLE projects   DROP COLUMN IF EXISTS estimation;
ALTER TABLE workspaces DROP COLUMN IF EXISTS estimation;
ALTER TABLE tasks      DROP COLUMN IF EXISTS estimate;
