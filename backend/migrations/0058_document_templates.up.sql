-- Document templates (D9 of #2718, point 8 of the task): a saved starting point
-- for new documents.
--
-- Deliberately a table of its own rather than a flag on `documents`. A template
-- is not a document that happens to be marked: it must not appear in the tree,
-- must not be nested, carries no comments, no versions and no block locks, and
-- it outlives the document it was made from — which is the whole point of
-- saving one. A boolean on `documents` would mean every list query, every tree
-- walk and every realtime payload grows a "and not a template" clause, and the
-- first one to forget it leaks templates into the section.
--
-- There is no FK back to the source document for the same reason: deleting the
-- spec you templated must not take the template with it.
CREATE TABLE document_templates (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    author_id    uuid REFERENCES users(id) ON DELETE SET NULL,
    title        text NOT NULL,
    description  text NOT NULL DEFAULT '',
    icon         text NOT NULL DEFAULT '',
    content      jsonb NOT NULL DEFAULT '{"type":"doc","content":[]}'::jsonb,
    -- Same plain-text line the document tiles show, derived server-side by
    -- validateDocContent so the gallery never has to fetch bodies to render.
    preview      text NOT NULL DEFAULT '',
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_document_templates_workspace ON document_templates (workspace_id);
