package website.msdnna.tessera.data.conference

import com.google.gson.Gson
import com.google.gson.annotations.SerializedName
import java.util.concurrent.TimeUnit
import kotlin.math.min
import kotlin.random.Random
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import website.msdnna.tessera.data.api.RetrofitClient

/*
 * The conference room socket (#2896 §6, web `useConfRoom`) — everything in a
 * call that is not media.
 *
 * Two connections, on purpose, and the split is not ours to change: audio and
 * video go straight to the SFU through [ConferenceEngine], while this one
 * carries the room's own bookkeeping — who is here, whose hand is up, and the
 * moderation commands. LiveKit only verifies a token signature, so «may this
 * person kick that one» has exactly one possible arbiter and it is our server.
 *
 * The server sends a whole-room snapshot on every change rather than a delta,
 * which is what makes a reconnect self-healing: the new state replaces the old
 * one, and a frame lost to a dropped connection cannot leave somebody on screen
 * who walked out ten minutes ago.
 */

/** One person in the call, as the room reports them. Mirrors `confroom.PersonView`. */
data class ConfPerson(
    @SerializedName("user_id") val userId: String = "",
    @SerializedName("name") val name: String = "",
    @SerializedName("role") val role: String = ROLE_MEMBER,
    /**
     * How many sockets this person has open. The panel shows one row either way —
     * a colleague with the call on a phone and a laptop is one colleague.
     */
    @SerializedName("conns") val conns: Int = 0,
    @SerializedName("mic") val mic: Boolean = false,
    @SerializedName("cam") val cam: Boolean = false,
    @SerializedName("force_muted") val forceMuted: Boolean = false,
    /** RFC3339 instant the hand went up, or null. Ordering reads it as a string. */
    @SerializedName("hand_at") val handAt: String? = null,
    @SerializedName("joined_at") val joinedAt: String = "",
) {
    val host: Boolean get() = role == ROLE_HOST
}

/** Who holds the screen-share stage. Parsed here, used by §8. */
data class ConfStageHolder(
    @SerializedName("user_id") val userId: String = "",
    @SerializedName("conn_id") val connId: String = "",
    @SerializedName("name") val name: String = "",
)

/** One waiting request for the stage, in the order it will be served (§8). */
data class ConfQueueEntry(
    @SerializedName("user_id") val userId: String = "",
    @SerializedName("conn_id") val connId: String = "",
    @SerializedName("name") val name: String = "",
    @SerializedName("role") val role: String = ROLE_MEMBER,
)

/** The «this call is being recorded» indicator (§8). */
data class ConfRecording(
    @SerializedName("id") val id: String = "",
    @SerializedName("started_at") val startedAt: String = "",
    @SerializedName("started_by") val startedBy: String = "",
)

/** A command the room refused, with the action it belonged to. */
data class ConfDenied(val action: String, val reason: String)

/** Why the room is over for us. */
enum class ConfRoomEnd {
    NONE,
    ENDED,
    DELETED,
    KICKED,
}

/**
 * The room as this connection sees it.
 *
 * [connected] is deliberately separate from an empty [people]: a socket that is
 * reconnecting has no roster to show, and a panel that kept the last one would
 * offer a mute button for somebody who may already have left.
 */
