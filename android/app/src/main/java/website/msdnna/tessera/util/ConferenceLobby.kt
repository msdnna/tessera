package website.msdnna.tessera.util

import website.msdnna.tessera.data.model.Conference
import website.msdnna.tessera.data.model.ConferenceParticipant
import website.msdnna.tessera.data.model.Member

/*
 * What the lobby of one conference decides (#2896 §3, web `ConferencesView`'s
 * detail half).
 *
 * Everything here reads a roster and answers a question about it: am I in the
 * room, may I end the call, who is still only invited, whom is there left to
 * invite. Pure on purpose — the lobby's controls are exactly these answers, and
 * a rule tested through the rendered screen would need a socket, a session and a
 * workspace to say anything at all.
 */

/** My seat in the call, or null when I was never invited to it. */
fun confSeat(participants: List<ConferenceParticipant>, meId: String): ConferenceParticipant? =
    participants.firstOrNull { it.userId == meId && meId.isNotBlank() }

/**
 * I am in the room right now.
 *
 * A seat is not presence: an invitation is a row with no `joined_at`, and a
 * yesterday's session leaves a row with both stamps set. Reading `joined_at`
 * alone would show «Выйти» to somebody who has not been in the call since
 * Tuesday — and hide the join button that is the only way back in.
 */
fun isInConfRoom(participants: List<ConferenceParticipant>, meId: String): Boolean =
    confSeat(participants, meId)?.isPresent == true

/** Workspace roles the backend accepts as a conference manager (`canManageConference`). */
val CONF_MANAGER_WS_ROLES = setOf("owner", "admin")

/**
 * May I end this call for everyone.
 *
 * Mirrors the server's `canManageConference`: the creator, a host inside the
 * call, or an owner/admin of the workspace. [workspaceRole] is my role in the
 * workspace, blank when the roster could not be read — the button then hides for
 * a workspace admin who is neither creator nor host, which is the safe way round:
 * a hidden control is an inconvenience, one that 403s is a bug report.
 *
 * The web client tests the *global* admin flag here instead. That is a wider
 * gate than the server's, so its «Завершить» can answer 403 to a global admin
 * who is not in this workspace; we ask about the workspace role we already have.
 */
fun canModerateConference(
    conference: Conference,
    participants: List<ConferenceParticipant>,
    meId: String,
    workspaceRole: String,
): Boolean {
    if (meId.isBlank()) return false
    if (conference.createdBy == meId) return true
    if (workspaceRole in CONF_MANAGER_WS_ROLES) return true
    return confSeat(participants, meId)?.isHost == true
}

/**
 * Whether anyone can still be invited. An ended call is not a dead end — it is
 * a reusable room (#2879) and joining it starts it again — but there is nothing
 * to call somebody into until it does.
 */
fun canInviteToConference(conference: Conference): Boolean = !conference.isEnded

/**
 * Who is in the room, hosts first and then by name.
 *
 * The order is the panel's own, not the server's: participants arrive in
 * invitation order, which puts whoever was invited first at the top forever,
 * even after they leave and rejoin.
 */
fun confRoster(participants: List<ConferenceParticipant>): List<ConferenceParticipant> =
    participants.filter { it.isPresent }
        .sortedWith(compareByDescending<ConferenceParticipant> { it.isHost }.thenBy { it.label().lowercase() })

/**
 * Invited, not in the room, and not somebody who came and left.
 *
 * The third case is the point: a row with both stamps is a person who attended
 * an earlier session, and listing them under «Приглашены» would say an
 * invitation is outstanding when nobody is being called.
 */
fun confInvited(participants: List<ConferenceParticipant>): List<ConferenceParticipant> =
    participants.filter { it.joinedAt == null && it.leftAt == null }
        .sortedBy { it.label().lowercase() }

/**
 * Workspace members there is any point in inviting.
 *
 * A seat counts as held while `left_at` is null — that covers both the people in
 * the room and the ones whose invitation is still ringing (#2875), so we don't
 * offer to call somebody who is already being called. Anyone who joined and left
 * is offered again: re-inviting them is a fresh call, which is the backend's own
 * rule for the notification.
 */
fun confInvitable(
    members: List<Member>,
    participants: List<ConferenceParticipant>,
): List<Member> {
    val held = participants.filter { it.leftAt == null }.map { it.userId }.toSet()
    return members.filter { it.userId !in held }.sortedBy { it.name.ifBlank { it.email }.lowercase() }
}

/**
 * What to print for a participant. The joined name is absent on the raw rows the
 * join/leave responses carry, so the id is the last resort rather than a blank
 * line in the roster.
 */
fun ConferenceParticipant.label(): String =
    userName?.takeIf { it.isNotBlank() }
        ?: userEmail?.takeIf { it.isNotBlank() }
        ?: userId
