-- Server-side recording state on conference_recordings (#2864, subtask #2877).
--
-- 0069 created the table for the finished artefact only: path, size, duration,
-- expiry. That is enough for a file somebody hands us, and not enough for a
-- recording produced by a separate process — between "start" and "there is an
-- mp4" LiveKit's egress worker lives for minutes, can fail, and is addressed by
-- an id we do not own. The columns below are that in-flight half.
ALTER TABLE conference_recordings
    -- The egress job id LiveKit gave us. This is the only handle for stopping a
    -- recording or asking how it is doing, so losing it means an orphan worker
    -- writing into the uploads volume with nobody left to stop it.
    ADD COLUMN egress_id  text NOT NULL DEFAULT '',
    -- Our own three-state vocabulary, not LiveKit's seven EGRESS_* values.
    -- Their set is theirs to extend (it already carries the distinctions
    -- STARTING/ACTIVE/ENDING and ABORTED/LIMIT_REACHED, which no screen of ours
    -- draws differently), and storing it raw would make every reader translate
    -- a foreign enum. The mapping lives in one place in Go instead.
    --
    -- A CHECK rather than an enum type, as on conferences.status above: adding
    -- a value to an enum needs its own migration and cannot run in a
    -- transaction on older servers.
    ADD COLUMN status     text NOT NULL DEFAULT 'active'
               CHECK (status IN ('active', 'completed', 'failed')),
    -- Why a recording failed, in the worker's words ("Chrome could not join the
    -- room", "no space left"). It is the only place that reason ever surfaces
    -- on our side: the egress container's logs are not ours to read from the
    -- API, and a failed recording with no explanation is a support ticket.
    ADD COLUMN error      text NOT NULL DEFAULT '',
    -- Who pressed record. Shown to everyone in the room next to the red dot —
    -- "N is recording" is the point of the indicator, an anonymous one would
    -- not be. SET NULL, like conferences.created_by: deleting an account must
    -- not delete the meeting's recording.
    ADD COLUMN started_by uuid REFERENCES users(id) ON DELETE SET NULL,
    -- When it actually stopped. started_at + duration_sec would be a guess:
    -- duration is what the worker managed to encode, and a recording that died
    -- at minute three of a one-hour call has the two far apart.
    ADD COLUMN ended_at   timestamptz;

-- One live recording per conference, enforced here rather than in the handler.
-- The check-then-start version has a real race: two hosts press record at the
-- same moment, both see "not recording", and we start two egress workers on one
-- room. That does not merely double the CPU — the second worker writes a second
-- file which nothing in the database points at, so it is never swept and never
-- deleted. A partial unique index makes the loser's INSERT fail instead.
CREATE UNIQUE INDEX idx_conference_recordings_one_active
    ON conference_recordings (conference_id) WHERE status = 'active';

-- The poller looks up rows by the id LiveKit reports back, and must never find
-- two. Partial because rows predating this migration have the '' default (there
-- are none in practice: nothing has ever written to this table).
CREATE UNIQUE INDEX idx_conference_recordings_egress
    ON conference_recordings (egress_id) WHERE egress_id <> '';
