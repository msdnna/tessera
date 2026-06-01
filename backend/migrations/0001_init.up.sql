-- Phase 0 bootstrap.
-- pgcrypto provides gen_random_uuid() on older Postgres; PG13+ has it built-in
-- but enabling the extension is harmless and keeps the schema portable.
-- The full domain model (workspaces, project_groups, projects, boards,
-- columns, tasks, tags, task_tags, assignees, notes, reminders) lands in
-- Phase 1.
CREATE EXTENSION IF NOT EXISTS pgcrypto;
