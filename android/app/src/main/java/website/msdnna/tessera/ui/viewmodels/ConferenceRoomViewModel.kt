package website.msdnna.tessera.ui.viewmodels

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import website.msdnna.tessera.data.conference.ConfPeer
import website.msdnna.tessera.data.conference.ConfPerson
import website.msdnna.tessera.data.conference.ConfRoomEnd
import website.msdnna.tessera.data.conference.ConfRoomState
import website.msdnna.tessera.data.conference.ConfSession
import website.msdnna.tessera.data.conference.ConferenceEngine
import website.msdnna.tessera.data.conference.ConferenceRoomSocket
import website.msdnna.tessera.data.repository.ConferenceRepository
import website.msdnna.tessera.util.ConfAudioRoute
import website.msdnna.tessera.util.ConfBanner
import website.msdnna.tessera.util.ConfControls
import website.msdnna.tessera.util.ConfDeniedKind
import website.msdnna.tessera.util.ConfMediaGrant
import website.msdnna.tessera.util.ConfMediaWant
import website.msdnna.tessera.util.ConfMiniLine
import website.msdnna.tessera.util.ConfRecordingFailure
import website.msdnna.tessera.util.ConfRecordingPress
import website.msdnna.tessera.util.ConfRowActions
import website.msdnna.tessera.util.ConfShare
import website.msdnna.tessera.util.ConfSharePress
import website.msdnna.tessera.util.ConfStage
import website.msdnna.tessera.util.ConfStageLine
import website.msdnna.tessera.util.confBanner
import website.msdnna.tessera.util.confCanShare
import website.msdnna.tessera.util.confControls
import website.msdnna.tessera.util.confDeniedKind
import website.msdnna.tessera.util.confHasLocalAudio
import website.msdnna.tessera.util.confLastSpeaker
import website.msdnna.tessera.util.confMiniLine
import website.msdnna.tessera.util.confMiniMicEnabled
import website.msdnna.tessera.util.confMiniOthers
import website.msdnna.tessera.util.confPanelOrder
import website.msdnna.tessera.util.confQueuePosition
import website.msdnna.tessera.util.confRecordingFailure
import website.msdnna.tessera.util.confRecordingPress
import website.msdnna.tessera.util.confRowActions
import website.msdnna.tessera.util.confShareAutoCapture
import website.msdnna.tessera.util.confShareHeartbeatMs
import website.msdnna.tessera.util.confSharePreempted
import website.msdnna.tessera.util.confSharePress
import website.msdnna.tessera.util.confShareState
import website.msdnna.tessera.util.confStage
import website.msdnna.tessera.util.confStageLine

