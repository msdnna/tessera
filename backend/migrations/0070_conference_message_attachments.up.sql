-- Attachments on conference chat messages (#2864, subtask #2873).
--
-- A separate table rather than a column on conference_messages: one message may
-- carry several files, and a chat that could only ever hold one would have to be
-- migrated the first time somebody drags two screenshots in at once.
--
-- It is also deliberately *not* task_attachments with a nullable task_id. That
-- table's task_id is NOT NULL and every query on it is task-scoped; widening it
-- would make "the attachments of this task" a filtered query in a dozen places
-- for the sake of saving one table. The two shapes match on purpose, so the
-- upload and download handlers read the same way.
--
-- Files live under UPLOAD_DIR/conf/<conference id>/, behind the API — the same
-- protection task attachments get, and one level stricter than the inline-media
-- directory, whose only guard is an unguessable name. A chat in a private call
-- is exactly the wrong place to relax that.
CREATE TABLE conference_message_attachments (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id   uuid NOT NULL REFERENCES conference_messages(id) ON DELETE CASCADE,
    -- Who uploaded it stays readable after the account is gone; the file does
    -- not disappear with its uploader, the way the message does not either.
    uploader_id  uuid REFERENCES users(id) ON DELETE SET NULL,
    -- The name as the user knows it, used for the download filename only. The
    -- name on disk is a UUID, so this is never joined to a path.
    filename     text NOT NULL,
    -- Sniffed from the leading bytes at upload, not taken from the request:
    -- this value decides whether the client renders the file as a picture, and
    -- a declared type is attacker-controlled.
    content_type text NOT NULL DEFAULT '',
    size         bigint NOT NULL DEFAULT 0,
    storage_path text NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_conference_message_attachments_msg
    ON conference_message_attachments (message_id, created_at);
