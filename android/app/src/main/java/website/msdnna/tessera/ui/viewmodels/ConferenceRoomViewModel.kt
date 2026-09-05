package website.msdnna.tessera.ui.viewmodels

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import website.msdnna.tessera.data.conference.ConfPeer
import website.msdnna.tessera.data.conference.ConfSession
import website.msdnna.tessera.data.conference.ConferenceEngine
import website.msdnna.tessera.util.ConfAudioRoute
import website.msdnna.tessera.util.ConfBanner
import website.msdnna.tessera.util.ConfControls
import website.msdnna.tessera.util.ConfMediaGrant
import website.msdnna.tessera.util.ConfMediaWant
import website.msdnna.tessera.util.ConfStage
import website.msdnna.tessera.util.confBanner
import website.msdnna.tessera.util.confControls
import website.msdnna.tessera.util.confLastSpeaker
import website.msdnna.tessera.util.confStage

data class ConferenceRoomUiState(
    val session: ConfSession = ConfSession(),
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
) {
    val stage: ConfStage<ConfPeer> get() = confStage(session.peers, lastSpeaker)

    val banner: ConfBanner
        get() = confBanner(session.status, session.reason, session.reconnecting, grant.mic)

    val controls: ConfControls
        get() = confControls(session.status, session.mic, session.cam, session.forceMuted)
}

/**
 * The room screen's state (#2896 §5, web `ConferenceRoom.vue`).
 *
 * Thin by design: the call itself lives in [ConferenceEngine], which outlives
 * this screen, so all this owns is what the screen adds on top — the remembered
 * speaker, the folded-away chrome and the routing menu.
 *
 * It holds no media knowledge and no `io.livekit` import. Every decision it
 * looks like it is making is a pure function in `util/ConferenceRoom.kt`.
 */
class ConferenceRoomViewModel : ViewModel() {
    private val _state = MutableStateFlow(ConferenceRoomUiState())
    val state: StateFlow<ConferenceRoomUiState> = _state.asStateFlow()

    private var conferenceId: String = ""

    init {
        viewModelScope.launch {
            ConferenceEngine.session.collect { session ->
                _state.update {
                    it.copy(session = session, lastSpeaker = confLastSpeaker(session.peers, it.lastSpeaker))
                }
            }
        }
    }

    /**
     * Take the media half of a seat the lobby has already taken.
     *
     * Idempotent on purpose: this is called from a `LaunchedEffect` that re-runs
     * whenever a permission answer changes the grant, and a second `join` for a
     * call already connecting would tear down the first one's token.
     */
    fun enter(conferenceId: String, grant: ConfMediaGrant) {
        this.conferenceId = conferenceId
        _state.update { it.copy(grant = grant) }
        ConferenceEngine.join(conferenceId, ConfMediaWant(mic = grant.mic, cam = false), grant)
    }

    /**
     * Leave the call's media.
     *
     * Called both from the hang-up button and from the screen going away, because
     * until §9's minimised bar exists there is nowhere for a call to live without
     * a screen — an invisible meeting still holding the microphone is worse than
     * one that ends when you navigate off it.
     */
    fun exit() {
        ConferenceEngine.leave()
        _state.update { it.copy(lastSpeaker = "", stageOnly = false, routeMenu = false) }
    }

    /** Retry a failed connection — the banner's «Повторить». */
    fun retry() {
        if (conferenceId.isBlank()) return
        ConferenceEngine.join(conferenceId, ConfMediaWant(mic = _state.value.grant.mic, cam = false), _state.value.grant)
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
}
