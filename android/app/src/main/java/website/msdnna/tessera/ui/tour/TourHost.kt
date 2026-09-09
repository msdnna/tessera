package website.msdnna.tessera.ui.tour

import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.Stable
import androidx.compose.runtime.compositionLocalOf
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.staticCompositionLocalOf
import website.msdnna.tessera.ui.components.TourOverlay
import website.msdnna.tessera.util.AdvanceOn
import website.msdnna.tessera.util.TourSnapshot

/**
 * Where the Get Started guide is *drawn* (#2860).
 *
 * One overlay in the app shell would have been enough on the web, where everything
 * lives in one document. Here the task form is a `Dialog`, i.e. its own window
 * stacked above the activity's: an overlay mounted in `MainScreen` is painted
 * behind it, so the `tm-*` steps would ask the user to fill in a field they cannot
 * see. So each surface that can be on top hosts an overlay of its own and claims a
 * layer while it is composed; only the topmost claimant draws.
 *
 * The registry of anchors stays shared — a `Dialog` is a sub-composition, so the
 * CompositionLocals reach into it, and both windows span the display, which keeps
 * the window coordinates the anchors report comparable.
 */
object TourLayer {
    /** The app shell: the sidebar, the board. */
    const val SHELL = 0

    /** The task form, on top of it. */
    const val TASK_MODAL = 10
}

@Stable
class TourLayers {
    private val claimed = mutableStateListOf<Int>()

    val top: Int get() = claimed.maxOrNull() ?: TourLayer.SHELL

    fun claim(layer: Int) {
        claimed.add(layer)
    }

    fun release(layer: Int) {
        claimed.remove(layer)
    }
}

val LocalTourLayers = staticCompositionLocalOf { TourLayers() }

/** What the guide currently shows. Inert by default, so a screen mounted outside
 *  the app shell (a preview, a test) hosts no guide and costs nothing. */
val LocalTourSnapshot = compositionLocalOf { TourSnapshot() }

/** The step's controls, wired to the ViewModel by the shell. */
@Stable
data class TourActions(
    val next: () -> Unit = {},
    val skip: () -> Unit = {},
    val anchorMissing: (String) -> Unit = {},
)

val LocalTourActions = staticCompositionLocalOf { TourActions() }

/**
 * Draws the guide over this surface while it is the topmost one composed. Costs a
 * claim and nothing else when no guide runs.
 */
@Composable
fun TourHost(layer: Int) {
    val snapshot = LocalTourSnapshot.current
    val layers = LocalTourLayers.current
    DisposableEffect(layers, layer) {
        layers.claim(layer)
        onDispose { layers.release(layer) }
    }
    if (!snapshot.active || layers.top != layer) return
    val actions = LocalTourActions.current
    TourOverlay(
        snapshot = snapshot,
        onNext = actions.next,
        onSkip = actions.skip,
        onAnchorMissing = actions.anchorMissing,
    )
}

/**
 * Feeds the engine's watching rules from the anchor registry: how many anchors
 * carry the step's prefix, and where the one it tracks currently sits. Both are
 * read out of the state map, so this recomposes exactly when the registry changes
 * and reports once per change rather than polling.
 *
 * Mounted once by the shell — the registry is shared, so a second reporter in the
 * task form would only duplicate the reports.
 */
@Composable
fun TourReporter(snapshot: TourSnapshot, onReport: (count: Int?, place: String?) -> Unit) {
    val anchors = LocalTourAnchors.current
    val step = snapshot.step
    if (!snapshot.active || step == null) return
    // Prefixes and `<key>:set` keys carry no `{project}`-style tokens, so what the
    // step declares is already what the registry holds — no resolve needed here.
    val count = when (val rule = step.advanceOn) {
        is AdvanceOn.Count -> anchors.count(rule.prefix)
        is AdvanceOn.Set -> anchors.count(rule.key)
        else -> null
    }
    val place = (step.advanceOn as? AdvanceOn.Moved)?.let { anchors.placeOf(it.prefix) }
    LaunchedEffect(step.id, count, place) { onReport(count, place) }
}
