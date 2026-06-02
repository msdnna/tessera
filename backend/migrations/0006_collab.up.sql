-- Phase: collaboration & rich tasks (features #3 and #8).
-- Adds the task activity journal, comments, relations, attachments and
-- per-user persistent notifications.

-- Task activity journal: an append-only history of what happened to a task.
CREATE TABLE task_events (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id    uuid NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    actor_id   uuid REFERENCES users(id) ON DELETE SET NULL,
    kind       text NOT NULL,            -- created | renamed | description | priority | due | completed | reopened | moved | assigned | unassigned | tagged | untagged | archived | restored | comment | relation | attachment
    data       jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_task_events_task ON task_events (task_id, created_at);

-- Comments on a task (the discussion thread in the advanced modal).
CREATE TABLE task_comments (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id    uuid NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    author_id  uuid REFERENCES users(id) ON DELETE SET NULL,
    body       text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_task_comments_task ON task_comments (task_id, created_at);

-- Directed relations between tasks (referenced in the UI by #N).
CREATE TABLE task_relations (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id         uuid NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    related_task_id uuid NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    kind            text NOT NULL DEFAULT 'relates',  -- relates | blocks | blocked_by | duplicates
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (task_id, related_task_id, kind),
    CHECK (task_id <> related_task_id)
);
CREATE INDEX idx_task_relations_task ON task_relations (task_id);
CREATE INDEX idx_task_relations_related ON task_relations (related_task_id);

-- File attachments on a task.
CREATE TABLE task_attachments (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id      uuid NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    uploader_id  uuid REFERENCES users(id) ON DELETE SET NULL,
    filename     text NOT NULL,
    content_type text NOT NULL DEFAULT '',
    size         bigint NOT NULL DEFAULT 0,
    storage_path text NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_task_attachments_task ON task_attachments (task_id);

-- Per-user persistent notifications (the activity bell, now durable).
CREATE TABLE notifications (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    task_id      uuid REFERENCES tasks(id) ON DELETE CASCADE,
    actor_id     uuid REFERENCES users(id) ON DELETE SET NULL,
    kind         text NOT NULL,           -- assigned | comment | mention | due_soon
    text         text NOT NULL,
    read_at      timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_notifications_user ON notifications (user_id, created_at DESC);
CREATE INDEX idx_notifications_unread ON notifications (user_id) WHERE read_at IS NULL;
