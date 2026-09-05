package website.msdnna.tessera.util

import website.msdnna.tessera.data.conference.ConfDenied
import website.msdnna.tessera.data.conference.ConfPerson
import website.msdnna.tessera.data.conference.ConferenceRoomSocket

/*
 * The rules the participants panel runs on (#2896 §6, web `ParticipantsPanel`).
 *
 * The panel deliberately mixes two sources that look alike and are not: the
 * roster is the server's truth about presence, roles, hands and force-mutes,
 * while the volume controls come from the media transport and never leave this
 * phone. «I cannot hear them» and «nobody may hear them» must not be two
 * controls that look the same, so the rules that decide each of them live apart
 * here too.
 */

/** What the row says about a person's microphone, in one badge. */
enum class ConfMicBadge {
    /** A moderator took it away — not their own choice, and the row says which. */
    FORCE_MUTED,

    /** Their own decision, or a device they never turned on. */
    OFF,

    ON,
}

fun confMicBadge(person: ConfPerson): ConfMicBadge = when {
    person.forceMuted -> ConfMicBadge.FORCE_MUTED
    !person.mic -> ConfMicBadge.OFF
    else -> ConfMicBadge.ON
}

/**
 * Orders the roster: raised hands first, oldest first, then the room's own
 * order.
 *
 * A raised hand is the one thing in this list waiting on somebody reading it, so
 * it must not need scrolling to notice. Within the hands the order is the order
 * they went up — anything else quietly reorders a queue people are relying on to
 * be fair.
 *
 * The room's own order (join time) is preserved below them by a stable sort:
 * re-sorting the rest on every snapshot would shuffle faces under a finger
 * reaching for a kick button.
 */
fun confPanelOrder(people: List<ConfPerson>): List<ConfPerson> =
    people.sortedWith(
        compareBy(
            { it.handAt == null },
            { it.handAt.orEmpty() },
        ),
    )

/** How many hands are up — the badge on the closed panel counts these. */
fun confHandCount(people: List<ConfPerson>): Int = people.count { it.handAt != null }

/**
 * The moderation controls available on one row.
 *
 * Two flags rather than one because the server draws the same distinction: a
 * host may be force-muted like anyone else, but kicking one is refused outright
 * — they are the call's owner. Showing the button anyway (as the web does) puts
 * a control on screen whose every press answers «cannot kick a host», and a
 * control that only ever fails teaches that controls do not work.
 */
data class ConfRowActions(val forceMute: Boolean, val kick: Boolean)

fun confRowActions(person: ConfPerson, meId: String, canModerate: Boolean): ConfRowActions {
    // Never on our own row: the server refuses «cannot kick yourself», and a
    // force-mute we could lift ourselves is not a moderation decision at all.
    if (!canModerate || person.userId == meId || meId.isBlank()) {
        return ConfRowActions(forceMute = false, kick = false)
    }
    return ConfRowActions(forceMute = true, kick = !person.host)
}

/**
 * Whether this row gets the local volume controls.
 *
 * Only for somebody we are actually receiving audio from. A person in the room
 * socket with no media descriptor is joining, or in the call with no devices:
 * there is no audio to turn down, and a slider that moves nothing is a worse
 * answer than no slider. Never for ourselves — the local track is not played
 * back to us.
 */
fun confHasLocalAudio(personId: String, meId: String, mediaIdentities: Collection<String>): Boolean =
    personId != meId && personId.isNotBlank() && mediaIdentities.contains(personId)

/**
 * Local playback volume for one person, as a multiplier.
 *
 * Above 1.0 on purpose: the common complaint on a phone is a quiet colleague on
 * a bad microphone, and the useful half of this control is the half above
 * normal.
 */
const val CONF_VOLUME_DEFAULT = 1.0f
const val CONF_VOLUME_MAX = 2.0f

/** Clamps a slider value to something the SDK will accept. */
fun confClampVolume(value: Float): Float = value.coerceIn(0f, CONF_VOLUME_MAX)

/** Which refusal message to show, keyed by the command the server named. */
enum class ConfDeniedKind {
    NONE,
    KICK,
    MUTE,
    OTHER,
}

/**
 * Reads a refusal.
 *
 * Keyed on the action rather than on the server's sentence: the reason is
 * English prose meant for a developer, and putting it in front of a user would
 * be showing them a log line.
 */
fun confDeniedKind(denied: ConfDenied?): ConfDeniedKind = when (denied?.action) {
    null -> ConfDeniedKind.NONE
    ConferenceRoomSocket.TYPE_KICK -> ConfDeniedKind.KICK
    ConferenceRoomSocket.TYPE_MUTE -> ConfDeniedKind.MUTE
    else -> ConfDeniedKind.OTHER
}
