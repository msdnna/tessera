-- GitLab issue hierarchy: grouped parents and pushed children (task #2592). Additive.
--
-- Loose coupling (see CLAUDE.md): the core `tasks` tree is untouched — a subtask is
-- still just a task with a parent_id. Everything GitLab-shaped lives on the
-- integration's own gitlab_links row.
--
-- Numbering: 0054 is also claimed by task/2712, task/2713 and feat/2718-documents
-- (which runs 0054..0060). Taking a "free" higher number to dodge them is wrong — it
-- leaves a gap on THIS branch, and backend/e2e/migrate_test.go requires contiguous
-- numbering (highest == number of .up.sql files). Numbers are diverged at merge time
-- by renumbering the later branch, not reserved in advance (see CLAUDE.md).
--
-- gl_is_group records that the mirrored issue carries the grouping label. The pull
-- stays the source of truth for it, but storing it lets a sync keep asking GitLab for
-- an ex-grouped parent's children after the label was dropped, instead of reading the
-- missing label as "every subtask was detached".
ALTER TABLE gitlab_links ADD COLUMN gl_is_group boolean NOT NULL DEFAULT false;

-- The issue's WorkItem global id ("gid://gitlab/WorkItem/<n>"). Deliberately NOT
-- derived from gl_global_id: a WorkItem gid and an Issue gid share the trailing
-- number today, but that is GitLab's internal business, not a documented identity.
-- Cached from project.workItems so the hierarchy mutation never has to guess.
ALTER TABLE gitlab_links ADD COLUMN gl_work_item_id text NOT NULL DEFAULT '';

-- The parent's WorkItem global id, when this issue is a child in GitLab's hierarchy.
-- Empty means "top-level as far as GitLab is concerned".
ALTER TABLE gitlab_links ADD COLUMN gl_parent_global_id text NOT NULL DEFAULT '';

-- Idempotency for child creation. Every other change_kind is a re-pushable update, so
-- a duplicate row is harmless there; 'child_create' opens a NEW issue in GitLab, and a
-- second row would open a second one. At most one in flight per task, enforced by the
-- database rather than by a read-then-write race in the enqueue path.
CREATE UNIQUE INDEX idx_gitlab_writebacks_child_create_inflight
    ON gitlab_writebacks (task_id)
    WHERE change_kind = 'child_create' AND status IN ('pending', 'sending');
