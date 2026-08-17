-- Annotations and comment threads anchored to a block (#2730, D5 of #2718).
--
-- The anchor is `block_id` — the stable id BlockId stamps on every top-level
-- node of the ProseMirror tree (D2) — and not a position or a mark inside the
-- content. Two consequences worth keeping in mind before "improving" this:
--
--   * an annotation survives editing. Text positions shift on every keystroke,
--     and a mark inside content would have to be rewritten by whoever saves the
--     document — including a client that knows nothing about comments;
--   * an imported document is annotatable the moment it has block ids, which is
--     exactly what the task asks for ("в т.ч. в импортированных документах").
--
-- The price is that deleting a block leaves its thread anchored to an id that is
-- no longer in the tree. That is deliberate: silently deleting someone's
-- discussion because a paragraph was reworded is worse than showing it detached,
-- so the client lists such threads separately instead.
CREATE TABLE document_comments (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id uuid NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    -- Empty means the thread is about the document as a whole.
    block_id    text NOT NULL DEFAULT '',
    -- A reply. One level only: the root carries the anchor and the resolved
    -- state, replies carry neither (see the CHECK below).
    parent_id   uuid REFERENCES document_comments(id) ON DELETE CASCADE,
    author_id   uuid REFERENCES users(id) ON DELETE SET NULL,
    body        text NOT NULL,
    -- The text the annotation was made on, copied at creation time. The block it
    -- points at goes on being edited, and a quote that silently follows the edit
    -- would make old discussions read as if they were about the new wording.
    quote       text NOT NULL DEFAULT '',
    resolved_at timestamptz,
    resolved_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    -- Resolving is a property of the thread, so only a root can hold it. Without
    -- this a reply could be resolved on its own and the client would have two
    -- disagreeing answers to "is this thread done".
    CONSTRAINT document_comments_reply_not_resolved
        CHECK (parent_id IS NULL OR (resolved_at IS NULL AND resolved_by IS NULL))
);

-- The list endpoint fetches a whole document's threads at once and the client
-- groups them; this index is what that read walks.
CREATE INDEX idx_document_comments_document ON document_comments (document_id, created_at);
CREATE INDEX idx_document_comments_parent ON document_comments (parent_id) WHERE parent_id IS NOT NULL;
