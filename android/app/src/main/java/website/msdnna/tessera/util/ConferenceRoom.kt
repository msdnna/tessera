package website.msdnna.tessera.util

/*
 * The rules the room screen runs on (#2896 §5, web `ConferenceRoom.vue`).
 *
 * Who gets the big tile, what the screen says when there is no picture, and
 * which toolbar buttons may be pressed — all of it decided here, away from the
 * composables, because none of it needs a surface to be right and all of it is
 * wrong in ways a screenshot does not show.
 *
 * They deliberately do not name the SDK's types: the layout reasons about
 * [ConfStagePerson], which `ConfPeer` implements, so a spec can lay out a call
 * without a `Room`, a renderer or an EGL context.
 */

/**
 * How good a participant's link is, as the tile paints it.
 *
 * Our own enum rather than the SDK's `ConnectionQuality` so the screen stays
 * clear of `io.livekit` — the boundary §4 put around the media core is only
 * worth having if the types it hands out respect it too.
 */
enum class ConfQuality {
    EXCELLENT,
    GOOD,
    POOR,
    LOST,
    UNKNOWN,
    ;

    /**
     * Only a struggling link is worth a badge (#2884). A tile that announced
     * «excellent» would put a permanent icon on every face in the call.
     */
    val weak: Boolean get() = this == POOR || this == LOST
}

/** The part of a participant the stage layout reasons about. */
interface ConfStagePerson {
    val identity: String
    val local: Boolean
    val speaking: Boolean

    /** This person is publishing a screen right now — pixels, not a claim on one. */
    val sharingScreen: Boolean
}

/**
 * What goes where: [stage] in the big tile, [strip] along the bottom.
 *
 * [screen] says the stage is showing a shared screen rather than a face, which
 * the tile needs to know — an empty screen tile is a first frame still in
 * flight, not a person without a camera.
 */
data class ConfStage<T>(
    val stage: T?,
    val screen: Boolean,
    val strip: List<T>,
)

/**
 * Lays the call out.
 *
 * A shared screen always takes the stage: it is the reason it was shared, so
 * the speaker steps down into the strip rather than competing with it — and the
 * presenter stays in the strip too, because their face is still worth seeing
 * next to their screen.
 *
 * Otherwise the loudest voice gets the big tile, and it *stays* there through
 * the silence that follows: [lastSpeaker] is what keeps a call from snapping to
 * an arbitrary participant every time somebody stops talking. The fallbacks
 * below it only matter before the first word — a fresh call shows a face rather
 * than an empty stage.
 */
fun <T : ConfStagePerson> confStage(peers: List<T>, lastSpeaker: String = ""): ConfStage<T> {
    val presenter = peers.firstOrNull { it.sharingScreen }
    if (presenter != null) return ConfStage(presenter, screen = true, strip = peers)
    val dominant = peers.firstOrNull { it.speaking }
        ?: peers.firstOrNull { it.identity == lastSpeaker }
        ?: peers.firstOrNull { !it.local }
        ?: peers.firstOrNull()
        ?: return ConfStage(null, screen = false, strip = emptyList())
    return ConfStage(dominant, screen = false, strip = peers.filter { it.identity != dominant.identity })
}

/**
 * Who the stage should remember once everyone goes quiet.
 *
 * [previous] is kept only while that person is still in the call: a stage held
 * for somebody who hung up would fall through to «first remote» on their way
 * out, moving the tile for a reason nobody in the room can see.
 */
fun confLastSpeaker(peers: List<ConfStagePerson>, previous: String): String =
    peers.firstOrNull { it.speaking }?.identity
        ?: previous.takeIf { prev -> peers.any { it.identity == prev } }
        ?: ""

/** What the room screen has to say for itself above the stage. */
enum class ConfBanner {
    NONE,

    /** No SFU on this install. Not a failure to retry — a fact about the server. */
    UNAVAILABLE,

    /** The call is over for everyone. Re-entering restarts the room (#2879). */
    ENDED,

    /** A moderator removed us; coming straight back is not on offer. */
    KICKED,

    /** Something we can try again. */
    ERROR,

    /** The SDK is re-establishing the connection — the call is not over. */
    RECONNECTING,

    CONNECTING,

    /** Live, but Android refused the microphone: nobody can hear us. */
    NO_MIC,
}

/**
 * Picks the one thing worth saying, most final first.
 *
 * One banner rather than a stack: a call that is both unavailable and without a
 * microphone has exactly one problem worth reading, and two lines above the
 * stage on a phone leaves no stage.
 *
 * There is no «insecure context» here, and no «audio is blocked» either — both
 * are browser policies the web has to explain and Android does not have. The
 * honest Android counterpart is [NO_MIC]: a permission the user denied is the
 * local reason a live call is silent, and it is the one the web cannot hit.
 */
fun confBanner(
    status: ConfMediaStatus,
    reason: ConfJoinReason,
    reconnecting: Boolean,
    micGranted: Boolean,
): ConfBanner = when {
    status == ConfMediaStatus.UNAVAILABLE -> ConfBanner.UNAVAILABLE
    reason == ConfJoinReason.ENDED -> ConfBanner.ENDED
    status == ConfMediaStatus.ERROR && reason == ConfJoinReason.KICKED -> ConfBanner.KICKED
    status == ConfMediaStatus.ERROR -> ConfBanner.ERROR
    reconnecting -> ConfBanner.RECONNECTING
    status == ConfMediaStatus.CONNECTING -> ConfBanner.CONNECTING
    status == ConfMediaStatus.LIVE && !micGranted -> ConfBanner.NO_MIC
    else -> ConfBanner.NONE
}

/**
 * Whether the banner should carry a «Повторить».
 *
 * Only [ConfBanner.ERROR] can be fixed by pressing it again. Offering it on an
 * install without LiveKit, on a call that ended, or on a kick teaches people
 * that the button does nothing.
 */
fun confCanRetry(banner: ConfBanner): Boolean = banner == ConfBanner.ERROR

/** The state of the toolbar, given the session and the host's opinion of us. */
data class ConfControls(
    val micOn: Boolean,
    val micEnabled: Boolean,
    val camOn: Boolean,
    val camEnabled: Boolean,
    val switchCamEnabled: Boolean,
    val routeEnabled: Boolean,
)

/**
 * Reads the toolbar off the session.
 *
 * The microphone is *disabled* under a force-mute rather than hidden, as on the
 * web: the server would refuse the publish anyway, and a button that silently
 * does nothing reads as a broken microphone rather than as a host's decision.
 *
 * The camera stays enabled even when Android has not granted it — unlike the
 * browser, we can ask for the permission from the press, and a control greyed
 * out because of a dialog the user has not seen yet is a dead end.
 */
fun confControls(
    status: ConfMediaStatus,
    micOn: Boolean,
    camOn: Boolean,
    forceMuted: Boolean,
): ConfControls {
    val live = status == ConfMediaStatus.LIVE
    return ConfControls(
        micOn = micOn,
        micEnabled = live && !forceMuted,
        camOn = camOn,
        camEnabled = live,
        // Nothing to flip without a camera published, and a front/back button on
        // a call with the camera off is the most pressed no-op on the toolbar.
        switchCamEnabled = live && camOn,
        routeEnabled = live,
    )
}
