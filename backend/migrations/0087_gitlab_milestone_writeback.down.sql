-- Откат 0087: база, ошибка записи и конфликт этапа GitLab.
ALTER TABLE gitlab_milestone_links
    DROP COLUMN IF EXISTS conflict,
    DROP COLUMN IF EXISTS push_actor_id,
    DROP COLUMN IF EXISTS push_fields,
    DROP COLUMN IF EXISTS push_error,
    DROP COLUMN IF EXISTS gl_snapshot;
