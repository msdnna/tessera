-- Configurable project-tree navigation: a project can show boards, milestones
-- (sprints), or both as its children in the sidebar. Milestones remain a
-- navigation overlay over the project's board(s), not a physical second board —
-- so no task-model change. Additive/safe.
--   boards (default) | milestones | both
ALTER TABLE projects ADD COLUMN tree_mode text NOT NULL DEFAULT 'boards';
