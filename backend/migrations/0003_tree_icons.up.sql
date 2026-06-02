-- Phase 10a: nested project groups + project icons.

-- Projects get an icon (emoji/glyph or short text; empty = derive initials in UI).
ALTER TABLE projects ADD COLUMN icon text NOT NULL DEFAULT '';

-- Project groups can nest (группа → подгруппа → проект). Deleting a parent
-- cascades to its subgroups (their projects fall back to ungrouped via the
-- existing projects.group_id ON DELETE SET NULL).
ALTER TABLE project_groups
    ADD COLUMN parent_id uuid REFERENCES project_groups(id) ON DELETE CASCADE;
CREATE INDEX idx_project_groups_parent ON project_groups (parent_id);