data class ConfRoomState(
    val connected: Boolean = false,
    val connId: String = "",
    val meId: String = "",
    val role: String = ROLE_MEMBER,
    /**
     * Whether the server will accept a kick or a force-mute from us (#2878).
     * Apart from [role] on purpose: a workspace admin sits in the call as a plain
     * member and still holds the controls, so the panel reads this and not «host».
     */
    val canModerate: Boolean = false,
    val people: List<ConfPerson> = emptyList(),
    val stage: ConfStageHolder? = null,
    val queue: List<ConfQueueEntry> = emptyList(),
    val recording: ConfRecording? = null,
    val denied: ConfDenied? = null,
    val ended: ConfRoomEnd = ConfRoomEnd.NONE,
    /**
     * Bumped whenever the room says the chat changed (§7). Payload-free by
     * design: messages are fetched over HTTP, so a nudge lost to a reconnect
     * costs a stale panel rather than a message that never existed.
     */
    val chatNudge: Int = 0,
    /**
     * How many snapshots this connection has seen (§8).
     *
     * The screen-share flow needs to tell «the server has not answered my
     * request yet» from «the server answered and the stage is somebody else's» —
     * the two are identical in [stage] alone, and acting on the first would kill
     * a capture the moment it started.
     */
    val stateSeq: Int = 0,
    /**
     * How long the stage survives without a refresh, straight from the server's
     * welcome, so our heartbeat cannot drift out of step with the TTL it feeds.
     */
    val stageTtlMs: Long = DEFAULT_STAGE_TTL_MS,
) {
    /** Me, as the room sees me — the source of truth for my own force-mute. */
    val self: ConfPerson? get() = people.firstOrNull { it.userId == meId }

    /** Whether a moderator has silenced me. The engine refuses my mic while true. */
    val forceMuted: Boolean get() = self?.forceMuted == true

    /** Whether my hand is up, as the room has it — not as we last pressed. */
    val handUp: Boolean get() = self?.handAt != null

    /**
     * Whether *this connection* holds the stage (§8).
     *
     * By connection and not by user: the same person may have the call open on a
     * phone and a laptop, and the second one must not believe it is presenting
     * and offer to stop a screen it is not publishing.
     */
    val presenting: Boolean get() = stage != null && stage.connId == connId

    /** The queue as connection ids, in the order the server will serve them. */
    val queueConns: List<String> get() = queue.map { it.connId }
}

const val ROLE_HOST = "host"
const val ROLE_MEMBER = "member"

/**
 * The stage TTL assumed until the welcome says otherwise — the same 30s
 * `internal/confroom` uses. A default rather than a zero: a snapshot that
 * arrives before the welcome would otherwise set the heartbeat to «never».
 */
const val DEFAULT_STAGE_TTL_MS = 30_000L

/**
 * The per-conference room socket.
 *
 * Held open for exactly as long as we occupy a seat. Permission is never checked
 * here: a member's kick is sent, refused by the server and answered with a
 * `denied` frame. Hiding the button is a courtesy to honest users, not a
 * security boundary — and the panel above still hides it, because a control that
 * only ever fails teaches that controls do not work.
 */
class ConferenceRoomSocket {
    private val _state = MutableStateFlow(ConfRoomState())
    val state: StateFlow<ConfRoomState> = _state.asStateFlow()

    private val client = OkHttpClient.Builder()
        .pingInterval(PING_SECONDS, TimeUnit.SECONDS)
        .retryOnConnectionFailure(true)
        .build()
    private val gson = Gson()

    @Volatile private var ws: WebSocket? = null

    @Volatile private var closed = true

    @Volatile private var conferenceId: String = ""

    private var attempts = 0

    /**
     * What we last told the room about our devices.
     *
     * Restated after every welcome: the server starts each connection with the
     * microphone and camera off, so a client that stayed quiet through a
     * reconnect would show as muted to the whole room while actually talking.
     */
    @Volatile private var media = MediaReport(mic = false, cam = false)

    /** Swapped by tests, which have no server and no main looper to post on. */
    internal var scheduler: (Long, () -> Unit) -> Unit = { delayMs, block ->
        android.os.Handler(android.os.Looper.getMainLooper()).postDelayed(block, delayMs)
    }

    fun open(conferenceId: String) {
        if (conferenceId.isBlank()) return
        close()
        this.conferenceId = conferenceId
        closed = false
        attempts = 0
        _state.value = ConfRoomState()
        connect()
    }

    fun close() {
        closed = true
        conferenceId = ""
        media = MediaReport(mic = false, cam = false)
        ws?.close(NORMAL_CLOSURE, null)
        ws = null
        // Everything drops with the socket, the recording dot included: this is
        // leaving the call rather than losing touch with it, and a dot carried
        // into the next room would announce a meeting we are no longer in.
        _state.value = ConfRoomState()
    }

    // ── commands ──────────────────────────────────────────────────────────

    /**
     * Report this connection's microphone and camera.
     *
     * The room is not the source of truth for the tracks — the SFU is — but the
     * roster has to paint a muted badge before the first audio packet arrives,
     * and a force-muted participant claiming a live microphone is refused rather
     * than believed.
     */
    fun setMedia(mic: Boolean, cam: Boolean) {
        media = MediaReport(mic = mic, cam = cam)
        send(mapOf("type" to TYPE_MEDIA, "mic" to mic, "cam" to cam))
    }

