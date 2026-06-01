-- Phase 1 — core domain model.
-- Ordering: float8 `position` (classic kanban-style midpoint between neighbours).
-- Simpler/safer than string LexoRank; rebalance on precision exhaustion is a
-- deferred concern (columns rarely exceed dozens of cards in this app).

-- Users / auth -------------------------------------------------------------
CREATE TABLE users (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email         text NOT NULL UNIQUE,
    name          text NOT NULL,
    password_hash text NOT NULL,
    is_admin      boolean NOT NULL DEFAULT false,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE refresh_tokens (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash text NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_refresh_tokens_user ON refresh_tokens (user_id);

-- Workspaces / membership --------------------------------------------------
CREATE TABLE workspaces (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name       text NOT NULL,
    owner_id   uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE memberships (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role         text NOT NULL DEFAULT 'member',  -- owner | admin | member
    created_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, user_id)
);
CREATE INDEX idx_memberships_user ON memberships (user_id);

-- Project groups / projects ------------------------------------------------
CREATE TABLE project_groups (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name         text NOT NULL,
    position     double precision NOT NULL DEFAULT 65536,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_project_groups_workspace ON project_groups (workspace_id);

CREATE TABLE projects (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    group_id     uuid REFERENCES project_groups(id) ON DELETE SET NULL,
    name         text NOT NULL,
    color        text NOT NULL DEFAULT '',
    position     double precision NOT NULL DEFAULT 65536,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_projects_workspace ON projects (workspace_id);
CREATE INDEX idx_projects_group ON projects (group_id);

-- Boards / columns ---------------------------------------------------------
CREATE TABLE boards (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name       text NOT NULL,
    position   double precision NOT NULL DEFAULT 65536,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_boards_project ON boards (project_id);

CREATE TABLE board_columns (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    board_id   uuid NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
    name       text NOT NULL,
    color      text NOT NULL DEFAULT '',
    position   double precision NOT NULL DEFAULT 65536,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_board_columns_board ON board_columns (board_id);

-- Tags ---------------------------------------------------------------------
CREATE TABLE tags (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name         text NOT NULL,
    color        text NOT NULL DEFAULT '',
    created_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, name)
);

-- Tasks --------------------------------------------------------------------
CREATE TABLE tasks (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    board_id     uuid NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
    column_id    uuid NOT NULL REFERENCES board_columns(id) ON DELETE CASCADE,
    parent_id    uuid REFERENCES tasks(id) ON DELETE CASCADE,
    title        text NOT NULL,
    description  text NOT NULL DEFAULT '',
    priority     integer NOT NULL DEFAULT 0,  -- 0 none, 1 low, 2 normal, 3 high, 4 urgent
    due_date     timestamptz,
    position     double precision NOT NULL DEFAULT 65536,
    created_by   uuid REFERENCES users(id) ON DELETE SET NULL,
    completed_at timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_tasks_board ON tasks (board_id);
CREATE INDEX idx_tasks_column ON tasks (column_id);
CREATE INDEX idx_tasks_parent ON tasks (parent_id);

CREATE TABLE task_tags (
    task_id uuid NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    tag_id  uuid NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (task_id, tag_id)
);
CREATE INDEX idx_task_tags_tag ON task_tags (tag_id);

CREATE TABLE task_assignees (
    task_id uuid NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    PRIMARY KEY (task_id, user_id)
);
CREATE INDEX idx_task_assignees_user ON task_assignees (user_id);

-- Notes / reminders --------------------------------------------------------
CREATE TABLE notes (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    project_id   uuid REFERENCES projects(id) ON DELETE CASCADE,
    author_id    uuid REFERENCES users(id) ON DELETE SET NULL,
    title        text NOT NULL,
    body         text NOT NULL DEFAULT '',
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_notes_workspace ON notes (workspace_id);

CREATE TABLE reminders (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    task_id    uuid REFERENCES tasks(id) ON DELETE CASCADE,
    remind_at  timestamptz NOT NULL,
    message    text NOT NULL DEFAULT '',
    done       boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_reminders_user ON reminders (user_id);