data class ConferenceRoomUiState(
    val session: ConfSession = ConfSession(),
    /** The room socket's half: presence, roles, hands, moderation (#2896 §6). */
    val room: ConfRoomState = ConfRoomState(),
    /** What Android has actually granted us — the toolbar and [banner] both read it. */
    val grant: ConfMediaGrant = ConfMediaGrant(),
    /**
     * Who held the stage last. Carried in the state rather than recomputed from
     * the peers because it is precisely the thing the peers no longer say: it is
     * what keeps the big tile still through a silence.
     */
    val lastSpeaker: String = "",
    /** The stage alone, strip and toolbar folded away — this screen's fullscreen. */
    val stageOnly: Boolean = false,
    val routeMenu: Boolean = false,
    /** The participants panel, as a sheet over the call. */
    val panelOpen: Boolean = false,
    /** Whose kick is waiting on a confirmation; blank when none is. */
    val confirmingKick: String = "",
    /**
     * We want the stage and have not given up on it (#2896 §8).
     *
     * Not the same as holding it and not the same as capturing: the three are
     * kept apart because that is what makes the queue work — waiting is a state
     * with a control of its own («Выйти из очереди»), not an absence.
     */
    val wantScreen: Boolean = false,
    /**
     * The snapshot number our request went out at, so «the stage is not ours»
     * can be told from «the server has not answered yet». Acting on the second
     * would stop a capture the instant it started.
     */
    val askedStageAt: Int = 0,
    /**
     * The consent dialog may follow the request without a second press, because
     * the stage was free when it was pressed. Consumed by the screen the moment
     * the turn arrives — see [website.msdnna.tessera.util.confShareAutoCapture].
     */
    val autoCapture: Boolean = false,
    /** A recording start/stop the server refused, until it is read. */
    val recordingError: ConfRecordingFailure? = null,
    /** A recording call is in flight; the button must not be pressed twice. */
    val recordingBusy: Boolean = false,
    /**
     * The room screen is composed right now (#2896 §9).
     *
     * The minimised bar's other half: since §9 a call outlives the screen, so
     * «is there a call» no longer answers «should the bar be drawn». Reported by
     * the screen itself rather than derived from the shell's destination — the
     * section, the lobby and the call are three layers of one screen, and a bar
     * keyed to the destination would paint itself over the call it links to.
     */
    val roomOnScreen: Boolean = false,
) {
    val stage: ConfStage<ConfPeer> get() = confStage(session.peers, lastSpeaker)

    val banner: ConfBanner
        get() = confBanner(session.status, session.reason, session.reconnecting, grant.mic)

    val controls: ConfControls
        get() = confControls(session.status, session.mic, session.cam, session.forceMuted)

    /** The roster the panel paints: hands first, oldest first, then join order. */
    val roster: List<ConfPerson> get() = confPanelOrder(room.people)

    val denied: ConfDeniedKind get() = confDeniedKind(room.denied)

    /** The person a kick confirmation is about, or null once they have gone. */
    val kickTarget: ConfPerson?
        get() = room.people.firstOrNull { it.userId == confirmingKick }

    // ── screen share (§8) ─────────────────────────────────────────────────

    /** Where we are between «не показываю» and «идут пиксели». */
    val share: ConfShare
        get() = confShareState(wantScreen, room.presenting, session.screenShare)

    /** Our 1-based place in the line for the stage; 0 when we are not in it. */
    val queuePosition: Int get() = confQueuePosition(room.queueConns, room.connId)

    /** The one line above the tiles about who is showing what. */
    val stageLine: ConfStageLine
        get() = confStageLine(
            share = share,
            stageName = room.stage?.name.orEmpty(),
            stageIsMine = room.presenting,
            position = queuePosition,
            waiting = room.queue.size,
        )

    val canShare: Boolean get() = confCanShare(room.connected)

    // ── recording (§8) ────────────────────────────────────────────────────

    /** Start, stop, or nothing at all — a member never sees the control. */
    val recordingPress: ConfRecordingPress
        get() = confRecordingPress(room.canModerate, room.recording != null)

    /** Who started the recording that is running, for the notice; blank if none. */
    val recordingBy: String get() = room.recording?.startedBy.orEmpty()

    fun rowActions(person: ConfPerson): ConfRowActions =
        confRowActions(person, room.meId, room.canModerate)

    /** Whether this row gets the local volume controls — media, not roster. */
    fun hasLocalAudio(person: ConfPerson): Boolean =
        confHasLocalAudio(person.userId, room.meId, session.peers.map { it.identity })

    // ── the minimised call (§9) ───────────────────────────────────────────

    /** What the bar over the rest of the app says, or null for no bar. */
    val mini: ConfMiniLine?
        get() = confMiniLine(session.status, session.reconnecting, roomOnScreen)

    /** How many other people are in it — ourselves excluded. */
    val miniOthers: Int get() = confMiniOthers(session.peers)

    /** The bar's microphone button, answering exactly as the toolbar's does. */
    val miniMicEnabled: Boolean get() = confMiniMicEnabled(session.status, session.forceMuted)
}

/**
 * The room screen's state (#2896 §5, §6; web `ConferenceRoom.vue` + `useConfRoom`).
 *
 * It owns the room socket and nothing else that outlives the screen: the call
 * itself lives in [ConferenceEngine]. The two halves are wired here, in one
 * direction each — the roster's force-mute goes down into the engine, and the
 * engine's device state goes up into the roster — because they are the two
 * places where a disagreement between the socket and the SFU would show as a
 * lie about who can be heard.
 *
 * It holds no media knowledge and no `io.livekit` import. Every decision it
 * looks like it is making is a pure function in `util/`.
 */
