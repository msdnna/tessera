-- Members v2 (#3063): ownership transfer, activity, access requests, groups.

-- ── ownership transfer (#3072) ─────────────────────────────────

-- LockWorkspaceOwner reads the owner under a row lock, so two transfers racing
-- on one workspace serialise and the second one sees the first one's result.
-- name: LockWorkspaceOwner :one
SELECT owner_id FROM workspaces WHERE id = $1 FOR UPDATE;

-- name: SetWorkspaceOwner :exec
UPDATE workspaces SET owner_id = $2, updated_at = now() WHERE id = $1;

-- CountWorkspaceOwners is the invariant the transfer tests hold: exactly one.
-- name: CountWorkspaceOwners :one
SELECT count(*) FROM memberships WHERE workspace_id = $1 AND role = 'owner';

-- ── activity (#3073) ───────────────────────────────────────────

-- TouchUserSeen stamps "was in the app". Throttled in the WHERE: a refresh
-- storm or a reconnecting socket writes at most once a minute.
-- name: TouchUserSeen :exec
UPDATE users SET last_seen_at = now()
WHERE id = $1 AND (last_seen_at IS NULL OR last_seen_at < now() - interval '1 minute');

-- TouchMembershipActive stamps "did something in this workspace", with the same
-- once-a-minute throttle.
-- name: TouchMembershipActive :exec
UPDATE memberships SET last_active_at = now()
WHERE workspace_id = $1 AND user_id = $2
  AND (last_active_at IS NULL OR last_active_at < now() - interval '1 minute');

-- ── access requests (#3077) ────────────────────────────────────

-- name: CreateAccessRequest :one
INSERT INTO workspace_access_requests (workspace_id, user_id, board_id, message)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetPendingAccessRequest :one
SELECT * FROM workspace_access_requests
WHERE workspace_id = $1 AND user_id = $2 AND status = 'pending';

-- LastDeclinedAccessRequest is the cooldown check: a declined request blocks a
-- new one for a while, so a refusal can't be answered with a request storm.
-- name: LastDeclinedAccessRequestAt :one
SELECT decided_at FROM workspace_access_requests
WHERE workspace_id = $1 AND user_id = $2 AND status = 'declined'
ORDER BY decided_at DESC NULLS LAST
LIMIT 1;

-- name: GetAccessRequest :one
SELECT * FROM workspace_access_requests WHERE id = $1;

-- ListPendingAccessRequests is the managers' queue, with who is asking and
-- which board they came from (a soft-deleted board is not named).
-- name: ListPendingAccessRequests :many
SELECT r.id, r.workspace_id, r.user_id, r.message, r.created_at,
       u.name AS user_name, u.email AS user_email,
       r.board_id,
       COALESCE(b.name, '')::text AS board_name
FROM workspace_access_requests r
JOIN users u ON u.id = r.user_id
LEFT JOIN boards b ON b.id = r.board_id AND b.deleted_at IS NULL
WHERE r.workspace_id = $1 AND r.status = 'pending'
ORDER BY r.created_at;

-- DecideAccessRequest closes a pending request. The status guard makes a double
-- click (or two admins at once) a no-op for the second: it gets no row back.
-- name: DecideAccessRequest :one
UPDATE workspace_access_requests
SET status = $2, decided_by = $3, decided_at = now()
WHERE id = $1 AND status = 'pending'
RETURNING *;

-- name: ListWorkspaceManagerIDs :many
SELECT user_id FROM memberships
WHERE workspace_id = $1 AND role IN ('owner', 'admin');

-- ── member groups (#3074) ──────────────────────────────────────

-- name: CreateMemberGroup :one
INSERT INTO member_groups (workspace_id, name, color)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetMemberGroup :one
SELECT * FROM member_groups WHERE id = $1;

-- name: UpdateMemberGroup :one
UPDATE member_groups SET name = $2, color = $3, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteMemberGroup :exec
DELETE FROM member_groups WHERE id = $1;

-- ListMemberGroups lists a workspace's groups with their members. A member who
-- has left the workspace drops out here (the join on memberships) even before
-- RemoveMember's cleanup — the group can never name an outsider.
-- name: ListMemberGroups :many
SELECT g.id, g.workspace_id, g.name, g.color, g.created_at, g.updated_at,
       COALESCE(array_agg(gm.user_id ORDER BY gm.user_id)
                FILTER (WHERE m.user_id IS NOT NULL), '{}')::uuid[] AS member_ids
FROM member_groups g
LEFT JOIN member_group_members gm ON gm.group_id = g.id
LEFT JOIN memberships m ON m.workspace_id = g.workspace_id AND m.user_id = gm.user_id
WHERE g.workspace_id = $1
GROUP BY g.id
ORDER BY lower(g.name);

-- name: ClearMemberGroupMembers :exec
DELETE FROM member_group_members WHERE group_id = $1;

-- AddMemberGroupMembers inserts only the ids that are members of the group's
-- workspace — the guard is in the write, not only in the handler.
-- name: AddMemberGroupMembers :exec
INSERT INTO member_group_members (group_id, user_id)
SELECT g.id, m.user_id
FROM member_groups g
JOIN memberships m ON m.workspace_id = g.workspace_id
WHERE g.id = $1 AND m.user_id = ANY(sqlc.arg(user_ids)::uuid[])
ON CONFLICT DO NOTHING;

-- CountWorkspaceMembersIn counts how many of ids are members of the workspace,
-- so the handler can refuse a list naming an outsider instead of dropping it
-- silently.
-- name: CountWorkspaceMembersIn :one
SELECT count(*) FROM memberships
WHERE workspace_id = $1 AND user_id = ANY(sqlc.arg(user_ids)::uuid[]);

-- DropUserFromWorkspaceGroups runs when a member leaves or is removed.
-- name: DropUserFromWorkspaceGroups :exec
DELETE FROM member_group_members gm
USING member_groups g
WHERE gm.group_id = g.id AND g.workspace_id = $1 AND gm.user_id = $2;
