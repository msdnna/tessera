package website.msdnna.tessera.util

import website.msdnna.tessera.data.repository.ConfTokenResult

/*
 * The rules the conference media core runs on (#2896 §4, web `useConfTransport`).
 *
 * Everything here is a decision that can be made without a socket, a microphone
 * or an SFU: what a refused token means for the screen, how long to wait before
 * trying again, which speaker to route a call to, and what we are allowed to
 * publish given the permissions the phone granted and the mute a host applied.
 *
 * They live apart from `ConferenceEngine` because the engine cannot be tested —
 * it needs a real LiveKit `Room`, a real audio device and a live server. These
 * are the parts that get them wrong.
 */

/** Where a media session is, as the room screen branches on it. */
enum class ConfMediaStatus {
    /** Not in the call. Also where an ended call lands — leaving is not failure. */
    IDLE,

    /** Token requested, or `Room.connect` in flight. */
    CONNECTING,

    /** Connected to the SFU; tracks may still be arriving. */
    LIVE,

    /** The attempt failed for a reason a retry could fix. */
    ERROR,

    /**
     * This install has no SFU configured. Deliberately not [ERROR]: retrying is
     * pointless and the screen should say so plainly, the way the web does.
     */
    UNAVAILABLE,
}

/**
 * What a token answer means for the session.
 *
 * [status] is where the session goes and [reason] is why, so the caller does not
 * re-read the sealed type to build a message. A grant carries [ConferenceToken]
 * onwards through [ConfJoinPlan.token].
 */
data class ConfJoinPlan(
    val status: ConfMediaStatus,
    val reason: ConfJoinReason = ConfJoinReason.NONE,
    val token: website.msdnna.tessera.data.model.ConferenceToken? = null,
    /** RFC3339 end of a kick cooldown; blank unless [reason] is [ConfJoinReason.KICKED]. */
    val retryAfter: String = "",
)

/** Why a join did not produce a live session. */
enum class ConfJoinReason {
    NONE,
    NOT_CONFIGURED,
    ENDED,
    KICKED,
}

/**
 * Reads the token answer.
 *
 * A refused token is the ordinary case, not the exception: a server without
 * LiveKit, a call somebody ended a minute ago and a moderator's kick are all
 * answers a working backend gives. Only [ConfJoinReason.KICKED] is [ERROR] —
 * an ended call sends the user back to the lobby, where the join button starts
 * the room again (#2879), and «unavailable» is a state of the install.
 */
fun confJoinPlan(result: ConfTokenResult): ConfJoinPlan = when (result) {
    is ConfTokenResult.Granted ->
        ConfJoinPlan(ConfMediaStatus.CONNECTING, token = result.token)

    ConfTokenResult.NotConfigured ->
        ConfJoinPlan(ConfMediaStatus.UNAVAILABLE, ConfJoinReason.NOT_CONFIGURED)

    ConfTokenResult.Ended ->
        ConfJoinPlan(ConfMediaStatus.IDLE, ConfJoinReason.ENDED)

    is ConfTokenResult.Kicked ->
        ConfJoinPlan(ConfMediaStatus.ERROR, ConfJoinReason.KICKED, retryAfter = result.retryAfter)
}

// ── Reconnecting ────────────────────────────────────────────────────────────

/** First backoff step, matching the web room socket. */
const val CONF_RECONNECT_BASE_MS = 1_000L

/**
 * The ceiling. Fifteen seconds rather than a minute because the usual cause is a
 * phone changing networks: the connection comes back long before the wait would.
 */
const val CONF_RECONNECT_MAX_MS = 15_000L

/**
 * How long to wait before reconnect number [attempt] (1-based).
 *
 * Doubling, capped. The cap is computed by comparison rather than by shifting
 * all the way up: on a call left open overnight `attempt` reaches the sixties,
 * and `BASE shl 60` overflows to a negative delay — a busy loop hammering the
 * SFU at exactly the moment it is already struggling.
 */
fun confReconnectDelay(attempt: Int): Long {
    if (attempt <= 1) return CONF_RECONNECT_BASE_MS
    val steps = (attempt - 1).coerceAtMost(MAX_BACKOFF_STEPS)
    val grown = CONF_RECONNECT_BASE_MS shl steps
    return grown.coerceAtMost(CONF_RECONNECT_MAX_MS)
}

/** Enough doublings to pass the ceiling from the base, and no more (see above). */
private const val MAX_BACKOFF_STEPS = 5

