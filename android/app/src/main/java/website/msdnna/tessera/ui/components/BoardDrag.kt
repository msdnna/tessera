package website.msdnna.tessera.ui.components

import androidx.compose.foundation.gestures.detectDragGesturesAfterLongPress
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.width
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.composed
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.layout.layout
import androidx.compose.ui.layout.onGloballyPositioned
import androidx.compose.ui.layout.positionInWindow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.IntOffset
import kotlin.math.roundToInt
import website.msdnna.tessera.data.model.Task
import website.msdnna.tessera.ui.viewmodels.BoardUiState
import website.msdnna.tessera.ui.viewmodels.BoardViewModel

/**
 * Drag state for kanban cards. Tracks the picked-up card, the live pointer
 * position (window coords) and the registered bounds of columns + cards so a
 * drop can be resolved to a target column and insertion slot.
 */
class BoardDragState {
    var dragging by mutableStateOf<Task?>(null)
        private set

    /** The column being dragged (header long-press), separate from card drag. */
    var draggingColumn by mutableStateOf<website.msdnna.tessera.data.model.BoardColumn?>(null)
        private set
    var pointer by mutableStateOf(Offset.Zero)
    private var grabOffset = Offset.Zero
    private var startPointer = Offset.Zero

    /** True once the finger has travelled past a threshold — so a bare long-press
     *  (press + release, no movement) is NOT treated as a drop (which would, e.g.,
     *  detach a subtask just for tapping-and-holding it). */
    var movedFar = false
        private set

    /** Window position of the board container, so window-space pointer coords can
     *  be expressed in the overlay's local space. */
    var rootOffset = Offset.Zero

    val columnBounds = mutableMapOf<String, Rect>()
    val cardBounds = mutableMapOf<String, Rect>()

    fun start(task: Task, cardOrigin: Offset, localGrab: Offset) {
        dragging = task
        grabOffset = localGrab
        pointer = cardOrigin + localGrab
        startPointer = pointer
        movedFar = false
    }

    fun drag(delta: Offset) {
        pointer += delta
        if ((pointer - startPointer).getDistance() > MOVE_THRESHOLD_PX) movedFar = true
    }

    /** Ghost top-left in the overlay's local coords, tracking the finger. */
    val overlayTopLeft: Offset get() = pointer - grabOffset - rootOffset

    fun cancel() {
        dragging = null
        movedFar = false
    }

    fun startColumn(column: website.msdnna.tessera.data.model.BoardColumn, origin: Offset, localGrab: Offset) {
        draggingColumn = column
        grabOffset = localGrab
        pointer = origin + localGrab
        startPointer = pointer
        movedFar = false
    }

    fun cancelColumn() {
        draggingColumn = null
        movedFar = false
    }

    /** Resolves the pointer to a column insertion slot → (beforeId, afterId) — the
     *  columns that should sit just left/right of the dropped one. Returns null
     *  when the slot is the column's CURRENT position (no move) so callers draw no
     *  indicator and skip the API call. */
    fun resolveColumnDrop(orderedIds: List<String>): Pair<String?, String?>? {
        val col = draggingColumn ?: return null
        val cols = orderedIds.filter { it != col.id && columnBounds[it] != null }
        val firstRight = cols.firstOrNull { pointer.x < columnBounds.getValue(it).center.x }
        val before = cols.takeWhile { it != firstRight }.lastOrNull()
        // Same slot it's already in → no move (the dragged column is excluded from
        // `cols`, so its original neighbours come out as before/after).
        val idx = orderedIds.indexOf(col.id)
        if (before == orderedIds.getOrNull(idx - 1) && firstRight == orderedIds.getOrNull(idx + 1)) return null
        return before to firstRight
    }

    private companion object {
        const val MOVE_THRESHOLD_PX = 24f
    }

    /**
     * Resolves the current pointer to a drop:
     *  - [Drop.Nest] when the pointer is over another top-level card's body →
     *    the dragged task becomes that card's subtask;
     *  - [Drop.ToColumn] otherwise → insert as a top-level card in that column
     *    between before/after.
     * Returns null when the pointer isn't over any column.
     */
    fun resolveDrop(tasksIn: (String) -> List<Task>, subtasksOf: (String) -> List<Task>): Drop? {
        val task = dragging ?: return null
        // A SUBTASK only reorders within its OWN parent — dragging never detaches
        // it (detach is a modal / context-menu action). It can't escape the parent.
        val parentId = task.parentId
        if (parentId != null) {
            val sibs = subtasksOf(parentId).filter { it.id != task.id && cardBounds[it.id] != null }
            val (before, after) = slotAmong(sibs)
            return Drop.Nest(parentId, before, after)
        }
        val col = columnBounds.entries.firstOrNull { it.value.contains(pointer) }?.key ?: return null
        val cards = tasksIn(col).filter { it.id != task.id }
        // A top-level card over another card's body or its subtask span → attach
        // as that card's subtask at the pointed slot.
        val over = cards.firstOrNull { c ->
            cardBounds[c.id]?.contains(pointer) == true || subtaskSpan(c.id, subtasksOf)?.contains(pointer.y) == true
        }
        if (over != null) {
            val sibs = subtasksOf(over.id).filter { cardBounds[it.id] != null }
            val (before, after) = slotAmong(sibs)
            return Drop.Nest(over.id, before, after)
        }
        // Else a top-level insertion slot.
        val (before, after) = slotAmong(cards.filter { cardBounds[it.id] != null })
        return Drop.ToColumn(col, before, after)
    }

