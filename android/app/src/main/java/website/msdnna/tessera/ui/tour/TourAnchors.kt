package website.msdnna.tessera.ui.tour

import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.Stable
import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.geometry.isSpecified
import androidx.compose.ui.input.pointer.PointerEventPass
import androidx.compose.ui.input.pointer.PointerInputScope
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.layout.onGloballyPositioned
import androidx.compose.ui.layout.positionOnScreen
import androidx.compose.ui.unit.toSize

/**
 * Where the Get Started guide's arrows point (#2860).
 *
 * The web resolves an anchor by querying the DOM for `data-tour="<key>"`. Compose
 * has no such thing, so an element registers itself: [Modifier.tourAnchor] reports
 * its rect and drops it again when it leaves composition.
 *
 * Rects are in **screen** coordinates, not window ones. Half the anchors the
 * scenario names live outside the activity's window — the «⋯» menus are `Popup`s
 * and the task form is a `Dialog`, each its own window with its own origin — so
 * window coordinates would put an arrow pointing at a menu item somewhere near the
 * top-left of the screen. The overlay subtracts its own screen origin, so both
 * sides speak the same units whichever window they are drawn in.
 *
 * The registry is also how the engine's rules are fed, without a line of reporting
 * code at the call sites:
 *
 *   * `advanceOn.count` — [count] over a key prefix (`project-row:`);
 *   * `advanceOn.set` — a task-modal field registers `<key>:set` while it holds a
 *     value, so mere existence is the signal;
 *   * `advanceOn.moved` — an anchor may carry a `place`, the address of the
 *     container it currently sits in (a card's column, a project row's group), and
 *     the step ends once that address changes.
 */
data class TourAnchor(val rect: Rect, val place: String? = null)

@Stable
class TourAnchors {
    /** Keyed by anchor key. A state map so the overlay redraws when a rect moves. */
    private val entries = mutableStateMapOf<String, TourAnchor>()

    fun put(key: String, anchor: TourAnchor) {
        // onGloballyPositioned fires on every scroll frame; writing an unchanged
        // rect back would recompose the overlay for nothing.
        if (entries[key] != anchor) entries[key] = anchor
    }

    fun remove(key: String) {
        entries.remove(key)
    }

    /**
     * The anchor a step points at. An exact key wins; a key ending in `:` is a
     * prefix — `task-card:` means "the card the user just created" the way the web
     * anchors the first matching node without knowing its id. Ties are broken by
     * position (topmost, then leftmost) rather than by insertion order, which a
     * hash map does not promise anyway.
     */
    fun find(key: String): TourAnchor? {
        if (key.isEmpty()) return null
        entries[key]?.let { return it }
        if (!key.endsWith(":")) return null
        return matching(key).minWithOrNull(byPosition)?.let { entries[it] }
    }

    /** How many anchors currently carry a key starting with [prefix]. */
    fun count(prefix: String): Int = matching(prefix).size

    /** The address of the first anchor under [prefix], or null while none exists. */
    fun placeOf(prefix: String): String? =
        matching(prefix).minWithOrNull(byPosition)?.let { entries[it]?.place }

    private fun matching(prefix: String): List<String> = entries.keys.filter { it.startsWith(prefix) }

    private val byPosition = Comparator<String> { a, b ->
        val ra = entries[a]?.rect
        val rb = entries[b]?.rect
        when {
            ra == null || rb == null -> 0
            ra.top != rb.top -> ra.top.compareTo(rb.top)
            else -> ra.left.compareTo(rb.left)
        }
    }
}

/** The registry in scope. Empty by default, so an anchor outside a running guide
 *  (a preview, a screen mounted by a test) costs one map write and nothing else. */
val LocalTourAnchors = staticCompositionLocalOf { TourAnchors() }

/** Taps on anchored elements, for `advanceOn.tap`. Ignored unless a guide runs. */
val LocalTourTap = staticCompositionLocalOf<(String) -> Unit> { {} }

/**
 * A drop that relocated an anchored element, for `advanceOn.moved`. The value is
 * the address it landed in — a project's new group id, empty at the tree root.
 *
 * Reported straight from the drop handler rather than inferred from the moved
 * row's `place`: a drop into a *collapsed* group unmounts that row, so its new
 * place was never registered and the step hung until the group was expanded by
 * hand (#2860 rework). Ignored unless a guide runs.
 */
val LocalTourMoved = staticCompositionLocalOf<(String) -> Unit> { {} }

/**
 * Registers this element as the anchor [key], reporting its rect and, for the
 * drag-and-drop steps, the [place] it currently sits in.
 *
 * Taps are observed, never consumed: the modifier watches the pointer on the
 * Initial pass and reports a press that ends without turning into a drag, so the
 * element's own `clickable` still fires exactly as it did. That keeps the guide
 * out of the way of the app — the same rule the one-shot [SidebarSpotlight] plays
 * by, and the reason its arrow can point at a row that stays tappable.
 */
@Composable
fun Modifier.tourAnchor(key: String, place: String? = null): Modifier {
    // A blank key is how a shared row says "not a target of the guide" (a nav row
    // outside the scenario, a creator the steps don't name). Registering it would
    // have every such row fight over one entry, rewriting it on each scroll frame.
    if (key.isBlank()) return this
    val anchors = LocalTourAnchors.current
    val onTap = LocalTourTap.current
    DisposableEffect(anchors, key) {
        onDispose { anchors.remove(key) }
    }
    return this
        .onGloballyPositioned { coords ->
            val at = coords.positionOnScreen()
            // Unspecified while the node is laid out but not attached to a window
            // yet; a rect built from NaN would blow up the mask's cutout.
            if (at.isSpecified) anchors.put(key, TourAnchor(Rect(at, coords.size.toSize()), place))
        }
        .pointerInput(key, onTap) { observeTaps { onTap(key) } }
}

/** A tap detector that consumes nothing (see [tourAnchor]). */
private suspend fun PointerInputScope.observeTaps(onTap: () -> Unit) {
    awaitPointerEventScope {
        while (true) {
            val down = awaitPointerEvent(PointerEventPass.Initial).changes.firstOrNull { it.pressed } ?: continue
            val start = down.position
            var dragged = false
            var pressed = true
            while (pressed) {
                val change = awaitPointerEvent(PointerEventPass.Initial).changes.firstOrNull() ?: break
                if ((change.position - start).getDistance() > viewConfiguration.touchSlop) dragged = true
                pressed = change.pressed
            }
            if (!dragged) onTap()
        }
    }
}