/**
 * Whether a dropped connection should be retried at all.
 *
 * A disconnect we asked for, and one that follows the call ending, are both
 * final: reconnecting into a room the server has closed re-opens it for everyone
 * who was on their way out.
 */
fun shouldConfReconnect(leaving: Boolean, ended: Boolean): Boolean = !leaving && !ended

// ── Audio routing ───────────────────────────────────────────────────────────

/** The outputs a phone can offer a call, in the order AudioSwitch reports them. */
enum class ConfAudioRoute {
    BLUETOOTH,
    WIRED_HEADSET,
    EARPIECE,
    SPEAKER,
}

/**
 * Which output a conference should start on.
 *
 * A headset — wired or not — wins whenever one is attached: someone who put one
 * on has already said where they want the sound. With nothing attached the
 * answer is the speaker, not the earpiece, because a conference sits on a desk
 * rather than against an ear.
 *
 * This is the same order `AudioSwitchHandler` applies by default, and it is
 * stated here anyway because the routing menu (§5) has to name the current
 * output *before* the call connects — the handler has no devices to report until
 * it starts. Two sources for one answer is how the menu ends up promising the
 * earpiece while the audio comes out of the speaker, so the handler is told this
 * list explicitly rather than left on its default.
 */
fun preferredAudioRoute(available: List<ConfAudioRoute>): ConfAudioRoute =
    CONF_ROUTE_PRIORITY.firstOrNull { it in available } ?: ConfAudioRoute.SPEAKER

private val CONF_ROUTE_PRIORITY = listOf(
    ConfAudioRoute.BLUETOOTH,
    ConfAudioRoute.WIRED_HEADSET,
    ConfAudioRoute.SPEAKER,
    ConfAudioRoute.EARPIECE,
)

// ── What we may publish ─────────────────────────────────────────────────────

/** Runtime permissions the phone granted for capture. */
data class ConfMediaGrant(val mic: Boolean = false, val cam: Boolean = false)

/** What the user asked for with the toolbar toggles. */
data class ConfMediaWant(val mic: Boolean = true, val cam: Boolean = false)

/** What actually gets published to the SFU. */
data class ConfMediaState(val mic: Boolean, val cam: Boolean)

/**
 * Reconciles the three parties with an opinion about the microphone.
 *
 * The user's toggle, Android's permission grant and the host's force-mute
 * (#2878) all have to agree before a single packet leaves the phone, and they
 * are checked in that order of authority: the host outranks everyone. A denied
 * camera does **not** take the call down — the meeting continues with audio,
 * which is what people came for.
 *
 * Force-mute is applied here rather than by hiding the button because the server
 * republishes the roster on every change: a client that only greyed out its own
 * control would keep transmitting until the user happened to press it.
 */
fun publishableMedia(
    want: ConfMediaWant,
    grant: ConfMediaGrant,
    forceMuted: Boolean = false,
): ConfMediaState = ConfMediaState(
    mic = want.mic && grant.mic && !forceMuted,
    cam = want.cam && grant.cam,
)

/** The Android permissions a join needs, given what it intends to publish. */
fun confRequiredPermissions(want: ConfMediaWant): List<String> = buildList {
    if (want.mic) add(android.Manifest.permission.RECORD_AUDIO)
    if (want.cam) add(android.Manifest.permission.CAMERA)
}

/**
 * Whether a participant's video should be painted, or their avatar instead.
 *
 * A muted camera keeps its publication *and* its track object — the SDK mutes in
 * place rather than unpublishing — so «is there a track» alone answers yes for
 * somebody who turned their camera off, and the tile paints a black rectangle
 * over the avatar. The web hit exactly this (#2890); the rule is the same here
 * because the SDK is.
 */
fun showsVideo(hasTrack: Boolean, muted: Boolean, subscribed: Boolean): Boolean =
    hasTrack && !muted && subscribed

// ── The foreground service ──────────────────────────────────────────────────

/**
 * Whether the call needs its foreground service running.
 *
 * It covers the connecting window too, not just a live call: the service is what
 * keeps the process alive when the user leaves the app, and someone who taps
 * «Войти» and immediately switches away would otherwise have the connection
 * killed halfway through and land back in the lobby with no explanation.
 */
fun needsCallService(status: ConfMediaStatus): Boolean =
    status == ConfMediaStatus.CONNECTING || status == ConfMediaStatus.LIVE
