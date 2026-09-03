-- Server-side recordings of a conference (#2864, subtask #2877).
--
-- A row is created the moment LiveKit accepts the egress job, not when the file
-- appears: until then the recording exists only inside the egress worker, and a
-- row is what lets us stop it, show the red dot to everyone in the room, and
-- notice that it died. Size, duration and expiry are filled in later by the
-- poller — see FinishConferenceRecording.

-- CreateConferenceRecording opens the in-flight row. The unique partial index on
-- (conference_id) WHERE status = 'active' does the "one at a time" check for us:
-- two hosts pressing record simultaneously means the loser gets a unique
-- violation here rather than a second egress worker nobody is tracking.
-- name: CreateConferenceRecording :one
INSERT INTO conference_recordings (conference_id, egress_id, file_path, file_name, started_by)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetConferenceRecording :one
SELECT * FROM conference_recordings WHERE id = $1;

-- ListConferenceRecordings backs the list on the conference card. It returns the
-- failed rows too — a recording somebody started and never got is exactly what
-- they will come asking about, and hiding the row would leave the answer only in
-- the container logs.
-- name: ListConferenceRecordings :many
SELECT r.*, u.name AS started_by_name
FROM conference_recordings r
LEFT JOIN users u ON u.id = r.started_by
WHERE r.conference_id = $1
ORDER BY r.started_at DESC;

-- ActiveConferenceRecording is the row behind the red "recording" indicator, and
-- the same query that refuses a second start. Joining the name here keeps the
-- room-state snapshot one statement: the indicator says who is recording, and an
-- anonymous one would defeat its purpose.
-- name: ActiveConferenceRecording :one
SELECT r.*, u.name AS started_by_name
FROM conference_recordings r
LEFT JOIN users u ON u.id = r.started_by
WHERE r.conference_id = $1 AND r.status = 'active';

-- ListActiveRecordings feeds the poller: every row still believed to be running,
-- with the TTL it will need at the finish line. The TTL is carried along rather
-- than fetched per row because the poller would otherwise do one GetConference
-- per recording every minute to read a single integer.
-- name: ListActiveRecordings :many
SELECT r.*, c.recording_ttl_days
FROM conference_recordings r
JOIN conferences c ON c.id = r.conference_id
WHERE r.status = 'active'
ORDER BY r.started_at;

-- FinishConferenceRecording closes a row: one statement for success and for
-- failure, because the difference between them is the status and the error text
-- and nothing else about the row's lifecycle.
--
-- WHERE status = 'active' is the guard that matters. The poller and an explicit
-- stop can both reach a recording in the same second; without it the slower one
-- would overwrite the finished row — resetting ended_at, and pushing expires_at
-- a fresh TTL into the future so the sweeper never picks the file up.
--
-- expires_at follows conferences.recording_ttl_days, where 0 means "keep
-- indefinitely" — NULL here, which is also what the sweeper's index skips.
-- name: FinishConferenceRecording :one
UPDATE conference_recordings
SET status       = sqlc.arg('status'),
    size_bytes   = sqlc.arg('size_bytes'),
    duration_sec = sqlc.arg('duration_sec'),
    error        = sqlc.arg('error'),
    ended_at     = now(),
    expires_at   = CASE WHEN sqlc.arg('ttl_days')::int > 0
                        THEN now() + make_interval(days => sqlc.arg('ttl_days')::int)
                        ELSE NULL END
WHERE id = sqlc.arg('id') AND status = 'active'
RETURNING *;

-- ListExpiredRecordings drives the TTL sweeper. Limited on purpose: a tick that
-- has to delete a thousand files should take a thousand files' worth of ticks
-- rather than hold the worker for minutes on its first run after a TTL change.
--
-- Failed rows are swept as well — a recording that died mid-way still left a
-- partial file in the uploads volume, and it is the one nobody will ever ask to
-- keep.
-- name: ListExpiredRecordings :many
SELECT * FROM conference_recordings
WHERE expires_at IS NOT NULL AND expires_at < now()
ORDER BY expires_at
LIMIT $1;

-- name: DeleteConferenceRecording :exec
DELETE FROM conference_recordings WHERE id = $1;

-- ConferenceForRecording resolves the conference a recording belongs to, so the
-- download route can authorize by workspace membership in one round trip —
-- the same shape as ConferenceForMessageAttachment next door.
-- name: ConferenceForRecording :one
SELECT c.*
FROM conference_recordings r
JOIN conferences c ON c.id = r.conference_id
WHERE r.id = $1;

-- ListConferenceRecordingPaths lists the files to unlink when a whole conference
-- goes away: the rows themselves leave by ON DELETE CASCADE, which is precisely
-- why the paths have to be read *before* the delete.
-- name: ListConferenceRecordingPaths :many
SELECT file_path FROM conference_recordings WHERE conference_id = $1;
