DROP INDEX IF EXISTS idx_gitlab_writebacks_child_create_inflight;
ALTER TABLE gitlab_links DROP COLUMN IF EXISTS gl_parent_global_id;
ALTER TABLE gitlab_links DROP COLUMN IF EXISTS gl_work_item_id;
ALTER TABLE gitlab_links DROP COLUMN IF EXISTS gl_is_group;
