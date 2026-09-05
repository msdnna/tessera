package website.msdnna.tessera.data.realtime

/**
 * What a realtime event means for the conference screens (#2896 §1).
 *
 * The server broadcasts eleven `conference.*` types and the client reacts to
 * them in four ways, not eleven. Naming the four here — instead of chaining
 * `startsWith` inside a view model, as the web client does — keeps the reaction
 * testable without a socket, and keeps §2 and §5 from drifting apart on which
 * event repaints what.
 */
enum class ConfEvent {
    /** The list changed: a call was created, started, ended, updated or deleted. */
    LIST,

    /** Only the roster moved — repaint the participants panel, not the page. */
    PARTICIPANTS,

    /**
     * A recording started, finished or was deleted. The conference row itself is
     * untouched, so reloading it would repaint the whole screen every time an
     * egress worker reports in.
     */
    RECORDING,

    /** Not ours — a task, note or document event on the same socket. */
    OTHER,
}

/**
 * Classifies [type] for the workspace [scope] we are showing.
 *
 * [current] is the open workspace id; an event from another workspace is
 * [ConfEvent.OTHER] rather than a repaint, because the socket carries every
 * workspace the user belongs to.
 *
 * Order matters: `conference.recording.*` is checked before the participant test
 * so that a future `conference.recording.participant…` cannot be read as a
 * roster change, and the participant test before the catch-all so a join does
 * not reload the whole list.
 */
fun classifyConfEvent(type: String, scope: String, current: String): ConfEvent = when {
    scope != current || !type.startsWith(CONF_PREFIX) -> ConfEvent.OTHER
    type.startsWith(RECORDING_PREFIX) -> ConfEvent.RECORDING
    type.contains(PARTICIPANT) -> ConfEvent.PARTICIPANTS
    else -> ConfEvent.LIST
}

/**
 * True when [type] means the call we are in is over for everyone.
 *
 * Both an explicit end and the pause that follows the last person leaving arrive
 * as `conference.ended` (#2879), and the room screen treats them the same way:
 * disconnect and go back. It is a separate question from [classifyConfEvent],
 * which answers what to repaint — this one answers whether to stay.
 */
fun isConfEnded(type: String, conferenceId: String, eventConferenceId: String?): Boolean =
    type == "conference.ended" && eventConferenceId == conferenceId

private const val CONF_PREFIX = "conference"
private const val RECORDING_PREFIX = "conference.recording"
private const val PARTICIPANT = "participant"
