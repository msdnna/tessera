package website.msdnna.tessera.ui.viewmodels

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import website.msdnna.tessera.update.WhatsNewRepository
import website.msdnna.tessera.util.AdvanceOn
import website.msdnna.tessera.util.GET_STARTED
import website.msdnna.tessera.util.TourContext
import website.msdnna.tessera.util.TourEngine
import website.msdnna.tessera.util.TourSnapshot

/**
 * Session state of the Get Started guide (#2860). The decision of *when* a step
 * ends is [TourEngine]'s and is pure; this republishes its snapshot to the overlay
 * and records the outcome.
 *
 * The outcome goes to the shared acknowledgement endpoint (`getstarted:done` /
 * `getstarted:skipped`) rather than to local preferences, which is where the plan
 * put it: the key space is one per user across clients, the web already writes
 * exactly these keys, and a guide walked through there should not greet the same
 * person again here. Nothing else is persisted — the guide is restarted from the
 * sidebar, not resumed.
 */
class TourViewModel(app: Application) : AndroidViewModel(app) {
    private val repo = WhatsNewRepository()

    private val _state = MutableStateFlow(TourSnapshot())
    val state: StateFlow<TourSnapshot> = _state.asStateFlow()

    private val engine = TourEngine(onAck = ::ack)

    private fun publish() {
        _state.value = engine.snapshot()
    }

    private fun ack(key: String) {
        viewModelScope.launch {
            // Offline: the guide is over for this session either way, and the
            // endpoint is idempotent — a retry next session costs nothing.
            runCatching { repo.ack(key) }
        }
    }

    /** «Обучение» in the sidebar: always from the first step, whatever was acked. */
    fun startGuide() {
        engine.start(GET_STARTED)
        publish()
    }

    fun next() {
        engine.next()
        publish()
    }

    fun skip() {
        engine.skip()
        publish()
    }

    fun anchorMissing(stepId: String) {
        engine.anchorMissing(stepId)
        publish()
    }

    /** A tap on an anchored element, reported by `Modifier.tourAnchor`. */
    fun tapped(key: String) {
        if (!engine.active) return
        engine.tapped(key)
        publish()
    }

    /** A drop reported by the sidebar for a [AdvanceOn.Moved] step — the address it
     *  landed in. Reported straight from the drop so a relocation into a collapsed
     *  group (whose row unmounts) still ends the step (#2860 rework). */
    fun moved(place: String) {
        if (!engine.active) return
        engine.located(place)
        publish()
    }

    /** The host reports what the user just created, so the steps that follow point
     *  at *that* row (the project, its board, the group). */
    fun noteCreated(next: TourContext.() -> TourContext) {
        engine.noteCreated(next)
        publish()
    }

    /**
     * What the anchor registry currently holds, for the rules that watch it. Called
     * on every change of the registry while a guide runs; the engine ignores
     * reports that don't concern the step it is on.
     */
    fun report(count: Int?, place: String?) {
        if (!engine.active) return
        when (engine.current?.advanceOn) {
            is AdvanceOn.Count, is AdvanceOn.Set -> count?.let(engine::counted)
            is AdvanceOn.Moved -> engine.located(place)
            else -> return
        }
        publish()
    }
}
