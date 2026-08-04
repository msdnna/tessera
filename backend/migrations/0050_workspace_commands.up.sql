-- Workspace-scoped dictionary of custom editor commands ("/approve", "/hold").
-- These are autocomplete entries only: the quick-action engine never executes
-- them, so they survive into the comment text as a note to whoever reads it.
-- Built-in command keys are rejected at the handler layer to keep the two sets
-- from colliding.
CREATE TABLE workspace_commands (
    workspace_id uuid        NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    key          text        NOT NULL,           -- canonical, without the leading "/"
    description  text        NOT NULL DEFAULT '',
    position     int         NOT NULL DEFAULT 0, -- popup order, as the admin arranged it
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, key)
);