class ConferenceRoomViewModel(
    private val socket: ConferenceRoomSocket = ConferenceRoomSocket(),
    private val repo: ConferenceRepository = ConferenceRepository(),
) : ViewModel() {
    private val _state = MutableStateFlow(ConferenceRoomUiState())
    val state: StateFlow<ConferenceRoomUiState> = _state.asStateFlow()

    private var conferenceId: String = ""

    /** What we last told the room, so an unchanged snapshot sends nothing. */
    private var reported: Pair<Boolean, Boolean>? = null

    /**
     * Keeps our hold on the stage alive while the capture runs (§8).
     *
     * A job on the model's own scope rather than an effect in the composable,
     * unlike the web's `watch`: sharing a screen means this app is *not* the
     * thing on screen, so a heartbeat tied to the room's composition would stop
     * at the very moment it matters and the server would collect the stage out
     * from under a presenter who is still presenting.
     */
    private var stageBeat: Job? = null

    /**
     * Whether the capture was running at the previous snapshot — the only way to
     * tell «Android stopped it» from «it never started».
     */
    private var wasCapturing = false

    /**
     * The TTL the running heartbeat was built for, so a restart is only paid when
     * the server changes its mind about it.
     */
    private var beatTtl = 0L

    // Both declared above `init` and not beside the code that uses them:
    // `viewModelScope` dispatches on `Main.immediate`, so the collectors below
    // run their first snapshot *during* construction — and an initializer that
    // came later would clobber what that first pass wrote.
    init {
        viewModelScope.launch {
            ConferenceEngine.session.collect { session ->
                _state.update {
                    it.copy(session = session, lastSpeaker = confLastSpeaker(session.peers, it.lastSpeaker))
                }
                // The room paints a muted badge before the first audio packet
                // arrives, so it has to hear about our devices from us. Sent on
                // change only: a snapshot per SDK event would be a frame every
                // time somebody's connection quality wobbles.
                val now = session.mic to session.cam
                if (reported != now) {
                    reported = now
                    socket.setMedia(session.mic, session.cam)
                }
                onShareChanged()
            }
        }
        viewModelScope.launch {
            socket.state.collect { room ->
                _state.update { it.copy(room = room) }
                // The one place a force-mute arrives: the SFU has no opinion
                // about who may speak, so a client waiting for a track event
                // would keep transmitting after being silenced.
                ConferenceEngine.applyForceMute(room.forceMuted)
                onShareChanged()
            }
        }
    }

    /**
     * Reconcile the two halves of the stage after either of them moved.
     *
     * Three things can part company and each of them is somebody's bad call:
     *
     *  - the stage went elsewhere while we were capturing — a host preempted us,
     *    and the capture stops rather than publishing into a stage that is not
     *    ours (two screens at once is what the queue exists to prevent);
     *  - the capture stopped without us asking — Android's own «Остановить» in
     *    the cast chip, which tells the server nothing, so we hand the stage back
     *    before the TTL parks the queue behind a share that ended;
     *  - the stage is ours and the capture is running — keep the hold alive.
     */
    private fun onShareChanged() {
        val s = _state.value
        if (confSharePreempted(s.session.screenShare, s.room.presenting, s.room.stateSeq, s.askedStageAt)) {
            _state.update { it.copy(wantScreen = false, autoCapture = false) }
            ConferenceEngine.setScreenShare(false)
            beat(false, s.room.stageTtlMs)
            return
        }
        if (s.wantScreen && s.room.presenting && !s.session.screenShare && wasCapturing) {
            wasCapturing = false
            _state.update { it.copy(wantScreen = false, autoCapture = false) }
            socket.releaseScreen()
            beat(false, s.room.stageTtlMs)
            return
        }
        wasCapturing = s.session.screenShare
        beat(s.session.screenShare && s.room.presenting, s.room.stageTtlMs)
    }

    /** The heartbeat, on or off. Restarting it on an unchanged TTL is a no-op. */
    private fun beat(on: Boolean, ttlMs: Long) {
        if (!on) {
            stageBeat?.cancel()
            stageBeat = null
            beatTtl = 0
            return
        }
        if (stageBeat?.isActive == true && beatTtl == ttlMs) return
        stageBeat?.cancel()
        beatTtl = ttlMs
        val every = confShareHeartbeatMs(ttlMs)
        stageBeat = viewModelScope.launch {
            while (isActive) {
                delay(every)
                socket.refreshScreen()
            }
        }
    }

    /**
     * Take the media half of a seat the lobby has already taken, and open the
     * room socket alongside it.
     *
     * Idempotent on purpose: this is called from a `LaunchedEffect` that re-runs
     * whenever a permission answer changes the grant, and a second `join` for a
     * call already connecting would tear down the first one's token.
     */
    fun enter(conferenceId: String, grant: ConfMediaGrant, title: String = "") {
        val fresh = this.conferenceId != conferenceId
        this.conferenceId = conferenceId
        _state.update { it.copy(grant = grant) }
        ConferenceEngine.join(conferenceId, ConfMediaWant(mic = grant.mic, cam = false), grant, title)
        if (fresh) {
            reported = null
            socket.open(conferenceId)
        }
    }

    /**
     * The room screen appeared or went away (#2896 §9).
     *
     * Only bookkeeping for the bar — nothing here touches the call, which is the
     * entire point of §9: navigating off a meeting minimises it instead of
     * hanging up.
     */
    fun onRoomShown() = _state.update { it.copy(roomOnScreen = true) }

    fun onRoomHidden() = _state.update { it.copy(roomOnScreen = false) }

    /**
     * Hang up from the minimised bar (§9).
     *
     * Drops the seat as well as the media. The room screen's own button leaves
     * that to the lobby — it is on screen, it owns membership and it re-reads the
     * roster afterwards — but a bar pressed from the board has no lobby behind
     * it, and media alone would leave the room counting somebody who is gone.
     *
     * The media goes first and does not wait for the request: the microphone
     * should stop on the press, not on the reply, and a `leave` that fails still
     * leaves a call the user has quit.
     */
    fun hangUp() {
        val id = conferenceId
        exit()
        if (id.isBlank()) return
        viewModelScope.launch { runCatching { repo.leave(id) } }
    }

    /**
     * Leave the call's media and the room's bookkeeping.
     *
     * Called both from the hang-up button and from the screen going away, because
     * until §9's minimised bar exists there is nowhere for a call to live without
     * a screen — an invisible meeting still holding the microphone is worse than
     * one that ends when you navigate off it.
     */
    fun exit() {
        // Before the engine goes: a projection outliving the room would keep the
        // system's cast chip up for a call that has ended.
        beat(false, 0)
        wasCapturing = false
        ConferenceEngine.setScreenShare(false)
        ConferenceEngine.leave()
        socket.close()
        conferenceId = ""
        reported = null
        _state.update {
            it.copy(
                lastSpeaker = "",
                stageOnly = false,
                routeMenu = false,
                panelOpen = false,
                confirmingKick = "",
                wantScreen = false,
                askedStageAt = 0,
                autoCapture = false,
                recordingError = null,
                recordingBusy = false,
            )
        }
    }

    /** Retry a failed connection — the banner's «Повторить». */
    fun retry() {
        if (conferenceId.isBlank()) return
        ConferenceEngine.join(conferenceId, ConfMediaWant(mic = _state.value.grant.mic, cam = false), _state.value.grant)
        if (!_state.value.room.connected) {
            reported = null
            socket.open(conferenceId)
        }
    }

    fun toggleMic() {
        val s = _state.value
        if (!s.controls.micEnabled) return
        ConferenceEngine.setMic(!s.session.mic)
    }

    /**
     * Turn the camera on or off. [granted] carries the answer to a permission
     * dialog the screen may have just shown, so a first «включить камеру» works
     * on the same press rather than needing a second one.
     */
    fun setCam(on: Boolean, granted: Boolean = _state.value.grant.cam) {
        _state.update { it.copy(grant = it.grant.copy(cam = granted)) }
        ConferenceEngine.updateGrant(_state.value.grant)
        ConferenceEngine.setCam(on && granted)
    }

    /** Microphone permission answered after the fact — publish if it was a yes. */
    fun micGranted(granted: Boolean) {
        _state.update { it.copy(grant = it.grant.copy(mic = granted)) }
        ConferenceEngine.updateGrant(_state.value.grant)
        if (granted) ConferenceEngine.setMic(true)
    }

    fun switchCamera() = ConferenceEngine.switchCamera()

    fun selectRoute(route: ConfAudioRoute) {
        ConferenceEngine.selectRoute(route)
        _state.update { it.copy(routeMenu = false) }
    }

    fun openRouteMenu() = _state.update { it.copy(routeMenu = true) }

    fun closeRouteMenu() = _state.update { it.copy(routeMenu = false) }

    fun toggleStageOnly() = _state.update { it.copy(stageOnly = !it.stageOnly) }

    // ── participants and moderation (#2896 §6) ────────────────────────────

    fun openPanel() = _state.update { it.copy(panelOpen = true) }

    fun closePanel() = _state.update { it.copy(panelOpen = false, confirmingKick = "") }

    /**
     * Raise or lower our hand.
     *
     * Read off the roster rather than off a local flag: the room is what orders
     * the queue, and a screen keeping its own answer would show a lowered hand
     * that is still holding a place in it.
     */
    fun toggleHand() = socket.raiseHand(!_state.value.room.handUp)

    fun forceMute(userId: String, muted: Boolean) = socket.forceMute(userId, muted)

    fun askKick(userId: String) = _state.update { it.copy(confirmingKick = userId) }

    fun cancelKick() = _state.update { it.copy(confirmingKick = "") }

    /** Behind a confirmation: it is visible to the whole room and the person
     *  who did it cannot take it back. */
    fun confirmKick() {
        val target = _state.value.confirmingKick
        _state.update { it.copy(confirmingKick = "") }
        if (target.isNotBlank()) socket.kick(target)
    }

    // ── screen share (#2896 §8) ───────────────────────────────────────────

    /**
     * The share button, in every state it can be pressed in.
     *
     * Returns what the screen still has to do, because exactly one branch needs
     * something this model may not do itself: [ConfSharePress.CAPTURE] ends in
     * Android's consent dialog, which is an Activity result and belongs to the
     * composable. Everything else — asking, cancelling, stopping — is finished
     * here. Null means the press did nothing: the room socket is down, and the
     * stage is arbitrated over it.
     */
    fun pressShare(): ConfSharePress? {
        val s = _state.value
        if (!s.canShare) return null
        return when (val press = confSharePress(s.share)) {
            ConfSharePress.REQUEST -> {
                _state.update {
                    it.copy(
                        wantScreen = true,
                        askedStageAt = it.room.stateSeq,
                        autoCapture = confShareAutoCapture(it.room.stage != null),
                    )
                }
                socket.requestScreen()
                press
            }

            ConfSharePress.CANCEL -> {
                _state.update { it.copy(wantScreen = false, autoCapture = false) }
                socket.releaseScreen()
                press
            }

            ConfSharePress.STOP -> {
                stopSharing()
                press
            }

            // The stage is ours and the dialog is the screen's to open. The flag
            // is cleared either way: a consent that was auto-followed once must
            // not be auto-followed again when the user dismisses it.
            ConfSharePress.CAPTURE -> {
                _state.update { it.copy(autoCapture = false) }
                press
            }
        }
    }

    /** The screen has taken the auto-follow; it fires once per request. */
    fun consumeAutoCapture(): Boolean {
        val armed = _state.value.autoCapture && _state.value.share == ConfShare.MY_TURN
        if (armed) _state.update { it.copy(autoCapture = false) }
        return armed
    }

    /**
     * Android answered the consent dialog.
     *
     * A dismissal gives the stage straight back rather than holding it: the
     * queue behind a presenter who never starts is the one thing this whole
     * arbitration exists to prevent, and the server would only free it after the
     * TTL — half a minute of everybody waiting on a dialog somebody closed.
     */
    fun onCaptureConsent(permission: android.content.Intent?) {
        if (permission == null) {
            _state.update { it.copy(wantScreen = false, autoCapture = false) }
            socket.releaseScreen()
            return
        }
        ConferenceEngine.setScreenShare(true, permission)
    }

    private fun stopSharing() {
        _state.update { it.copy(wantScreen = false, autoCapture = false) }
        ConferenceEngine.setScreenShare(false)
        socket.releaseScreen()
    }

    // ── recording (#2896 §8) ──────────────────────────────────────────────

    /**
     * Start or stop the server-side recording.
     *
     * Nothing is captured here: an egress worker joins the call and writes the
     * file. What this owes the user is the refusal — an install without that
     * worker answers every start with a 502, and «сервис недоступен» is a fact
     * about the server rather than something to press again.
     *
     * The button never flips itself. `running` comes from the room snapshot, so
     * the red dot appears when the worker has actually joined — which is also
     * how everyone who did *not* press it finds out.
     */
    fun toggleRecording() {
        val s = _state.value
        val press = s.recordingPress
        if (press == ConfRecordingPress.NONE || s.recordingBusy || conferenceId.isBlank()) return
        _state.update { it.copy(recordingBusy = true, recordingError = null) }
        viewModelScope.launch {
            val result = runCatching {
                if (press == ConfRecordingPress.STOP) {
                    repo.stopRecording(conferenceId)
                } else {
                    repo.startRecording(conferenceId)
                }
            }
            _state.update {
                it.copy(
                    recordingBusy = false,
                    recordingError = result.exceptionOrNull()?.let(::confRecordingFailure),
                )
            }
        }
    }

    fun clearRecordingError() = _state.update { it.copy(recordingError = null) }

    fun setPeerVolume(userId: String, volume: Float) = ConferenceEngine.setPeerVolume(userId, volume)

    fun toggleLocalMute(userId: String) =
        ConferenceEngine.setPeerLocalMuted(userId, userId !in _state.value.session.localMuted)

    fun clearDenied() = socket.clearDenied()

    /** Whether the room threw us out or ended under us — the screen leaves on it. */
    val roomEnd: ConfRoomEnd get() = _state.value.room.ended

    /**
     * The shell itself is going away — end the call with it (#2896 §9).
     *
     * Since §9 nothing else does: leaving the room screen minimises the meeting
     * instead of hanging up, so this is the last thing standing between a swiped
     * -away app and a microphone still publishing behind a notification nobody
     * connects to a screen they closed.
     */
    override fun onCleared() {
        exit()
    }
}
