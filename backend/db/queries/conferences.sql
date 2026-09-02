-- name: CreateConference :one
INSERT INTO conferences (workspace_id, task_id, created_by, title, description, scheduled_at, recording_ttl_days)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- ListConferences returns the workspace's conferences with the two numbers the
-- list screen shows next to every row: how many people are invited and how many
-- are in the room right now. Counting them here rather than per-row keeps
-- opening the section one round trip instead of one plus two per conference.
--
-- Order puts live calls first — you join the meeting that is happening, not the
-- one planned for Thursday — then the nearest scheduled, then the archive.
-- name: ListConferences :many
SELECT c.*,
       u.name AS created_by_name,
       (SELECT count(*) FROM conference_participants p WHERE p.conference_id = c.id) AS participant_count,
       (SELECT count(*) FROM conference_participants p
         WHERE p.conference_id = c.id AND p.joined_at IS NOT NULL AND p.left_at IS NULL) AS active_count
FROM conferences c
LEFT JOIN users u ON u.id = c.created_by
WHERE c.workspace_id = $1
  AND (sqlc.narg('status')::text IS NULL OR c.status = sqlc.narg('status')::text)
ORDER BY (c.status = 'live') DESC,
         c.scheduled_at ASC NULLS LAST,
         c.created_at DESC;

-- name: GetConference :one
SELECT * FROM conferences WHERE id = $1;

-- name: ListConferencesByTask :many
SELECT * FROM conferences WHERE task_id = $1 ORDER BY created_at DESC;

-- UpdateConference is a full replace of the editable fields (the same shape the
-- other Update handlers in this codebase use); status transitions have their own
-- statements below so a PATCH of the title can never take a call off the air.
-- name: UpdateConference :one
UPDATE conferences
SET title = $2, description = $3, scheduled_at = $4, task_id = $5, recording_ttl_days = $6, updated_at = now()
WHERE id = $1
RETURNING *;

-- StartConference is idempotent on purpose: everyone's client calls join at
-- once when a meeting begins, and the first caller is whoever's network was
-- fastest. The WHERE keeps started_at at the true first join instead of letting
-- the last racer overwrite it, and refuses to resurrect a call that has ended.
-- name: StartConference :one
UPDATE conferences
SET status = 'live', started_at = COALESCE(started_at, now()), updated_at = now()
WHERE id = $1 AND status = 'scheduled'
RETURNING *;

-- name: EndConference :one
UPDATE conferences
SET status = 'ended', ended_at = COALESCE(ended_at, now()), updated_at = now()
WHERE id = $1 AND status <> 'ended'
RETURNING *;

-- name: DeleteConference :exec
DELETE FROM conferences WHERE id = $1;

-- name: WorkspaceIDForConference :one
SELECT workspace_id FROM conferences WHERE id = $1;

-- InviteConferenceParticipant adds an invitee, or re-invites someone who had
-- left. ON CONFLICT rather than an existence check: invitations are sent from
-- several places at once (the dialog, the notification, an /invite call) and
-- the loser of that race must not get a unique-violation 500. The role is left
-- alone on conflict — re-inviting the host must not demote them to member.
-- name: InviteConferenceParticipant :one
INSERT INTO conference_participants (conference_id, user_id, role)
VALUES ($1, $2, $3)
ON CONFLICT (conference_id, user_id) DO UPDATE
SET invited_at = now(), left_at = NULL
RETURNING *;

-- name: GetConferenceParticipant :one
SELECT * FROM conference_participants WHERE conference_id = $1 AND user_id = $2;

-- name: ListConferenceParticipants :many
SELECT p.*, u.name AS user_name, u.email AS user_email
FROM conference_participants p
JOIN users u ON u.id = p.user_id
WHERE p.conference_id = $1
ORDER BY p.role, u.name;

-- JoinConferenceParticipant records attendance. It doubles as the invite for an
-- open conference: a member of the workspace may walk into a call they were not
-- explicitly invited to, and the row is created on the spot rather than the
-- join being refused.
-- name: JoinConferenceParticipant :one
INSERT INTO conference_participants (conference_id, user_id, role, joined_at)
VALUES ($1, $2, $3, now())
ON CONFLICT (conference_id, user_id) DO UPDATE
SET joined_at = now(), left_at = NULL
RETURNING *;

-- name: LeaveConferenceParticipant :one
UPDATE conference_participants
SET left_at = now()
WHERE conference_id = $1 AND user_id = $2
RETURNING *;

-- name: SetConferenceParticipantMuted :one
UPDATE conference_participants
SET force_muted = $3
WHERE conference_id = $1 AND user_id = $2
RETURNING *;

-- name: RemoveConferenceParticipant :exec
DELETE FROM conference_participants WHERE conference_id = $1 AND user_id = $2;

-- name: CountActiveConferenceParticipants :one
SELECT count(*) FROM conference_participants
WHERE conference_id = $1 AND joined_at IS NOT NULL AND left_at IS NULL;
