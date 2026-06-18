-- Friendly display names for tag prefixes. Tags use a "<prefix>: value" /
-- "<prefix>::value" naming convention (heavily so for GitLab-synced labels:
-- S:/P:/M:/C:/…), but the bare prefix is opaque in the UI. This table maps a
-- canonical prefix (trimmed + lowercased, e.g. "s:", "effort::") to a
-- human-readable label ("Статус", "Усилие"). It is provider-neutral on purpose:
-- the GitLab modal is just one editor; user-defined tag prefixes will reuse the
-- same store (backlog). Scoped per-project to mirror tags (mig 0023).
CREATE TABLE tag_prefixes (
    project_id   uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    prefix       text NOT NULL,            -- canonical key (trimmed, lowercased)
    label        text NOT NULL,            -- friendly display name
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (project_id, prefix)
);

CREATE INDEX idx_tag_prefixes_workspace ON tag_prefixes (workspace_id);
