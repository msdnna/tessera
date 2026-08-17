-- Version history for documents (#2731, D6 of #2718): journal, snapshots,
-- comparison and rollback.
--
-- A version stores the whole content tree rather than a delta. Deltas would be
-- smaller, but every read (compare, restore, preview) would then have to replay
-- the chain from a base, and a single corrupt link would take the tail of the
-- history with it. A document is capped at maxDocContentSize, so a snapshot has
-- a known upper bound.
--
-- The snapshots are *coalesced*, and that is the part worth understanding before
-- changing anything here: autosave (D2) fires every few seconds of typing, so a
-- row per save would mean hundreds of copies of the same document per afternoon.
-- The content endpoint instead keeps writing into the version opened by the
-- current editing session (docVersionWindow in handlers/document_versions.go)
-- and starts a new one when that window is over, when someone else takes over
-- the document, or when a manual snapshot cuts the session short. The invariant
-- this maintains is: the newest version of a document always holds what the
-- document holds now.
CREATE TABLE document_versions (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id uuid NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    -- Per-document counter, 1-based. The journal is read and talked about by
    -- ordinal ("верните версию 12"), and created_at cannot serve that role: two
    -- snapshots can share a timestamp, and a rollback makes time non-monotonic
    -- with respect to content.
    revision    integer NOT NULL,
    author_id   uuid REFERENCES users(id) ON DELETE SET NULL,
    -- The title as it was, so the journal can show what the document was called
    -- at the time instead of decorating every old entry with today's name.
    title       text NOT NULL DEFAULT '',
    content     jsonb NOT NULL,
    -- Same materialised plain text as documents.preview: the journal lists
    -- versions without their content (see ListDocumentVersions) and still has to
    -- show what each one was about.
    preview     text NOT NULL DEFAULT '',
    -- Set by a human ("перед согласованием"), or by a rollback naming its
    -- source. Free text; `manual` below is what carries the meaning.
    label       text NOT NULL DEFAULT '',
    -- A milestone rather than a point in an editing session. Manual versions are
    -- never coalesced into and never pruned by retention — the whole reason to
    -- press "сохранить версию" is that this one must still be there next month.
    manual      boolean NOT NULL DEFAULT false,
    -- created_at is when the session started, updated_at when its last edit
    -- landed. Both are shown: "13:05–13:40, Пётр" is the journal entry people
    -- can actually place, and the window is measured from created_at.
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT document_versions_revision_unique UNIQUE (document_id, revision)
);

-- The journal reads a document's versions newest first, and the coalescing check
-- on every content save reads exactly the first row of that ordering.
CREATE INDEX idx_document_versions_document ON document_versions (document_id, revision DESC);