    /** Raise or lower a hand; the server orders the list by when it went up. */
    fun raiseHand(up: Boolean) = send(mapOf("type" to TYPE_HAND, "up" to up))

    /** Remove someone from the call. Moderators only — the server decides. */
    fun kick(userId: String) {
        _state.update { it.copy(denied = null) }
        send(mapOf("type" to TYPE_KICK, "user_id" to userId))
    }

    /** Silence someone for everyone, or lift it. Moderators only. */
    fun forceMute(userId: String, muted: Boolean) {
        _state.update { it.copy(denied = null) }
        send(mapOf("type" to TYPE_MUTE, "user_id" to userId, "muted" to muted))
    }

    // ── the screen-share stage (§8) ───────────────────────────────────────

    /**
     * Ask for the stage.
     *
     * The answer arrives as a snapshot, never as a return value: the server is
     * the only arbiter, and a client that started capturing because it asked is
     * how two screens end up published at once. Asking while somebody else
     * presents queues us — unless we may preempt them, which is also the
     * server's call and not ours.
     */
    fun requestScreen() = send(mapOf("type" to TYPE_SCREEN_REQUEST))

    /** Keep our hold alive; the server ignores it from anyone else. */
    fun refreshScreen() = send(mapOf("type" to TYPE_SCREEN_REFRESH))

    /** Give the stage up, or step out of the queue if we were only waiting. */
    fun releaseScreen() = send(mapOf("type" to TYPE_SCREEN_RELEASE))

    /** Drop a refusal once it has been read — a stale one under an untouched
     *  button is worse than no message at all. */
    fun clearDenied() = _state.update { it.copy(denied = null) }

    // ── transport ─────────────────────────────────────────────────────────

    private fun send(msg: Map<String, Any>) {
        ws?.send(gson.toJson(msg))
    }

