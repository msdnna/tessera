package website.msdnna.tessera.ui.components

import androidx.compose.foundation.gestures.detectDragGesturesAfterLongPress
import androidx.compose.runtime.getValue
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.ui.Modifier
import androidx.compose.ui.composed
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.input.pointer.pointerInput
import website.msdnna.tessera.data.model.BoardColumn

/** Arms long-press drag on a column header. [onDrop] runs at release. The
 *  pointerInput is keyed by column id (so it survives recomposition), so [onDrop]
 *  is read via rememberUpdatedState — otherwise it would capture a STALE closure
 *  (e.g. the pre-reorder column order) and the next drag would resolve wrongly. */
fun Modifier.draggableColumn(state: BoardDragState, column: BoardColumn, onDrop: () -> Unit): Modifier = composed {
    val latestOnDrop by rememberUpdatedState(onDrop)
    this.pointerInput(column.id) {
        detectDragGesturesAfterLongPress(
            onDragStart = { local ->
                val origin = state.columnBounds[column.id]?.topLeft ?: Offset.Zero
                state.startColumn(column, origin, local)
            },
            onDrag = { change, dragAmount ->
                change.consume()
                state.drag(dragAmount)
            },
            onDragEnd = {
                latestOnDrop()
                state.cancelColumn()
            },
            onDragCancel = { state.cancelColumn() },
        )
    }
}

/** Dims a column while it's the one being dragged (it stays in place as a
 *  placeholder; a full ghost clone tracks the finger). Always applied so a drop
 *  doesn't add/remove modifier nodes. */
fun Modifier.dragDim(active: Boolean): Modifier = this.graphicsLayer { alpha = if (active) 0.3f else 1f }
