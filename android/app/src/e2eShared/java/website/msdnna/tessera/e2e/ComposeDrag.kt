package website.msdnna.tessera.e2e

import android.os.SystemClock
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
 * A drop point meaning "past every card in this column" — where a card is released
 * to land at the end.
 *
 * Deliberately measured from the column rather than from the cards. A target
 * computed off the resting layout has to predict how the column re-lays-out once
 * the dragged card collapses to zero height, and that prediction does not hold on
 * a device: aimed at the footer tile minus the collapsed card's height, the finger
 * came back down **on the card above**, and the board did as it was told and nested
 * the card as its subtask instead of reordering it (#2711).
 *
 * The empty column body below the footer needs no prediction. Whatever the column
 * does while a card is in flight it can only pull its contents *up*, so a point
 * past the footer stays past every card — and it stays inside the node that
 * registers the column's drop bounds, which spans the full board height rather
 * than just the painted tile (`BoardViews.kt`, same modifier chain as the tag).
 */
fun ComposeContentTestRule.columnEndTarget(columnId: String): Offset {
    val column = windowRect(TestTags.boardColumn(columnId))
    val footer = windowRect(TestTags.columnAddTask(columnId))
    val gap = with(density) { CARD_GAP.toPx() }
    return Offset(column.center.x, minOf(footer.bottom + gap, column.bottom - 1f))
}

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
 * **The hold has to be stamped *and* served.** `advanceEventTime` moves the
 * timestamps the injected events carry, which is what drives the gesture's clock
 * on the JVM tier — but on a device the long-press timer runs on the real one, so
 * a press and a move injected in the same breath arrive inside the timeout and
 * read as a fling. Hence [holdPastLongPress], which spends the wait in real time
 * on a device and skips it under Robolectric, where wall-clock time buys nothing.
 *
 * The whole gesture stays inside a single `performTouchInput`, and nothing in it
 * touches the semantics tree. Every Compose query synchronises on idleness first,
 * and a board with a card in flight is not idle — reading the tree mid-drag makes
 * the spec die of `ComposeNotIdleException` instead of reporting what the drag
 * did.
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
        advanceEventTime(viewConfiguration.longPressTimeoutMillis + LONG_PRESS_MARGIN_MS)
        holdPastLongPress(viewConfiguration.longPressTimeoutMillis + LONG_PRESS_MARGIN_MS)
        repeat(DRAG_STEPS) {
            advanceEventTime(FRAME_MS)
            moveBy(step)
        }
        up()
    }
}

/** Margin past `longPressTimeoutMillis` so the hold never lands on the boundary. */
private const val LONG_PRESS_MARGIN_MS = 100L

/** Event spacing between move steps — roughly one frame at 60Hz. */
private const val FRAME_MS = 16L

/**
 * Lets [millis] of real time pass mid-gesture, but only where it is the clock
 * that matters.
 *
 * On a device the pressed finger is already with the app — the `down` event went
 * out as it was issued — and the long-press timer that arms the drag is counting
 * real milliseconds on the main thread. Blocking the instrumentation thread is
 * what gives that timer its chance to fire, and it is exactly what a thumb resting
 * on a card does. Under Robolectric the same wait would be dead wall-clock: there
 * the gesture runs on the injected clock, which `advanceEventTime` has already
 * moved past the timeout.
 */
private fun holdPastLongPress(millis: Long) {
    if (E2eBackend.onDevice) SystemClock.sleep(millis)
}
