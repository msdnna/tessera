package website.msdnna.tessera.util

/*
 * The rules the screen-share stage runs on (#2896 §8, web `ConferenceRoom.vue`).
 *
 * Two connections disagree by design here, exactly as on the web: our own room
 * socket owns the *stage* — who may present, who waits, and a host preempting a
 * member — while the SFU owns the pixels. The arbiter is the server, never the
 * client: a phone that started capturing because it asked is how two people end
 * up presenting at once.
 *
 * What Android changes is the order of the two questions. A browser can call
 * `getDisplayMedia()` and wait in the queue with the picker already answered;
 * MediaProjection cannot. Its consent is a full-screen warning that this app is
 * about to see everything on the display, it is spent on a single capture
 * (Android 14 refuses to reuse the result), and a user who declines it once does
 * not get asked again by accident. So the consent is asked *after* the stage is
 * ours, never before — which is why [ConfShare.MY_TURN] is a state here and only
 * a notice on the web.
 */

/** Where we are in the business of showing our screen. */
enum class ConfShare {
    /** Not asking and not showing. */
    OFF,

    /** We asked, somebody else holds the stage — we are in the line. */
    QUEUED,

    /** The stage is ours and nothing is being captured yet: consent is pending. */
    MY_TURN,

    /** Our pixels are going out. */
    SHARING,
}

/**
 * Reads the share state off the three facts that make it up.
 *
 * [presenting] is the server's answer and [capturing] is this phone's, and the
 * whole point of keeping them apart is that they disagree for a second at every
 * transition. Collapsing them would either offer «остановить показ» for a
 * capture that never started, or leave the stage parked while the consent dialog
 * is on screen.
 */
fun confShareState(want: Boolean, presenting: Boolean, capturing: Boolean): ConfShare = when {
    capturing && presenting -> ConfShare.SHARING

    // Capturing without the stage is the moment a host preempted us; the screen
    // stops the capture on it rather than painting a share nobody receives.
    capturing -> ConfShare.OFF

    !want -> ConfShare.OFF

    presenting -> ConfShare.MY_TURN

    else -> ConfShare.QUEUED
}

/** What pressing the share button does next. */
enum class ConfSharePress {
    /** Get in the line (`screen.request`). */
    REQUEST,

    /** The stage is ours — ask Android for the display. */
    CAPTURE,

    /** «Never mind»: drop out of the queue. */
    CANCEL,

    /** Stop the capture and hand the stage on. */
    STOP,
}

fun confSharePress(state: ConfShare): ConfSharePress = when (state) {
    ConfShare.OFF -> ConfSharePress.REQUEST
    ConfShare.QUEUED -> ConfSharePress.CANCEL
    ConfShare.MY_TURN -> ConfSharePress.CAPTURE
    ConfShare.SHARING -> ConfSharePress.STOP
}

/**
 * Whether the consent dialog may follow a request without a second press.
 *
 * Pressing «Показать экран» on a free stage should cost one press, as it does on
 * the web. Pressing it into a queue must not: the dialog would come up now and
 * the turn may arrive in ten minutes, by which time the consent is stale and the
 * user has forgotten agreeing to it. So the auto-follow is armed only when the
 * stage was visibly free at the moment of the press — anything else waits for
 * «Начать показ».
 */
fun confShareAutoCapture(stageTaken: Boolean): Boolean = !stageTaken

/**
 * Our place in the line, 1-based; 0 when we are not waiting.
 *
 * Keyed by connection and not by user: the same person may have the call open on
 * a phone and a laptop, and the laptop's place in the queue is not the phone's.
 */
fun confQueuePosition(queue: List<String>, connId: String): Int {
    if (connId.isBlank()) return 0
    val at = queue.indexOf(connId)
    return if (at < 0) 0 else at + 1
}

/** The one line the room says about the stage, above the tiles. */
enum class ConfStageNotice {
    NONE,

    /** Our pixels are going out right now. */
    YOU_PRESENT,

    /** The stage is ours and we have not started capturing. */
    STAGE_YOURS,

    /** Somebody else is on the stage; their name goes with it. */
    OTHER_PRESENTS,

    /** Nobody presents, and we are only queued behind a stage that just freed. */
    WAITING,
}

/**
 * Picks the notice, most personal first.
 *
 * The name is carried separately because only [ConfStageNotice.OTHER_PRESENTS]
 * has one, and a notice that formatted a blank name would read «Экран показывает».
 */
data class ConfStageLine(
    val notice: ConfStageNotice = ConfStageNotice.NONE,
    val name: String = "",
    /** Our 1-based place in the queue, or 0. Shown alongside any notice. */
    val position: Int = 0,
    /** How many are waiting behind the stage, ourselves included. */
    val waiting: Int = 0,
)

/**
 * What to say about the stage.
 *
 * [stageName] is blank when the stage is free — and a free stage with an empty
 * queue is the normal state of a call, which is why it says nothing at all
 * rather than «никто не показывает экран».
 */
fun confStageLine(
    share: ConfShare,
    stageName: String,
    stageIsMine: Boolean,
    position: Int,
    waiting: Int,
): ConfStageLine {
    val notice = when {
        share == ConfShare.SHARING -> ConfStageNotice.YOU_PRESENT
        share == ConfShare.MY_TURN -> ConfStageNotice.STAGE_YOURS
        stageName.isNotBlank() && !stageIsMine -> ConfStageNotice.OTHER_PRESENTS
        position > 0 -> ConfStageNotice.WAITING
        else -> ConfStageNotice.NONE
    }
    if (notice == ConfStageNotice.NONE) return ConfStageLine()
    return ConfStageLine(
        notice = notice,
        name = if (notice == ConfStageNotice.OTHER_PRESENTS) stageName else "",
        position = position,
        waiting = waiting,
    )
}

/**
 * How often to re-send `screen.refresh` while we hold the stage.
 *
 * A third of the server's own TTL, taken from the server's own welcome rather
 * than from a constant on this side: a heartbeat that drifted out of step with
 * the TTL it feeds would drop the stage out from under a presenter who is still
 * presenting. Three beats per TTL survives two lost frames, which is what a
 * phone changing cells costs.
 */
fun confShareHeartbeatMs(ttlMs: Long): Long =
    (ttlMs / HEARTBEATS_PER_TTL).coerceAtLeast(MIN_HEARTBEAT_MS)

/**
 * Whether a capture in progress has just lost the stage.
 *
 * The snapshot counter is what makes this answerable: «the stage is not ours»
 * and «the server has not answered yet» look identical in the stage alone, and
 * acting on the second would kill a share the moment it started. So a capture is
 * only abandoned once a snapshot *newer than our request* says the stage went
 * elsewhere.
 */
fun confSharePreempted(capturing: Boolean, presenting: Boolean, seq: Int, askedAt: Int): Boolean =
    capturing && !presenting && seq > askedAt

/**
 * Whether the share control may be pressed at all.
 *
 * Tied to the room socket rather than to the media session, and deliberately so:
 * the stage is arbitrated over the socket, so a call whose SFU has not connected
 * can still get into the queue. What it cannot do is capture — but that is the
 * server's turn to give, and by the time it does the media is up or the request
 * has been dropped with the socket.
 */
fun confCanShare(connected: Boolean): Boolean = connected

private const val HEARTBEATS_PER_TTL = 3
private const val MIN_HEARTBEAT_MS = 1000L
