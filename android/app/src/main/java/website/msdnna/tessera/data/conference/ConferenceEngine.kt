package website.msdnna.tessera.data.conference

import com.twilio.audioswitch.AudioDevice
import io.livekit.android.AudioOptions
import io.livekit.android.LiveKit
import io.livekit.android.LiveKitOverrides
import io.livekit.android.RoomOptions
import io.livekit.android.audio.AudioSwitchHandler
import io.livekit.android.events.RoomEvent
import io.livekit.android.events.collect
import io.livekit.android.renderer.TextureViewRenderer
import io.livekit.android.room.Room
import io.livekit.android.room.participant.ConnectionQuality
import io.livekit.android.room.participant.Participant
import io.livekit.android.room.track.LocalAudioTrackOptions
import io.livekit.android.room.track.LocalVideoTrack
import io.livekit.android.room.track.Track
import io.livekit.android.room.track.VideoTrack
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.updateAndGet
import kotlinx.coroutines.launch
import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.repository.ConferenceRepository
import website.msdnna.tessera.util.ConfAudioRoute
import website.msdnna.tessera.util.ConfJoinReason
import website.msdnna.tessera.util.ConfMediaGrant
import website.msdnna.tessera.util.ConfMediaStatus
import website.msdnna.tessera.util.ConfMediaWant
import website.msdnna.tessera.util.ConfQuality
import website.msdnna.tessera.util.ConfStagePerson
import website.msdnna.tessera.util.confJoinPlan
import website.msdnna.tessera.util.confReconnectDelay
import website.msdnna.tessera.util.needsCallService
import website.msdnna.tessera.util.preferredAudioRoute
import website.msdnna.tessera.util.publishableMedia
import website.msdnna.tessera.util.shouldConfReconnect

/**
 * One participant, as the room screen sees them.
 *
 * Flat and rebuilt whole on every SDK event rather than patched per event — the
 * same contract `internal/confroom` uses on the wire, and for the same reason: a
 * missed event can then never leave a tile on screen for somebody who hung up
 * ten minutes ago.
 *
 * The tracks are SDK objects with their own lifecycle and are held as plain
 * references: a renderer needs the real thing, and copying one is not a thing
 * that exists.
 */
data class ConfPeer(
    override val identity: String,
    val sid: String,
    val name: String,
    override val local: Boolean,
    override val speaking: Boolean,
    val micOn: Boolean,
    val camOn: Boolean,
    val quality: ConfQuality,
    val videoTrack: VideoTrack? = null,
    val screenTrack: VideoTrack? = null,
) : ConfStagePerson {
    override val sharingScreen: Boolean get() = screenTrack != null
}

/** Everything a screen needs to know about the call it is showing. */
data class ConfSession(
    val conferenceId: String = "",
    val status: ConfMediaStatus = ConfMediaStatus.IDLE,
    val reason: ConfJoinReason = ConfJoinReason.NONE,
    /** The server's own sentence, when it sent one. */
    val error: String = "",
    /** RFC3339 end of a kick cooldown; blank unless [reason] is `KICKED`. */
    val retryAfter: String = "",
    val peers: List<ConfPeer> = emptyList(),
    val mic: Boolean = false,
    val cam: Boolean = false,
    /** The SDK is re-establishing the connection; the call is not over. */
    val reconnecting: Boolean = false,
    /** Where the audio is coming out, as the routing menu shows it. */
    val route: ConfAudioRoute = ConfAudioRoute.SPEAKER,
    /**
     * The outputs this phone is offering right now. Empty before the call
     * connects — the handler has nothing to report until it starts — which is
     * why the menu is disabled rather than showing a made-up list.
     */
    val routes: List<ConfAudioRoute> = emptyList(),
    /**
     * A host has silenced us (#2878). Published rather than kept private to the
     * engine because the toolbar has to *say* so: a microphone button that is
     * merely off looks like our own last press.
     */
    val forceMuted: Boolean = false,
) {
    val live: Boolean get() = status == ConfMediaStatus.LIVE
}

/**
 * The media core of a conference (#2896 §4, web `useConfTransport`).
 *
 * Everything the app knows about call media goes through here, and nothing above
 * this file imports `io.livekit`. That is not ceremony: the SDK is the one
 * dependency in this feature we do not control, and its participant/track API
 * has already been renamed once across a major version — keeping it behind a
 * plain `{ status, peers, mic, … }` surface means a version bump touches one
 * file instead of five screens.
 *
 * A singleton, not a ViewModel-owned object, because a call outlives the screen
 * that started it: the user opens a task mid-meeting and the audio has to keep
 * going. That is also what the minimised bar in §9 attaches to.
 *
 * The decisions worth arguing about are not here — they are pure functions in
 * `util/ConferenceMedia.kt`, where they can be tested without an SFU. This file
 * is the wiring that cannot.
 */