    private fun connect() {
        if (closed || conferenceId.isBlank()) return
        val url = wsUrl(conferenceId) ?: return
        // Read the token per attempt: a refresh-on-401 may have rotated it since.
        val token = RetrofitClient.authToken
        if (token.isBlank()) {
            scheduleReconnect()
            return
        }
        val req = Request.Builder().url(url).header("Authorization", "Bearer $token").build()
        ws = client.newWebSocket(
            req,
            object : WebSocketListener() {
                override fun onOpen(webSocket: WebSocket, response: Response) {
                    attempts = 0
                    _state.update { it.copy(connected = true) }
                }

                override fun onMessage(webSocket: WebSocket, text: String) = onFrame(text)

                override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) =
                    onGone()

                override fun onClosed(webSocket: WebSocket, code: Int, reason: String) = onGone()
            },
        )
    }

    private fun onGone() {
        ws = null
        // A room we were thrown out of, or one that ended, must not be retried:
        // the handshake answers 403 or 409 and the loop would run until the
        // screen goes away.
        if (_state.value.ended != ConfRoomEnd.NONE) return
        _state.update {
            it.copy(connected = false, people = emptyList(), stage = null, queue = emptyList())
        }
        scheduleReconnect()
    }

    private fun scheduleReconnect() {
        if (closed) return
        // Full jitter, as everywhere else: a backend restart must not have every
        // open call reconnect in lockstep. Compared rather than shifted — an
        // attempt count that reaches the sixties overflows a shift into a
        // negative delay, and the retry turns into a busy loop against a server
        // that is already having a bad time.
        val cap = min(RECONNECT_MAX_MS, RECONNECT_BASE_MS * (1L shl min(attempts, MAX_SHIFT)))
        attempts += 1
        scheduler(Random.nextLong(cap + 1)) { connect() }
    }

    private fun onFrame(text: String) {
        val type = runCatching { gson.fromJson(text, TypeOnly::class.java)?.type }.getOrNull()
        when (type) {
            TYPE_WELCOME -> {
                val msg = parse<WelcomeMsg>(text) ?: return
                _state.update {
                    it.copy(
                        connId = msg.connId,
                        meId = msg.userId,
                        role = msg.role.ifBlank { ROLE_MEMBER },
                        canModerate = msg.canModerate,
                        // A server that did not send one keeps the default; a
                        // zero TTL would turn the heartbeat into a busy loop.
                        stageTtlMs = msg.stageTtlMs.takeIf { it > 0 } ?: it.stageTtlMs,
                    )
                }
                // A reconnect is a fresh connection to a server that knows
                // nothing about the devices we already have open.
                val m = media
                if (m.mic || m.cam) send(mapOf("type" to TYPE_MEDIA, "mic" to m.mic, "cam" to m.cam))
            }

            TYPE_STATE -> {
                val msg = parse<StateMsg>(text) ?: return
                _state.update {
                    it.copy(
                        people = msg.participants.orEmpty(),
                        stage = msg.stage,
                        queue = msg.queue.orEmpty(),
                        recording = msg.recording,
                        stateSeq = it.stateSeq + 1,
                    )
                }
            }

            TYPE_DENIED -> {
                val msg = parse<DeniedMsg>(text) ?: return
                _state.update { it.copy(denied = ConfDenied(msg.action, msg.reason)) }
            }

            TYPE_CHAT -> _state.update { it.copy(chatNudge = it.chatNudge + 1) }

            TYPE_ENDED -> {
                val msg = parse<EndedMsg>(text)
                val end = when (msg?.reason) {
                    REASON_KICKED -> ConfRoomEnd.KICKED
                    REASON_DELETED -> ConfRoomEnd.DELETED
                    else -> ConfRoomEnd.ENDED
                }
                // Set before the close so the listener's `onGone` sees it and
                // does not schedule a reconnect into a room that is gone.
                _state.update { it.copy(ended = end, connected = false) }
                closed = true
                ws?.close(NORMAL_CLOSURE, null)
                ws = null
            }

            else -> Unit
        }
    }

    private inline fun <reified T> parse(text: String): T? =
        runCatching { gson.fromJson(text, T::class.java) }.getOrNull()

    /** Feeds a raw frame in without a server. Tests only. */
    internal fun receiveForTest(text: String) = onFrame(text)

    /** Derives ws(s)://…/api/conferences/{id}/ws from the active server root. */
    private fun wsUrl(id: String): String? {
        val root = RetrofitClient.serverRoot.ifBlank { return null }
        val scheme = when {
            root.startsWith("https") -> "wss" + root.removePrefix("https")
            root.startsWith("http") -> "ws" + root.removePrefix("http")
            else -> return null
        }
        return "$scheme/api/conferences/$id/ws"
    }

    private data class MediaReport(val mic: Boolean, val cam: Boolean)

    private data class TypeOnly(@SerializedName("type") val type: String = "")

    private data class WelcomeMsg(
        @SerializedName("conn_id") val connId: String = "",
        @SerializedName("user_id") val userId: String = "",
        @SerializedName("role") val role: String = "",
        @SerializedName("can_moderate") val canModerate: Boolean = false,
        @SerializedName("stage_ttl_ms") val stageTtlMs: Long = 0,
    )

    private data class StateMsg(
        @SerializedName("participants") val participants: List<ConfPerson>? = null,
        @SerializedName("stage") val stage: ConfStageHolder? = null,
        @SerializedName("queue") val queue: List<ConfQueueEntry>? = null,
        @SerializedName("recording") val recording: ConfRecording? = null,
    )

    private data class DeniedMsg(
        @SerializedName("action") val action: String = "",
        @SerializedName("reason") val reason: String = "",
    )

    private data class EndedMsg(@SerializedName("reason") val reason: String = "")

    companion object {
        const val TYPE_WELCOME = "welcome"
        const val TYPE_STATE = "state"
        const val TYPE_DENIED = "denied"
        const val TYPE_CHAT = "chat"
        const val TYPE_ENDED = "ended"

        const val TYPE_MEDIA = "media"
        const val TYPE_HAND = "hand"
        const val TYPE_KICK = "kick"
        const val TYPE_MUTE = "mute"
        const val TYPE_SCREEN_REQUEST = "screen.request"
        const val TYPE_SCREEN_REFRESH = "screen.refresh"
        const val TYPE_SCREEN_RELEASE = "screen.release"

        const val REASON_KICKED = "kicked"
        const val REASON_DELETED = "deleted"

        private const val PING_SECONDS = 20L
        private const val NORMAL_CLOSURE = 1000
        private const val RECONNECT_BASE_MS = 1000L
        private const val RECONNECT_MAX_MS = 15000L
        private const val MAX_SHIFT = 5
    }
}
