package website.msdnna.tessera.ui.viewmodels

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import website.msdnna.tessera.data.conference.ConfPeer
import website.msdnna.tessera.data.conference.ConfPerson
import website.msdnna.tessera.data.conference.ConfRoomEnd
import website.msdnna.tessera.data.conference.ConfRoomState
import website.msdnna.tessera.data.conference.ConfSession
import website.msdnna.tessera.data.conference.ConferenceEngine
import website.msdnna.tessera.data.conference.ConferenceRoomSocket
import website.msdnna.tessera.util.ConfAudioRoute
import website.msdnna.tessera.util.ConfBanner
import website.msdnna.tessera.util.ConfControls
import website.msdnna.tessera.util.ConfDeniedKind
import website.msdnna.tessera.util.ConfMediaGrant
import website.msdnna.tessera.util.ConfMediaWant
import website.msdnna.tessera.util.ConfRowActions
import website.msdnna.tessera.util.ConfStage
import website.msdnna.tessera.util.confBanner
import website.msdnna.tessera.util.confControls
import website.msdnna.tessera.util.confDeniedKind
import website.msdnna.tessera.util.confHasLocalAudio
import website.msdnna.tessera.util.confLastSpeaker
import website.msdnna.tessera.util.confPanelOrder
import website.msdnna.tessera.util.confRowActions
import website.msdnna.tessera.util.confStage

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

    fun rowActions(person: ConfPerson): ConfRowActions =
        confRowActions(person, room.meId, room.canModerate)

    /** Whether this row gets the local volume controls — media, not roster. */
    fun hasLocalAudio(person: ConfPerson): Boolean =
        confHasLocalAudio(person.userId, room.meId, session.peers.map { it.identity })
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
) : ViewModel() {
    private val _state = MutableStateFlow(ConferenceRoomUiState())
    val state: StateFlow<ConferenceRoomUiState> = _state.asStateFlow()

    private var conferenceId: String = ""

    /** What we last told the room, so an unchanged snapshot sends nothing. */
    private var reported: Pair<Boolean, Boolean>? = null

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
            }
        }
        viewModelScope.launch {
            socket.state.collect { room ->
                _state.update { it.copy(room = room) }
                // The one place a force-mute arrives: the SFU has no opinion
                // about who may speak, so a client waiting for a track event
                // would keep transmitting after being silenced.
                ConferenceEngine.applyForceMute(room.forceMuted)
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
    fun enter(conferenceId: String, grant: ConfMediaGrant) {
        val fresh = this.conferenceId != conferenceId
        this.conferenceId = conferenceId
        _state.update { it.copy(grant = grant) }
        ConferenceEngine.join(conferenceId, ConfMediaWant(mic = grant.mic, cam = false), grant)
        if (fresh) {
            reported = null
            socket.open(conferenceId)
        }
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

    fun setPeerVolume(userId: String, volume: Float) = ConferenceEngine.setPeerVolume(userId, volume)

    fun toggleLocalMute(userId: String) =
        ConferenceEngine.setPeerLocalMuted(userId, userId !in _state.value.session.localMuted)

    fun clearDenied() = socket.clearDenied()

    /** Whether the room threw us out or ended under us — the screen leaves on it. */
    val roomEnd: ConfRoomEnd get() = _state.value.room.ended

    override fun onCleared() {
        socket.close()
    }
}
