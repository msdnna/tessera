package website.msdnna.tessera.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.gestures.detectDragGesturesAfterLongPress
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.composed
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.layout.onGloballyPositioned
import androidx.compose.ui.layout.positionInWindow
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import kotlin.math.roundToInt
import website.msdnna.tessera.ui.theme.RadiusSm
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.accentGradient

/** Whether a draggable sidebar node is a group or a project. */
enum class SbKind { GROUP, PROJECT }

/** A draggable sidebar node flattened into render order, with its tree depth and
 *  its parent container (group parent for a group, owning group for a project). */
data class SbNode(
    val id: String,
    val kind: SbKind,
    val depth: Int,
    val parentId: String?,
    val name: String,
    val icon: String,
    val color: String,
    val iconMode: String = "badge",
)

/** The resolved target of an in-flight sidebar drag. */
data class SbDrop(
    val parentId: String?,
    val beforeId: String?,
    val afterId: String?,
    val depth: Int,
    /** Window-space Y of the insertion line / placeholder. */
    val gapY: Float,
)

/**
 * Drag state for the sidebar tree (groups + projects). Tracks the picked-up node,
 * the live pointer, and the registered window bounds of every row so a drop can be
 * projected to a target container + depth + slot (dnd-kit style).
 */
class SidebarDragState {
    var dragging by mutableStateOf<SbNode?>(null)
        private set
    var pointer by mutableStateOf(Offset.Zero)
    private var grabOffset = Offset.Zero
    private var startPointer = Offset.Zero
    var movedFar by mutableStateOf(false)
        private set
    var rowHeight by mutableStateOf(0f)
        private set
    var rootOffset = Offset.Zero
    val rowBounds = mutableMapOf<String, Rect>()

    /** Horizontal travel since pick-up — drives the projected nesting depth. */
    val dragOffsetX: Float get() = pointer.x - startPointer.x

    fun start(node: SbNode, origin: Offset, localGrab: Offset, height: Float) {
        dragging = node
        grabOffset = localGrab
        pointer = origin + localGrab
        startPointer = pointer
        rowHeight = height
        movedFar = false
    }

    fun drag(delta: Offset) {
        pointer += delta
        if ((pointer - startPointer).getDistance() > MOVE_THRESHOLD_PX) movedFar = true
    }

    val overlayTopLeft: Offset get() = pointer - grabOffset - rootOffset

    fun cancel() {
        dragging = null
        movedFar = false
    }

    private companion object {
        const val MOVE_THRESHOLD_PX = 16f
    }
}

@Composable
fun rememberSidebarDragState(): SidebarDragState = remember { SidebarDragState() }

/**
 * Projects the current pointer onto a drop target. [rows] is the full visible
 * flat tree (in render order); the dragged node and its whole subtree are
 * excluded internally so a group can't be dropped inside itself.
 */
// The nearest-group-parent scan uses two breaks (match found / scanned past the
// parent depth); both are intrinsic to the upward walk, so keep them as-is.
@Suppress("LoopWithTooManyJumpStatements")
fun resolveSidebarDrop(
    state: SidebarDragState,
    rows: List<SbNode>,
    indentStepPx: Float,
    treeTopPx: Float,
): SbDrop? {
    val node = state.dragging ?: return null
    val excluded = subtreeIds(node.id, rows)
    val visible = rows.filter { it.id !in excluded && state.rowBounds[it.id] != null }
    if (visible.isEmpty()) return SbDrop(null, null, null, 0, treeTopPx)

    val py = state.pointer.y
    var gapIndex = visible.indexOfFirst { py < state.rowBounds.getValue(it.id).center.y }
    if (gapIndex < 0) gapIndex = visible.size
    val prev = visible.getOrNull(gapIndex - 1)
    val next = visible.getOrNull(gapIndex)

    // A project can never be a parent, so you can only nest *into* a group above.
    val maxDepth = when {
        prev == null -> 0
        prev.kind == SbKind.GROUP -> prev.depth + 1
        else -> prev.depth
    }
    val minDepth = next?.depth ?: 0
    val raw = node.depth + (state.dragOffsetX / indentStepPx).roundToInt()
    val depth = raw.coerceIn(minOf(minDepth, maxDepth), maxOf(minDepth, maxDepth)).coerceAtLeast(0)

    val parentId = if (depth == 0) {
        null
    } else {
        var p: String? = null
        for (i in gapIndex - 1 downTo 0) {
            val r = visible[i]
            if (r.depth == depth - 1 && r.kind == SbKind.GROUP) {
                p = r.id
                break
            }
            if (r.depth < depth - 1) break
        }
        p
    }

    // Siblings share the parent AND the kind (groups and projects keep separate
    // position sequences server-side, matching the before/after midpoint logic).
    val siblings = visible.withIndex().filter { it.value.parentId == parentId && it.value.kind == node.kind }
    val beforeId = siblings.lastOrNull { it.index < gapIndex }?.value?.id
    val afterId = siblings.firstOrNull { it.index >= gapIndex }?.value?.id

    val gapY = next?.let { state.rowBounds.getValue(it.id).top }
        ?: prev?.let { state.rowBounds.getValue(it.id).bottom }
        ?: treeTopPx
    return SbDrop(parentId, beforeId, afterId, depth, gapY)
}

