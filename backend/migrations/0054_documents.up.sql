-- 0054 is also taken by #2712 (comment threads) and #2713 (gitlab uploads) on
-- their own unmerged branches. Numbering has to stay contiguous per branch —
-- backend/e2e/migrate_test.go asserts highest == count(*.up.sql) — so the
-- collision is resolved by renumbering whichever branch merges later, not by
-- leaving a gap here.
--
-- Documents: block-based pages (product A of #2718). Content is one ProseMirror
-- tree in jsonb rather than a row per block: the editor hands us a tree, and a
-- block table would have to be reassembled on every read and re-sequenced on
-- every paragraph drag. Per-block locks (D4) and annotations (D5) hang off the
-- stable node id inside this JSON, not off separate rows.
CREATE TABLE documents (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    -- Nesting is a property of the page itself, as in the reference (ClickUp):
    -- any document may contain others. RESTRICT is a backstop under the handler
    -- rule that deleting a container with children is a 409, not a silent cascade.
    parent_id    uuid REFERENCES documents(id) ON DELETE RESTRICT,
    -- SET NULL, unlike notes' CASCADE: deleting a project must not take agreed
    -- specs and protocols with it. Its documents surface to the workspace level.
    project_id   uuid REFERENCES projects(id) ON DELETE SET NULL,
    author_id    uuid REFERENCES users(id) ON DELETE SET NULL,
    title        text NOT NULL,
    slug         text NOT NULL DEFAULT '',
    icon         text NOT NULL DEFAULT '',
    content      jsonb NOT NULL DEFAULT '{"type":"doc","content":[]}'::jsonb,
    position     double precision NOT NULL DEFAULT 65536,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_documents_workspace ON documents (workspace_id);
CREATE INDEX idx_documents_parent ON documents (parent_id) WHERE parent_id IS NOT NULL;
CREATE INDEX idx_documents_project ON documents (project_id) WHERE project_id IS NOT NULL;
CREATE UNIQUE INDEX idx_documents_ws_slug ON documents (workspace_id, slug);
