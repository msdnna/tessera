-- Links between documents and tasks (#2732, D7 of #2718) — point 2 of the parent
-- task, first half.
--
-- The link optionally carries a block_id: the same stable anchor the annotations
-- hang on (0056), stamped on every top-level node by BlockId (D2). That is what
-- makes "задача ↔ конкретное место в документе" one relation with an anchor
-- rather than a second kind of link — and it is only possible because the
-- document is a block tree. Against an opaque office blob (the product B path,
-- D10) there is nothing addressable to point at, which is exactly the trade the
-- parent task settled.
--
-- Deleting a block leaves its links anchored to an id no longer in the tree.
-- Deliberate, and the same call as document_comments: dropping someone's link
-- because a paragraph was reworded loses more than showing it detached does. The
-- client lists such links separately.
CREATE TABLE document_task_links (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id uuid NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    task_id     uuid NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    -- Empty means the link is to the document as a whole.
    block_id    text NOT NULL DEFAULT '',
    -- The block's text when the link was made, for the reason
    -- document_comments.quote exists: the paragraph goes on being edited, and a
    -- label that silently followed the edit would misstate what was linked.
    quote       text NOT NULL DEFAULT '',
    created_by  uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    -- block_id is part of the key, not excluded from it: a task may legitimately
    -- be linked both to a spec as a whole and to the one clause it implements.
    CONSTRAINT document_task_links_unique UNIQUE (document_id, task_id, block_id)
);

-- The document panel reads a document's links; the task modal reads a task's.
-- Both directions are first-class, so both get an index.
CREATE INDEX idx_document_task_links_document ON document_task_links (document_id, created_at);
CREATE INDEX idx_document_task_links_task ON document_task_links (task_id);
