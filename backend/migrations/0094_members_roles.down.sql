DROP TABLE IF EXISTS member_group_members;
DROP TABLE IF EXISTS member_groups;
DROP TABLE IF EXISTS workspace_access_requests;
ALTER TABLE memberships DROP COLUMN IF EXISTS last_active_at;
ALTER TABLE users DROP COLUMN IF EXISTS last_seen_at;
