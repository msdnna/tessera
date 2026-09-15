/**
 * What the lobby of one conference decides (#2932) — the web half of the rules
 * Android keeps in `util/ConferenceLobby.kt`.
 *
 * Pure on purpose: the lobby's controls ARE these answers, and a rule that only
 * exists inside `ConferencesView.vue` needs a router, a session and a socket
 * before it can say anything at all — which is how the one below drifted away
 * from the server without a test noticing.
 */

/** Workspace roles the backend accepts as a conference manager (`canManageConference`). */
export const CONF_MANAGER_WS_ROLES = ['owner', 'admin']

/** My seat in the call, or null when I was never invited to it. */
export function confSeat(participants, meId) {
  if (!meId) return null
  return (participants || []).find((p) => p.user_id === meId) || null
}

/**
 * May I end this call for everyone, and delete what it recorded.
 *
 * Mirrors the server's `canManageConference` (`handlers/conferences.go`): the
 * creator, a host inside the call, or an owner/admin **of the workspace**.
 *
 * The workspace role is the part this used to get wrong — it tested the global
 * `is_admin` flag, which is a different question with a different answer on both
 * sides. An owner of the workspace who did not schedule the call lost the
 * «Завершить» the server would have accepted (while still being able to kick
 * people inside the room, where the seat is computed server-side); an instance
 * admin who is a plain member of someone else's workspace was shown a button
 * that answers 403. A hidden control is an inconvenience, one that 403s is a bug
 * report — so the gate has to be the server's, not a wider one.
 *
 * @param {object|null} conference the call, or null before it is loaded
 * @param {Array<object>} participants its roster
 * @param {string} meId my user id ('' when the session has not resolved yet)
 * @param {boolean} workspaceManager owner/admin of the call's workspace
 */
export function canModerateConference(conference, participants, meId, workspaceManager) {
  if (!conference || !meId) return false
  if (conference.created_by === meId) return true
  if (workspaceManager) return true
  return confSeat(participants, meId)?.role === 'host'
}
