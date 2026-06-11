-- Per-user saved board views (layouts): a named snapshot of the board toolbar
-- state (grouping, sort, filters, expanded subtasks, layout) stored server-side
-- so it follows the user across devices. The config shape is owned by the
-- frontend and kept opaque here.
CREATE TABLE board_views (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    board_id   uuid NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name       text NOT NULL,
    config     jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (board_id, user_id, name)
);
CREATE INDEX idx_board_views_board_user ON board_views (board_id, user_id);