object ConferenceEngine {
    private val _session = MutableStateFlow(ConfSession())
    val session: StateFlow<ConfSession> = _session.asStateFlow()

    private val repo = ConferenceRepository()
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)

    private var room: Room? = null
    private var audio: AudioSwitchHandler? = null
    private var connectJob: Job? = null
    private var eventJob: Job? = null

    /** Set by [leave] so a connect still in flight tears itself down on arrival. */
    private var leaving = false

    /** The call ended for everyone — a reconnect would re-open a closed room. */
    private var ended = false
    private var attempt = 0

    private var want = ConfMediaWant()
    private var grant = ConfMediaGrant()

    /** A host silenced us (#2878). Held here so a reconnect re-applies it. */
    private var forceMuted = false

    /** Turned off by tests, which have no ServiceManager to start anything on. */
    internal var serviceEnabled = true

    /**
     * Join [conferenceId] and start publishing what [grant] allows.
     *
     * The seat itself is not taken here — `/join` is the lobby's business (§3)
     * and stays server state. This is the media half: a token, an SFU connection
     * and the tracks.
     */
    fun join(conferenceId: String, want: ConfMediaWant, grant: ConfMediaGrant) {
        val status = _session.value.status
        if (status == ConfMediaStatus.CONNECTING || status == ConfMediaStatus.LIVE) return
        this.want = want
        this.grant = grant
        leaving = false
        ended = false
        attempt = 0
        connectJob?.cancel()
        connectJob = scope.launch { connect(conferenceId) }
    }

    private suspend fun connect(conferenceId: String) {
        // Before the first status change, because that is what starts the
        // service: a foreground notification posted to a channel that does not
        // exist yet is dropped, and a service without one is killed on the spot.
        // Here rather than at startup — this is the first moment we are in a
        // coroutine and the profile's language is readable.
        if (serviceEnabled) runCatching { ConferenceCallService.ensureChannel(AppContainer.appContext) }
        set { it.copy(conferenceId = conferenceId, status = ConfMediaStatus.CONNECTING, error = "", reason = ConfJoinReason.NONE) }
        val plan = try {
            confJoinPlan(repo.token(conferenceId))
        } catch (e: Exception) {
            failed(e)
            return
        }
        val token = plan.token
        if (token == null) {
            set { it.copy(status = plan.status, reason = plan.reason, retryAfter = plan.retryAfter) }
            if (plan.reason == ConfJoinReason.ENDED) ended = true
            return
        }
        if (leaving) {
            set { it.copy(status = ConfMediaStatus.IDLE) }
            return
        }
        try {
            val handler = AudioSwitchHandler(AppContainer.appContext).apply {
                preferredDeviceList = AUDIO_PRIORITY
            }
            val r = LiveKit.create(
                appContext = AppContainer.appContext,
                options = RoomOptions(
                    // Drops the resolution of a tile nobody is looking at, and stops
                    // publishing layers nobody subscribes to. Both matter more on a
                    // phone than on a desktop: this is somebody's mobile data.
                    adaptiveStream = true,
                    dynacast = true,
                    // The baseline every call app turns on. AGC also lifts a quiet
                    // speaker, which is what makes a laptop microphone audible.
                    audioTrackCaptureDefaults = LocalAudioTrackOptions(
                        echoCancellation = true,
                        noiseSuppression = true,
                        autoGainControl = true,
                    ),
                ),
                overrides = LiveKitOverrides(audioOptions = AudioOptions(audioHandler = handler)),
            )
            room = r
            audio = handler
            listen(r)
            r.connect(token.url, token.token)
            if (leaving) {
                teardown()
                set { it.copy(status = ConfMediaStatus.IDLE) }
                return
            }
            applyMedia()
            attempt = 0
            set { it.copy(status = ConfMediaStatus.LIVE, reconnecting = false, route = currentRoute(), routes = availableRoutes()) }
            rebuild()
        } catch (e: Exception) {
            teardown()
            failed(e)
            scheduleRetry(conferenceId)
        }
    }

    /**
     * Try again after a connection we did not give up on.
     *
     * The whole join is repeated, token included, rather than just the socket:
     * the token is short-lived, and one that expired while the phone was in a
     * tunnel would fail the reconnect for a reason the user cannot act on.
     */
    private fun scheduleRetry(conferenceId: String) {
        if (!shouldConfReconnect(leaving, ended)) return
        attempt += 1
        val wait = confReconnectDelay(attempt)
        connectJob = scope.launch {
            delay(wait)
            if (shouldConfReconnect(leaving, ended)) connect(conferenceId)
        }
    }

    /** Leave the call and release the microphone, camera and audio focus. */
    fun leave() {
        leaving = true
        connectJob?.cancel()
        connectJob = null
        // A host's mute is server state and the room socket restates it on the
        // next join (#2878, §6). Keeping it here would silence the *next* call
        // instead, with nothing on screen able to lift it.
        forceMuted = false
        teardown()
        set {
            ConfSession(conferenceId = it.conferenceId, status = ConfMediaStatus.IDLE)
        }
    }

    private fun teardown() {
        eventJob?.cancel()
        eventJob = null
        val r = room
        room = null
        audio = null
        runCatching { r?.disconnect() }
        serviceNeeded(false)
    }

    /** Turn the microphone on or off; a host's force-mute still overrides it. */
    fun setMic(on: Boolean) {
        want = want.copy(mic = on)
        scope.launch { applyMedia() }
    }

    fun setCam(on: Boolean) {
        want = want.copy(cam = on)
        scope.launch { applyMedia() }
    }

    /**
     * Android answered a permission dialog mid-call.
     *
     * The grant is normally settled before [join], but the camera is asked for
     * from the toolbar button — the first person to press it does so in a call
     * that is already live, and without this the answer would only take effect
     * on the next join.
     */
    fun updateGrant(grant: ConfMediaGrant) {
        if (this.grant == grant) return
        this.grant = grant
        scope.launch { applyMedia() }
    }

    /** Front/back. Silently does nothing without a camera published. */
    fun switchCamera() {
        val track = room?.localParticipant?.getTrackPublication(Track.Source.CAMERA)?.track
        (track as? LocalVideoTrack)?.switchCamera()
    }

    /**
     * Apply the roster's view of our own microphone (#2878).
     *
     * Called by the room socket, which is the only place a force-mute arrives:
     * the SFU has no opinion about who may speak, so a client that waited for a
     * track event would keep transmitting after being silenced.
     */
    fun applyForceMute(muted: Boolean) {
        if (forceMuted == muted) return
        forceMuted = muted
        set { it.copy(forceMuted = muted) }
        scope.launch { applyMedia() }
    }

    /** Send the call to a particular output; the menu in §5 drives this. */
    fun selectRoute(route: ConfAudioRoute) {
        val handler = audio ?: return
        val device = handler.availableAudioDevices.firstOrNull { it.toRoute() == route } ?: return
        handler.selectDevice(device)
        set { it.copy(route = route) }
    }

    /** What we are permitted to publish right now, pushed to the SFU. */
    private suspend fun applyMedia() {
        val local = room?.localParticipant ?: return
        val state = publishableMedia(want, grant, forceMuted)
        runCatching { local.setMicrophoneEnabled(state.mic) }
        runCatching { local.setCameraEnabled(state.cam) }
        set { it.copy(mic = state.mic, cam = state.cam) }
    }

    private fun listen(r: Room) {
        eventJob?.cancel()
        eventJob = scope.launch {
            r.events.collect { event ->
                when (event) {
                    // The SDK's own reconnect. Not an error and not a leave — the
                    // screen keeps the tiles and says so, because a call that
                    // blanked itself on every lift ride would look broken.
                    is RoomEvent.Reconnecting -> set { it.copy(reconnecting = true) }

                    is RoomEvent.Reconnected -> {
                        set { it.copy(reconnecting = false) }
                        // The SFU starts a reconnected session with nothing
                        // published: restate the tracks, or we come back silent
                        // while our own toolbar shows the microphone on.
                        applyMedia()
                        rebuild()
                    }

                    is RoomEvent.Disconnected -> onDisconnected(event)

                    is RoomEvent.FailedToConnect -> {
                        failed(event.error)
                        scheduleRetry(_session.value.conferenceId)
                    }

                    else -> rebuild()
                }
            }
        }
    }

    /**
     * The SFU dropped us and the SDK has stopped trying.
     *
     * Falling back to [ConfMediaStatus.IDLE] rather than an error keeps the
     * screen honest about the common case — the meeting ended, or we were
     * removed — while [scheduleRetry] covers the case where the network is what
     * went away.
     */
    private fun onDisconnected(event: RoomEvent.Disconnected) {
        val id = _session.value.conferenceId
        teardown()
        set { it.copy(status = ConfMediaStatus.IDLE, peers = emptyList(), mic = false, cam = false, reconnecting = false) }
        if (event.error != null) scheduleRetry(id)
    }

    private fun failed(e: Throwable) {
        set {
            it.copy(
                status = ConfMediaStatus.ERROR,
                error = e.message.orEmpty(),
                reconnecting = false,
            )
        }
    }

    /** Rebuild the whole roster from the room — never a per-event patch. */
    private fun rebuild() {
        val r = room ?: return
        val peers = buildList {
            add(describe(r.localParticipant, local = true))
            r.remoteParticipants.values.forEach { add(describe(it, local = false)) }
        }
        set { it.copy(peers = peers, route = currentRoute(), routes = availableRoutes()) }
    }

    private fun describe(p: Participant, local: Boolean): ConfPeer {
        val cam = p.getTrackPublication(Track.Source.CAMERA)
        val screen = p.getTrackPublication(Track.Source.SCREEN_SHARE)
        return ConfPeer(
            identity = p.identity?.value.orEmpty(),
            sid = p.sid.value,
            // The display name is minted server-side alongside the token; the
            // identity is a raw UUID and would be printed on the tile.
            name = p.name?.takeIf { it.isNotBlank() } ?: p.identity?.value.orEmpty(),
            local = local,
            speaking = p.isSpeaking,
            micOn = p.isMicrophoneEnabled,
            camOn = p.isCameraEnabled,
            quality = p.connectionQuality.toConfQuality(),
            // A muted camera keeps both its publication and its track object —
            // the SDK mutes in place instead of unpublishing — so "has a track"
            // alone paints a black rectangle over the avatar of somebody who
            // turned their camera off. The web hit exactly this (#2890).
            videoTrack = (cam?.track as? VideoTrack)?.takeIf { cam.muted.not() && cam.subscribed },
            screenTrack = (screen?.track as? VideoTrack)?.takeIf { screen.muted.not() && screen.subscribed },
        )
    }

    private fun currentRoute(): ConfAudioRoute {
        val handler = audio ?: return ConfAudioRoute.SPEAKER
        handler.selectedAudioDevice?.toRoute()?.let { return it }
        return preferredAudioRoute(availableRoutes())
    }

    private fun availableRoutes(): List<ConfAudioRoute> =
        audio?.availableAudioDevices?.mapNotNull { it.toRoute() }.orEmpty()

    private fun set(block: (ConfSession) -> ConfSession) {
        val next = _session.updateAndGet(block)
        serviceNeeded(needsCallService(next.status))
    }

    private fun serviceNeeded(needed: Boolean) {
        if (!serviceEnabled) return
        ConferenceCallService.apply(AppContainer.appContext, needed)
    }

    /**
     * Hand a freshly created renderer the room's EGL context.
     *
     * Internal, and the only reason `data/conference` owns a composable at all:
     * a renderer that skips this shows a black rectangle for a track that is
     * arriving perfectly well, which looks exactly like a camera nobody turned
     * on.
     */
    internal fun attachRenderer(view: TextureViewRenderer) {
        runCatching { room?.initVideoRenderer(view) }
    }

    private fun ConnectionQuality.toConfQuality(): ConfQuality = when (this) {
        ConnectionQuality.EXCELLENT -> ConfQuality.EXCELLENT
        ConnectionQuality.GOOD -> ConfQuality.GOOD
        ConnectionQuality.POOR -> ConfQuality.POOR
        ConnectionQuality.LOST -> ConfQuality.LOST
        ConnectionQuality.UNKNOWN -> ConfQuality.UNKNOWN
    }

    /** AudioSwitch's device classes, in the order [preferredAudioRoute] states. */
    private val AUDIO_PRIORITY = listOf(
        AudioDevice.BluetoothHeadset::class.java,
        AudioDevice.WiredHeadset::class.java,
        AudioDevice.Speakerphone::class.java,
        AudioDevice.Earpiece::class.java,
    )

    private fun AudioDevice.toRoute(): ConfAudioRoute? = when (this) {
        is AudioDevice.BluetoothHeadset -> ConfAudioRoute.BLUETOOTH

        is AudioDevice.WiredHeadset -> ConfAudioRoute.WIRED_HEADSET

        is AudioDevice.Speakerphone -> ConfAudioRoute.SPEAKER

        is AudioDevice.Earpiece -> ConfAudioRoute.EARPIECE

        // A device class this SDK version added and we do not map yet. Dropping
        // it leaves the menu naming the ones we know rather than crashing on an
        // output somebody plugged in.
        else -> null
    }
}
