package website.msdnna.tessera.e2e

import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.test.junit4.ComposeContentTestRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onRoot
import androidx.compose.ui.test.performTouchInput
import androidx.compose.ui.unit.dp
import website.msdnna.tessera.ui.TestTags

/** How many move events a drag is split into (see [dragCardTo]). */
private const val DRAG_STEPS = 8

/**
 * The node's rectangle in window coordinates — the same space the board's drag
 * state works in ([website.msdnna.tessera.ui.components.BoardDragState.cardBounds]).
 *
 * Built from `positionInWindow` + `size` rather than `boundsInWindow`, because
 * the latter is clipped to what is visible: a kanban column is deliberately
 * wider than the screen, so its clipped rect would stop at the window edge and a
 * spec aiming at the next column would compute a target inside the current one.
 */
fun ComposeContentTestRule.windowRect(tag: String): Rect {
    val node = onNodeWithTag(tag).fetchSemanticsNode()
    val topLeft = node.positionInWindow
    return Rect(
        topLeft.x,
        topLeft.y,
        topLeft.x + node.size.width,
        topLeft.y + node.size.height,
    )
}

/** Width of the test window in pixels — the limit a pointer may travel to. */
fun ComposeContentTestRule.windowWidthPx(): Float = onRoot().fetchSemanticsNode().size.width.toFloat()

/**
 * How far the column's contents slide up once the card for [taskId] is picked up.
 *
 * A dragged card is not removed from the layout — it stays composed (removing it
 * would dispose its gesture node and cancel the drag) but reports zero height via
 * `dragCollapse`. Everything below it therefore moves up by the card's height plus
 * the 8dp the column puts between cards. A target measured on the resting layout
 * has to be corrected by this, or a drop aimed just under the last card arrives
 * under the column itself, where there is nothing to drop onto.
 */
fun ComposeContentTestRule.collapseShift(taskId: String): Float =
    windowRect(TestTags.taskCard(taskId)).height + with(density) { CARD_GAP.toPx() }

/** Vertical spacing between cards in a column (`BoardViews.kt` LazyColumn). */
private val CARD_GAP = 8.dp

/**
 * Drags the card for [taskId] until the finger sits at [target] (window coords)
 * and releases it there.
 *
 * The board arms dragging with `detectDragGesturesAfterLongPress`
 * (`DND_ARCHITECTURE.md` §3), so a plain `swipe()` never picks the card up at
 * all — it reads as a fling and the spec would report "drag does nothing" for a
 * board that drags fine. Hence the explicit press → hold past the long-press
 * timeout → move → release.
 *
 * The move is split into [DRAG_STEPS] events rather than one jump for two
 * reasons: the drag state only arms `movedFar` once the finger has travelled
 * past its 24px threshold (a single event that starts and ends the movement
 * would be treated as a stray long-press and drop nothing), and the board
 * resolves the hovered slot per frame, which is what a user's drag actually
 * looks like.
 */
fun ComposeContentTestRule.dragCardTo(taskId: String, target: Offset) {
    val tag = TestTags.taskCard(taskId)
    val start = windowRect(tag).center
    val step = (target - start) / DRAG_STEPS.toFloat()
    onNodeWithTag(tag).performTouchInput {
        down(center)
        // The gesture only starts after the long-press timeout has elapsed on the
        // injected event clock; +100ms of margin keeps it off the boundary.
        advanceEventTime(viewConfiguration.longPressTimeoutMillis + 100)
        repeat(DRAG_STEPS) { moveBy(step) }
        up()
    }
}
