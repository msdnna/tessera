-- Conferences (#2864, subtask #2868): scheduled rooms for audio/video calls.
--
-- Media itself never touches these tables — it goes through the LiveKit SFU
-- (#2866/#2867). What lives here is only what has to survive a restart: which
-- conferences exist, who was invited, what was said in the chat, what was
-- recorded. Live room state (who is speaking, who is sharing a screen, whose
-- turn is next) stays in memory, the way internal/docroom keeps presence — it
-- is meaningless once the process is gone, and writing it would mean a row per
-- speaking turn.
--
-- Access rights are not modelled here either: membership in the workspace is
-- the permission, exactly as for boards and documents. `role` below is the
-- role *inside the call* (who may kick and force-mute), not a second copy of
-- the workspace role.
CREATE TABLE conferences (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    -- A conference held to discuss a task keeps a link to it, so the protocol
    -- can be filed back into the task later. SET NULL, not CASCADE: deleting a
    -- task must not erase the recording of the meeting about it.
    task_id      uuid REFERENCES tasks(id) ON DELETE SET NULL,
    -- The author stays addressable after an account is removed only through
    -- the messages and participant rows; the conference itself survives.
    created_by   uuid REFERENCES users(id) ON DELETE SET NULL,
    title        text NOT NULL,
    description  text NOT NULL DEFAULT '',
    -- Planned start. NULL means "ad-hoc, starts when someone joins".
    scheduled_at timestamptz,
    started_at   timestamptz,
    ended_at     timestamptz,
    -- scheduled -> live -> ended. Kept as text with a CHECK rather than an enum
    -- type: adding a state to an enum needs its own migration and cannot run
    -- inside a transaction on older servers, and this set will grow.
    status       text NOT NULL DEFAULT 'scheduled'
                 CHECK (status IN ('scheduled', 'live', 'ended')),
    -- How long recordings of this conference are kept; 0 = keep indefinitely.
    -- The sweeper that acts on it arrives with the recording subtask.
    recording_ttl_days integer NOT NULL DEFAULT 30 CHECK (recording_ttl_days >= 0),
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_conferences_workspace ON conferences (workspace_id, scheduled_at DESC NULLS LAST);
CREATE INDEX idx_conferences_task ON conferences (task_id) WHERE task_id IS NOT NULL;

-- Invitations and attendance in one table: an invited user is a row with
-- joined_at NULL, and joining stamps it. Two tables would need a join for the
-- one question the participants panel always asks — "who is expected and who
-- is actually here".
CREATE TABLE conference_participants (
    conference_id uuid NOT NULL REFERENCES conferences(id) ON DELETE CASCADE,
    user_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    -- host may kick and force-mute; member may not. The creator is host.
    role          text NOT NULL DEFAULT 'member' CHECK (role IN ('host', 'member')),
    invited_at    timestamptz NOT NULL DEFAULT now(),
    joined_at     timestamptz,
    left_at       timestamptz,
    -- Muted by an admin for everyone. Unlike a local mute (which is a client-side
    -- gain of zero) this one has to be enforced server-side, so it is persisted:
    -- otherwise a reconnect would hand the muted participant their microphone back.
    force_muted   boolean NOT NULL DEFAULT false,
    PRIMARY KEY (conference_id, user_id)
);
CREATE INDEX idx_conference_participants_user ON conference_participants (user_id);

-- In-call chat. Attachments hang off these rows via the existing attachment
-- machinery in the subtask that builds the chat; the body alone is enough for
-- the CRUD layer.
CREATE TABLE conference_messages (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    conference_id uuid NOT NULL REFERENCES conferences(id) ON DELETE CASCADE,
    user_id       uuid REFERENCES users(id) ON DELETE SET NULL,
    body          text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_conference_messages_conf ON conference_messages (conference_id, created_at);

CREATE TABLE conference_recordings (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    conference_id uuid NOT NULL REFERENCES conferences(id) ON DELETE CASCADE,
    -- Path under UPLOAD_DIR, as for attachments.
    file_path     text NOT NULL,
    file_name     text NOT NULL DEFAULT '',
    size_bytes    bigint NOT NULL DEFAULT 0,
    duration_sec  integer NOT NULL DEFAULT 0,
    started_at    timestamptz NOT NULL DEFAULT now(),
    -- NULL = never expires. The sweeper deletes the file and then the row.
    expires_at    timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_conference_recordings_conf ON conference_recordings (conference_id, started_at DESC);
CREATE INDEX idx_conference_recordings_expiry ON conference_recordings (expires_at) WHERE expires_at IS NOT NULL;