    /** (beforeId, afterId) for the pointer's vertical slot among [items]. */
    private fun slotAmong(items: List<Task>): Pair<String?, String?> {
        val firstBelow = items.firstOrNull { pointer.y < cardBounds.getValue(it.id).center.y }
        return items.takeWhile { it.id != firstBelow?.id }.lastOrNull()?.id to firstBelow?.id
    }

    /** The vertical span covered by a parent's (bounded) subtasks, or null. */
    private fun subtaskSpan(parentId: String, subtasksOf: (String) -> List<Task>): ClosedFloatingPointRange<Float>? {
        val subs = subtasksOf(parentId).filter { cardBounds[it.id] != null }
        if (subs.isEmpty()) return null
        return subs.minOf { cardBounds.getValue(it.id).top }..subs.maxOf { cardBounds.getValue(it.id).bottom }
    }
}

/** The outcome of resolving a drag drop. */
sealed interface Drop {
    /** Insert as a top-level card in [columnId], between [beforeId]/[afterId]. */
    data class ToColumn(val columnId: String, val beforeId: String?, val afterId: String?) : Drop

    /** Make the dragged task a subtask of [parentId], optionally positioned
     *  between sibling [beforeId]/[afterId] (both null = append to the end). */
    data class Nest(val parentId: String, val beforeId: String?, val afterId: String?) : Drop
}

@Composable
fun rememberBoardDragState(): BoardDragState = remember { BoardDragState() }

/** Registers a card's window bounds and arms long-press drag on it. [onDrop]
 *  runs at release while the drag state is still populated. Read via
 *  rememberUpdatedState so the recomposition-surviving pointerInput never fires a
 *  stale closure (which would resolve against an out-of-date board). */
fun Modifier.draggableCard(state: BoardDragState, task: Task, onDrop: () -> Unit): Modifier = composed {
    val latestOnDrop by rememberUpdatedState(onDrop)
    // Cards live in a virtualised LazyColumn — when one scrolls off and is disposed,
    // drop its now-stale bounds so drop resolution only ever sees on-screen cards
    // (a lingering off-screen Rect would skew slotAmong / nest hit-testing).
    DisposableEffect(task.id) {
        onDispose { state.cardBounds.remove(task.id) }
    }
    this
        .onGloballyPositioned { coords ->
            val tl = coords.positionInWindow()
            state.cardBounds[task.id] = Rect(tl.x, tl.y, tl.x + coords.size.width, tl.y + coords.size.height)
        }
        .pointerInput(task.id) {
            detectDragGesturesAfterLongPress(
                onDragStart = { local ->
                    val origin = state.cardBounds[task.id]?.topLeft ?: Offset.Zero
                    state.start(task, origin, local)
                },
                onDrag = { change, delta ->
                    change.consume()
                    state.drag(delta)
                },
                onDragEnd = {
                    latestOnDrop()
                    state.cancel()
                },
                onDragCancel = { state.cancel() },
            )
        }
}

/** Registers a column's window bounds so drops can target it. */
fun Modifier.dropColumn(state: BoardDragState, columnId: String): Modifier =
    this.onGloballyPositioned { coords ->
        val tl = coords.positionInWindow()
        state.columnBounds[columnId] = Rect(tl.x, tl.y, tl.x + coords.size.width, tl.y + coords.size.height)
    }

/** A full card ghost of the dragged task, tracking the finger. Suppressed for
 *  subtasks (reorder is within the parent's small list, where a full-card ghost
 *  just obscures the slot) — there the faded preview + slide carry the feedback. */
@Composable
fun BoardDragOverlay(state: BoardDragState, width: Dp, boardState: BoardUiState, vm: BoardViewModel) {
    val task = state.dragging ?: return
    if (task.parentId != null) return
    Box(Modifier.fillMaxSize()) {
        Box(
            Modifier
                .offset { IntOffset(state.overlayTopLeft.x.roundToInt(), state.overlayTopLeft.y.roundToInt()) }
                .width(width)
                .graphicsLayer {
                    alpha = 0.97f
                    scaleX = 1.03f
                    scaleY = 1.03f
                },
        ) {
            // Compact: just the card body (no subtasks / "+ create subtask"), so
            // the ghost doesn't carry that trailing area and its heavy shadow.
            TaskCard(task = task, state = boardState, vm = vm, onOpen = {}, compact = true)
        }
    }
}

/**
 * Collapses the dragged item's slot while it's in flight: it's hidden (alpha 0)
 * and reports zero height so the layout reflows around the placeholder gap — but
 * stays composed and measured, so its long-press gesture node is NOT disposed
 * (disposing it would cancel the drag).
 *
 * The modifiers are ALWAYS applied (only the params switch) so dropping doesn't
 * add/remove modifier nodes — a structural change there can leave a card's draw
 * un-invalidated (the frame/accent vanishing on a same-spot drop).
 */
fun Modifier.dragCollapse(active: Boolean): Modifier = this
    .graphicsLayer { alpha = if (active) 0f else 1f }
    .layout { measurable, constraints ->
        val placeable = measurable.measure(constraints)
        layout(placeable.width, if (active) 0 else placeable.height) { placeable.place(0, 0) }
    }