/** The dragged node's id plus every descendant's id (so it can't drop into itself). */
private fun subtreeIds(rootId: String, rows: List<SbNode>): Set<String> {
    val byParent = rows.groupBy { it.parentId }
    val out = mutableSetOf(rootId)
    val stack = ArrayDeque(byParent[rootId].orEmpty().map { it.id })
    while (stack.isNotEmpty()) {
        val id = stack.removeLast()
        if (out.add(id)) byParent[id].orEmpty().forEach { stack.addLast(it.id) }
    }
    return out
}

/** Registers a row's window bounds so drops can be projected against it. */
fun Modifier.sidebarRowBounds(state: SidebarDragState, id: String): Modifier =
    this.onGloballyPositioned { coords ->
        val tl = coords.positionInWindow()
        state.rowBounds[id] = Rect(tl.x, tl.y, tl.x + coords.size.width, tl.y + coords.size.height)
    }

/** Arms long-press drag on a sidebar row. [onDrop] runs at release (read live so
 *  it never fires a stale closure after a reorder). */
fun Modifier.draggableSidebarRow(state: SidebarDragState, node: SbNode, onDrop: () -> Unit): Modifier = composed {
    val latestOnDrop by rememberUpdatedState(onDrop)
    this.pointerInput(node.id) {
        detectDragGesturesAfterLongPress(
            onDragStart = { local ->
                val r = state.rowBounds[node.id]
                state.start(node, r?.topLeft ?: Offset.Zero, local, r?.height ?: 0f)
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

/** Dims the dragged row's slot while it's in flight (it stays in place as a
 *  placeholder; a floating clone tracks the finger). */
fun Modifier.sidebarDragDim(active: Boolean): Modifier =
    this.graphicsLayer { alpha = if (active) 0.3f else 1f }

/**
 * The drag overlay: a horizontal insertion line indented to the projected nesting
 * depth (so the user sees *where* and *how deep* it will land), plus a floating
 * clone of the dragged row tracking the finger.
 */
@Composable
fun SidebarDragOverlay(state: SidebarDragState, drop: SbDrop?, indentOf: (Int) -> Float) {
    val node = state.dragging ?: return
    val c = Tessera.colors
    val density = LocalDensity.current
    Box(Modifier.fillMaxSize()) {
        if (drop != null && state.movedFar) {
            val lineLeftDp = indentOf(drop.depth)
            Box(
                Modifier
                    .offset {
                        IntOffset(
                            with(density) { lineLeftDp.dp.toPx() }.roundToInt(),
                            (drop.gapY - state.rootOffset.y).roundToInt(),
                        )
                    }
                    .padding(end = 12.dp)
                    .fillMaxWidth()
                    .height(2.dp)
                    .clip(RoundedCornerShape(2.dp))
                    .background(accentGradient(c.primary)),
            )
        }
        Box(
            Modifier
                .offset { IntOffset(state.overlayTopLeft.x.roundToInt(), state.overlayTopLeft.y.roundToInt()) }
                .graphicsLayer { alpha = 0.92f },
        ) {
            SidebarRowGhost(node)
        }
    }
}

/** A floating, tinted clone of the dragged row (icon + label). */
@Composable
private fun SidebarRowGhost(node: SbNode) {
    val c = Tessera.colors
    Row(
        Modifier
            .clip(RoundedCornerShape(RadiusSm))
            .background(c.surface)
            .border(2.dp, c.primary, RoundedCornerShape(RadiusSm))
            .padding(horizontal = 10.dp, vertical = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        ProjectIcon(
            name = node.name,
            icon = node.icon,
            color = node.color,
            size = 20.dp,
            fallbackFolder = node.kind == SbKind.GROUP,
            iconMode = node.iconMode,
        )
        Spacer(Modifier.width(9.dp))
        Text(node.name, color = c.text1, fontSize = 14.sp, fontWeight = FontWeight.Medium, maxLines = 1)
    }
}
