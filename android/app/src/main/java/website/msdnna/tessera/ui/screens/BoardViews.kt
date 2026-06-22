package website.msdnna.tessera.ui.screens

import android.graphics.Paint
import android.graphics.Typeface
import androidx.activity.compose.BackHandler
import androidx.compose.animation.AnimatedContent
import androidx.compose.animation.core.Spring
import androidx.compose.animation.core.animate
import androidx.compose.animation.core.animateDpAsState
import androidx.compose.animation.core.calculateTargetValue
import androidx.compose.animation.core.spring
import androidx.compose.animation.core.tween
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.rememberSplineBasedDecay
import androidx.compose.animation.scaleIn
import androidx.compose.animation.togetherWith
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.ScrollState
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.gestures.FlingBehavior
import androidx.compose.foundation.gestures.ScrollScope
import androidx.compose.foundation.gestures.awaitEachGesture
import androidx.compose.foundation.gestures.awaitFirstDown
import androidx.compose.foundation.gestures.calculateCentroid
import androidx.compose.foundation.gestures.calculateZoom
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.gestures.scrollBy
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.requiredWidth
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListScope
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.key
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.runtime.withFrameNanos
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.clipToBounds
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.draw.rotate
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.PathEffect
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.compositeOver
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.drawscope.clipRect
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.graphics.nativeCanvas
import androidx.compose.ui.graphics.toArgb
import androidx.compose.ui.input.pointer.PointerEventPass
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.layout.onGloballyPositioned
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.layout.positionInWindow
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Density
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import java.util.Calendar
import kotlin.math.ceil
import kotlin.math.roundToInt
import kotlinx.coroutines.launch
import website.msdnna.tessera.data.model.Tag
import website.msdnna.tessera.data.model.Task
import website.msdnna.tessera.ui.components.BoardDragOverlay
import website.msdnna.tessera.ui.components.BoardDragState
import website.msdnna.tessera.ui.components.ColorDot
import website.msdnna.tessera.ui.components.Drop
import website.msdnna.tessera.ui.components.InlineCreateField
import website.msdnna.tessera.ui.components.InlineTitleEditor
import website.msdnna.tessera.ui.components.IonIcon
import website.msdnna.tessera.ui.components.IonIconButton
import website.msdnna.tessera.ui.components.TConfirmPopover
import website.msdnna.tessera.ui.components.TDropdown
import website.msdnna.tessera.ui.components.TMenuDivider
import website.msdnna.tessera.ui.components.TMenuItem
import website.msdnna.tessera.ui.components.TSwitch
import website.msdnna.tessera.ui.components.TaskCard
import website.msdnna.tessera.ui.components.animatePlacement
import website.msdnna.tessera.ui.components.clickableNoRipple
import website.msdnna.tessera.ui.components.dashedBorder
import website.msdnna.tessera.ui.components.dragCollapse
import website.msdnna.tessera.ui.components.dragDim
import website.msdnna.tessera.ui.components.draggableColumn
import website.msdnna.tessera.ui.components.dropColumn
import website.msdnna.tessera.ui.components.fadeInOnAppear
import website.msdnna.tessera.ui.components.leftAccentFrame
import website.msdnna.tessera.ui.components.rememberBoardDragState
import website.msdnna.tessera.ui.components.topAccentFrame
import website.msdnna.tessera.ui.theme.PriorityColors
import website.msdnna.tessera.ui.theme.RadiusLg
import website.msdnna.tessera.ui.theme.RadiusMd
import website.msdnna.tessera.ui.theme.RadiusSm
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.accentGradient
import website.msdnna.tessera.ui.viewmodels.BoardUiState
import website.msdnna.tessera.ui.viewmodels.BoardViewModel
import website.msdnna.tessera.util.Estimation
import website.msdnna.tessera.util.Ion
import website.msdnna.tessera.util.dueShort
import website.msdnna.tessera.util.isOverdue
import website.msdnna.tessera.util.isoDateKey
import website.msdnna.tessera.util.parseHexColor
import website.msdnna.tessera.util.parseInstantMillis
import website.msdnna.tessera.util.shortDate

/** A kanban lane: title, swatch, count, cards, and (status lanes) a + button. */
private class Lane(val id: String, val title: String, val color: String?, val tasks: List<Task>, val canAdd: Boolean)

@Composable
fun KanbanView(
    state: BoardUiState,
    vm: BoardViewModel,
    onOpenTask: (Task) -> Unit,
) {
    // Lanes carry each column's filtered + multi-level-sorted card list. That work
    // is O(n log n) per column; recomputing it on every recomposition re-sorts the
    // whole board on every drag frame (a drag mutates only drag state, not the
    // inputs below). Memoise on the actual inputs so a drag — or any unrelated
    // recomposition — reuses the cached lanes instead of re-sorting 100s of cards.
    val lanes = remember(
        state.groupByTag,
        state.tagPrefix,
        state.columns,
        state.tasks,
        state.tags,
        state.filter,
        state.sortLevels,
    ) {
        if (state.groupByTag) tagLanes(state) else columnLanes(state)
    }
    val scrollState = rememberScrollState()
    val drag = rememberBoardDragState()
    val focusManager = LocalFocusManager.current
    var addingColumn by remember { mutableStateOf<String?>(null) }
    var addingNewColumn by remember { mutableStateOf(false) }

    // Pause live (WebSocket) reloads while a card or column is being dragged, so
    // the board isn't reshuffled out from under the finger.
    LaunchedEffect(drag.dragging, drag.draggingColumn) {
        vm.dragging = drag.dragging != null || drag.draggingColumn != null
    }

    // Column reorder drop: resolve the pointer to a slot, then move via the API.
    val onDropColumn: (String) -> Unit = { colId ->
        if (drag.movedFar) {
            drag.resolveColumnDrop(lanes.map { it.id })?.let { (before, after) ->
                vm.moveColumn(colId, before, after)
            }
        }
    }

    // Shared drop handler for cards AND subtasks: resolve the pointer to a
    // column slot, then move (a subtask detaches to top-level first via dropTask).
    // A no-op drop of an already-top-level card in place is ignored.
    val onDropTask: (Task) -> Unit = { dropped ->
        val resolved = if (drag.movedFar) {
            drag.resolveDrop({ cid -> state.tasksIn(cid) }, { pid -> state.subtasksOf(pid) })
        } else {
            null
        }
        when (val d = resolved) {
            is Drop.Nest -> vm.nestTask(dropped, d.parentId, d.beforeId, d.afterId)

            is Drop.ToColumn ->
                if (!(dropped.parentId == null && d.columnId == dropped.columnId && d.beforeId == null && d.afterId == null)) {
                    vm.dropTask(dropped, d.columnId, d.beforeId, d.afterId)
                }

            null -> Unit
        }
    }

    BoxWithConstraints(
        Modifier.fillMaxSize()
            .onGloballyPositioned { drag.rootOffset = it.positionInWindow() }
            // Tap on empty board area commits/cancels an open inline field
            // (blur), mirroring the web's tap-away behaviour.
            .pointerInput(Unit) { detectTapGestures { focusManager.clearFocus() } },
    ) {
        // Columns sit a clear margin short of full width so the next column
        // peeks in (like the web mobile board); the row snaps column to column.
        val colWidth = maxWidth - 52.dp
        // One "page" = a column plus the 12dp inter-column gap; the snap fling
        // settles the scroll on a whole number of these so a column locks to the
        // left edge after a flick/drag-release.
        val stepPx = with(LocalDensity.current) { (colWidth + 12.dp).toPx() }
        val snapFling = rememberColumnSnapFling(scrollState, stepPx)
        val dropTarget = if (drag.dragging != null) {
            drag.resolveDrop({ state.tasksIn(it) }, { state.subtasksOf(it) })
        } else {
            null
        }
        val colDrop = dropTarget as? Drop.ToColumn
        val nestDrop = dropTarget as? Drop.Nest
        val draggingId = drag.dragging?.id
        val draggingColId = drag.draggingColumn?.id
        KanbanAutoScroll(drag, scrollState, this.maxWidth)
        // A plain horizontally-scrolling Row (NOT LazyRow): boards have few
        // columns, and not recycling means a dragged column that scrolls off-edge
        // keeps its gesture node alive (LazyRow would dispose it → drag reset).
        Row(
            Modifier.fillMaxSize().horizontalScroll(scrollState, flingBehavior = snapFling).padding(12.dp),
            horizontalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            lanes.forEach { lane ->
                // Stable key so Compose tracks each column's composable across a
                // reorder → animatePlacement slides it to its new spot on drop.
                key(lane.id) {
                    val laneColor = parseHexColor(lane.color, Tessera.colors.text3)
                    val laneHasColor = !lane.color.isNullOrBlank()
                    val column = state.columns.firstOrNull { it.id == lane.id }
                    var renamingCol by remember { mutableStateOf(false) }
                    // The dragged column stays in place but dimmed (a full ghost
                    // clone tracks the finger) — no collapse, so it doesn't jump.
                    Column(
                        Modifier.animatePlacement().width(colWidth).fillMaxHeight()
                            .dragDim(lane.id == draggingColId),
                    ) {
                        Column(
                            Modifier.fillMaxWidth()
                                .clip(RoundedCornerShape(RadiusLg))
                                .topAccentFrame(
                                    accent = laneColor,
                                    surface = Tessera.colors.surfaceAlt,
                                    border = Tessera.colors.border,
                                    barHeight = 3.dp,
                                    radius = RadiusLg,
                                    gradient = laneHasColor,
                                )
                                .then(if (lane.canAdd) Modifier.dropColumn(drag, lane.id) else Modifier),
                        ) {
                            Spacer(Modifier.height(4.dp)) // clear the coloured top strip drawn by the frame
                            Row(
                                Modifier.fillMaxWidth()
                                    // Long-press the header to drag the whole column.
                                    .then(
                                        if (lane.canAdd && column != null) {
                                            Modifier.draggableColumn(drag, column) { onDropColumn(lane.id) }
                                        } else {
                                            Modifier
                                        },
                                    )
                                    .padding(horizontal = 12.dp, vertical = 10.dp),
                                verticalAlignment = Alignment.CenterVertically,
                            ) {
                                val laneIcon = when {
                                    state.groupByTag -> Ion.PRICETAG
                                    lane.id == state.doneColumnId -> Ion.CHECK_CIRCLE
                                    lane.id == state.sortedColumns.firstOrNull()?.id -> Ion.ELLIPSE
                                    else -> Ion.CONTRAST
                                }
                                IonIcon(laneIcon, size = 16.dp, tint = laneColor, gradient = laneHasColor)
                                Spacer(Modifier.width(8.dp))
                                if (renamingCol && column != null) {
                                    InlineTitleEditor(
                                        initial = lane.title,
                                        onCommit = { name ->
                                            vm.renameColumn(column, name)
                                            renamingCol = false
                                        },
                                        onCancel = { renamingCol = false },
                                        modifier = Modifier.weight(1f),
                                        fontSize = 15.sp,
                                        fontWeight = FontWeight.SemiBold,
                                    )
                                } else {
                                    Text(
                                        lane.title,
                                        color = Tessera.colors.text1,
                                        fontSize = 15.sp,
                                        fontWeight = FontWeight.SemiBold,
                                        maxLines = 1,
                                        // Tap the title to rename inline (status lanes only).
                                        modifier = Modifier.weight(1f)
                                            .then(if (lane.canAdd) Modifier.clickableNoRipple { renamingCol = true } else Modifier),
                                    )
                                }
                                // Count sits just left of the column menu, both pinned right.
                                Text("${lane.tasks.size}", color = Tessera.colors.text3, fontSize = 13.sp)
                                if (lane.canAdd) {
                                    Spacer(Modifier.width(6.dp))
                                    ColumnMenu(lane, state, vm, onRename = { renamingCol = true })
                                }
                            }
                            // Cards are virtualised: a column with 100s of cards (e.g.
                            // an imported "Done" lane) composes only the visible window,
                            // not every card. The header above stays fixed; this list
                            // scrolls beneath it (web parity). weight(fill = false) lets a
                            // short column still hug its content instead of stretching.
                            //
                            // LazyColumn disposes off-screen cards, which is safe here:
                            // there is no vertical auto-scroll during a card drag, so the
                            // dragged card (collapsed in place via dragCollapse) stays in
                            // the composed window and its long-press gesture node survives.
                            // Disposed cards drop their stale entry from `cardBounds`
                            // (see draggableCard) so drop resolution only ever considers
                            // on-screen cards — the same set the finger can actually reach.
                            LazyColumn(
                                Modifier.fillMaxWidth().weight(1f, fill = false),
                                contentPadding = PaddingValues(start = 8.dp, end = 8.dp, bottom = 10.dp),
                                verticalArrangement = Arrangement.spacedBy(8.dp),
                            ) {
                                items(lane.tasks, key = { it.id }) { task ->
                                    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                                        // A faded copy of the dragged card marks the landing
                                        // slot (the dragged card itself collapses + floats).
                                        if (colDrop?.columnId == lane.id && colDrop.afterId == task.id) {
                                            DraggedPreview(drag.dragging, state, vm)
                                        }
                                        TaskCard(
                                            task = task,
                                            state = state,
                                            vm = vm,
                                            onOpen = onOpenTask,
                                            modifier = Modifier.fadeInOnAppear().animatePlacement().dragCollapse(task.id == draggingId),
                                            drag = if (lane.canAdd) drag else null,
                                            onDropTask = if (lane.canAdd) onDropTask else null,
                                            nestSlot = nestDrop?.takeIf { it.parentId == task.id }?.let { it.beforeId to it.afterId },
                                        )
                                    }
                                }
                                if (lane.canAdd) {
                                    item(key = "__footer__") {
                                        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                                            if (colDrop?.columnId == lane.id && colDrop.afterId == null && drag.dragging != null) {
                                                DraggedPreview(drag.dragging, state, vm)
                                            }
                                            if (addingColumn == lane.id) {
                                                InlineCreateField(
                                                    placeholder = "Название задачи, Enter",
                                                    onCommit = {
                                                        vm.createTask(lane.id, it)
                                                        addingColumn = null
                                                    },
                                                    onDismiss = { addingColumn = null },
                                                )
                                            } else {
                                                CreateText("+ СОЗДАТЬ ЗАДАЧУ") { addingColumn = lane.id }
                                            }
                                        }
                                    }
                                }
                            }
                        }
                    }
                }
            }
            // "+ column" placeholder at the end of the status lanes (web parity):
            // tap names a new column; a blank entry cancels back to the tile.
            if (!state.groupByTag) {
                Column(Modifier.width(colWidth)) {
                    if (addingNewColumn) {
                        Column(
                            Modifier.fillMaxWidth()
                                .clip(RoundedCornerShape(RadiusLg))
                                .background(Tessera.colors.surfaceAlt)
                                .padding(8.dp),
                        ) {
                            InlineCreateField(
                                placeholder = "Название колонки, Enter",
                                onCommit = {
                                    vm.createColumn(it)
                                    addingNewColumn = false
                                },
                                onDismiss = { addingNewColumn = false },
                            )
                        }
                    } else {
                        Box(
                            Modifier.fillMaxWidth()
                                .clip(RoundedCornerShape(RadiusLg))
                                .dashedBorder(Tessera.colors.border, RadiusLg)
                                .clickableNoRipple { addingNewColumn = true }
                                .padding(vertical = 14.dp),
                            contentAlignment = Alignment.Center,
                        ) {
                            Text(
                                "+ СОЗДАТЬ КОЛОНКУ",
                                color = Tessera.colors.text3,
                                fontSize = 11.sp,
                                fontWeight = FontWeight.Medium,
                            )
                        }
                    }
                }
            }
        }
        BoardDragOverlay(drag, colWidth, state, vm)
        ColumnDragLayer(drag, lanes, state, vm, colWidth, this.maxWidth)
    }
}

/** Column "⋯" settings: rename, colour swatches, "завершающая" toggle, delete —
 *  a themed popover with a confirm popover for the destructive delete. */
@Composable
private fun ColumnMenu(lane: Lane, state: BoardUiState, vm: BoardViewModel, onRename: () -> Unit) {
    val c = Tessera.colors
    val column = state.columns.firstOrNull { it.id == lane.id }
    var menu by remember { mutableStateOf(false) }
    var confirmDelete by remember { mutableStateOf(false) }
    val isDone = lane.id == state.doneColumnId

    Box {
        IonIconButton(Ion.ELLIPSIS_H, onClick = { menu = true }, boxSize = 28.dp, iconSize = 16.dp, tint = c.text3)
        TDropdown(expanded = menu, onDismiss = { menu = false }) {
            TMenuItem("Переименовать", icon = Ion.PENCIL, onClick = {
                menu = false
                onRename()
            })
            TMenuDivider()
            Text(
                "Цвет колонки",
                color = c.text3,
                fontSize = 12.sp,
                modifier = Modifier.padding(start = 14.dp, top = 6.dp, bottom = 4.dp),
            )
            FlowRow(
                Modifier.padding(horizontal = 12.dp).padding(bottom = 6.dp),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                ColumnPalette.forEach { hex ->
                    val col = parseHexColor(hex, c.text3)
                    val selected = column?.color.equals(hex, ignoreCase = true)
                    Box(
                        Modifier.size(22.dp).clip(CircleShape).background(accentGradient(col))
                            .then(if (selected) Modifier.border(2.dp, c.text1, CircleShape) else Modifier)
                            .clickableNoRipple {
                                column?.let { vm.setColumnColor(it, hex) }
                                menu = false
                            },
                    )
                }
            }
            TMenuDivider()
            Row(
                Modifier.fillMaxWidth().padding(horizontal = 14.dp, vertical = 8.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text("Завершающая", color = c.text1, fontSize = 14.sp, modifier = Modifier.weight(1f))
                TSwitch(checked = isDone, onCheckedChange = { vm.setDoneColumn(if (isDone) null else lane.id) })
            }
            TMenuDivider()
            TMenuItem("Удалить колонку", icon = Ion.TRASH, danger = true, onClick = {
                menu = false
                confirmDelete = true
            })
        }
        TConfirmPopover(
            expanded = confirmDelete,
            message = "Удалить «${lane.title}» и задачи в ней?",
            confirmText = "Удалить",
            onConfirm = {
                confirmDelete = false
                vm.deleteColumn(lane.id)
            },
            onDismiss = { confirmDelete = false },
        )
    }
}

/** Column colour swatches (the Naive accent set). */
private val ColumnPalette = listOf(
    "#7c5cff", "#2f80ed", "#0eb0a9", "#18a058", "#f0a020", "#e0533d", "#eb2f96", "#9aa0aa",
)

@Composable
private fun CreateText(label: String, onClick: () -> Unit) {
    val c = Tessera.colors
    Text(
        label,
        color = c.text3,
        fontSize = 11.sp,
        fontWeight = FontWeight.Medium,
        textAlign = TextAlign.Center,
        modifier = Modifier.fillMaxWidth().clip(RoundedCornerShape(RadiusMd)).clickableNoRipple(onClick = onClick)
            .padding(vertical = 10.dp),
    )
}

@Composable
fun BoardListView(state: BoardUiState, vm: BoardViewModel, onOpenTask: (Task) -> Unit) {
    val c = Tessera.colors
    LazyColumn(
        Modifier.fillMaxSize(),
        contentPadding = PaddingValues(12.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        state.sortedColumns.forEach { col ->
            val colTasks = state.visibleTasksIn(col.id)
            item(key = "h-${col.id}") {
                Row(Modifier.padding(top = 8.dp, bottom = 2.dp), verticalAlignment = Alignment.CenterVertically) {
                    ColorDot(parseHexColor(col.color, c.text3))
                    Spacer(Modifier.width(8.dp))
                    Text(col.name, color = c.text2, fontSize = 13.sp, fontWeight = FontWeight.SemiBold)
                    Spacer(Modifier.width(6.dp))
                    Text("${colTasks.size}", color = c.text3, fontSize = 12.sp)
                }
            }
            items(colTasks, key = { it.id }) { task ->
                TaskCard(task = task, state = state, vm = vm, onOpen = onOpenTask)
            }
        }
    }
}

private val CalMonths = listOf(
    "Январь", "Февраль", "Март", "Апрель", "Май", "Июнь",
    "Июль", "Август", "Сентябрь", "Октябрь", "Ноябрь", "Декабрь",
)
private val CalWeekdays = listOf("Пн", "Вт", "Ср", "Чт", "Пт", "Сб", "Вс")

/** One day cell of the month grid. */
private class CalCell(val key: String, val day: Int, val inMonth: Boolean)

private fun dayKey(cal: Calendar): String =
    "%04d-%02d-%02d".format(cal.get(Calendar.YEAR), cal.get(Calendar.MONTH) + 1, cal.get(Calendar.DAY_OF_MONTH))

/** 42 cells (6 weeks, Monday-first) covering [month0] of [year]. */
private fun monthCells(year: Int, month0: Int): List<CalCell> {
    val cal = Calendar.getInstance().apply {
        clear()
        set(year, month0, 1)
    }
    // Calendar weekday: SUNDAY=1..SATURDAY=7 → Monday-first index 0..6.
    val lead = (cal.get(Calendar.DAY_OF_WEEK) + 5) % 7
    cal.add(Calendar.DAY_OF_MONTH, -lead)
    return (0 until 42).map {
        val cell = CalCell(dayKey(cal), cal.get(Calendar.DAY_OF_MONTH), cal.get(Calendar.MONTH) == month0)
        cal.add(Calendar.DAY_OF_MONTH, 1)
        cell
    }
}

/**
 * A month-grid calendar (mirrors the web `BoardCalendarView`): a Пн-first 6×7
 * grid with task chips (priority-coloured left bar, strike-through when done) in
 * each day cell, a today highlight, prev/next/today nav, and an undated strip.
 */
@Composable
fun BoardCalendarView(state: BoardUiState, vm: BoardViewModel, onOpenTask: (Task) -> Unit) {
    val c = Tessera.colors
    val today = remember { Calendar.getInstance() }
    val todayKey = remember { dayKey(today) }
    var cursor by remember { mutableStateOf(today.get(Calendar.YEAR) to today.get(Calendar.MONTH)) }
    val (year, month) = cursor

    // Memoised so zoom/scroll recompositions don't re-filter+sort 200 tasks each frame.
    val tasks = remember(state.tasks, state.filter, state.sortLevels) { state.applyFilterSort(state.tasks) }
    val byDay = tasks.filter { !it.dueDate.isNullOrBlank() }.groupBy { isoDateKey(it.dueDate) }
    val undated = tasks.filter { it.dueDate.isNullOrBlank() }
    val cells = remember(year, month) { monthCells(year, month) }

    fun shift(delta: Int) {
        var m = month + delta
        var y = year
        while (m < 0) {
            m += 12
            y--
        }
        while (m > 11) {
            m -= 12
            y++
        }
        cursor = y to m
    }

    Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(12.dp)) {
        // Nav: ‹ Сегодня › + "Июнь 2026".
        Row(Modifier.fillMaxWidth().padding(bottom = 10.dp), verticalAlignment = Alignment.CenterVertically) {
            Row(
                Modifier.clip(RoundedCornerShape(RadiusMd)).border(1.dp, c.border, RoundedCornerShape(RadiusMd)),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                IonIconButton(Ion.CHEVRON_FORWARD, onClick = { shift(-1) }, boxSize = 32.dp, iconSize = 15.dp, tint = c.text2, modifier = Modifier.rotate(180f))
                Text(
                    "Сегодня",
                    color = c.text1,
                    fontSize = 13.sp,
                    fontWeight = FontWeight.Medium,
                    modifier = Modifier.clickableNoRipple { cursor = today.get(Calendar.YEAR) to today.get(Calendar.MONTH) }
                        .padding(horizontal = 6.dp, vertical = 6.dp),
                )
                IonIconButton(Ion.CHEVRON_FORWARD, onClick = { shift(1) }, boxSize = 32.dp, iconSize = 15.dp, tint = c.text2)
            }
            Spacer(Modifier.width(12.dp))
            Text("${CalMonths[month]} $year", color = c.text1, fontSize = 16.sp, fontWeight = FontWeight.SemiBold)
        }

        // Grid: 1px gaps over a border-coloured backing render the grid lines.
        Column(
            Modifier.fillMaxWidth().clip(RoundedCornerShape(RadiusMd)).background(c.border).padding(1.dp),
            verticalArrangement = Arrangement.spacedBy(1.dp),
        ) {
            Row(horizontalArrangement = Arrangement.spacedBy(1.dp)) {
                CalWeekdays.forEach { wd ->
                    Box(Modifier.weight(1f).background(c.surfaceAlt).padding(vertical = 6.dp), contentAlignment = Alignment.Center) {
                        Text(wd, color = c.text3, fontSize = 12.sp, fontWeight = FontWeight.SemiBold)
                    }
                }
            }
            cells.chunked(7).forEach { week ->
                Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(1.dp)) {
                    week.forEach { cell ->
                        DayCell(cell, byDay[cell.key].orEmpty(), cell.key == todayKey, state, onOpenTask)
                    }
                }
            }
        }

        if (undated.isNotEmpty()) {
            Text("Без срока", color = c.text3, fontSize = 13.sp, fontWeight = FontWeight.SemiBold, modifier = Modifier.padding(top = 14.dp, bottom = 6.dp))
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                undated.forEach { task -> TaskCard(task = task, state = state, vm = vm, onOpen = onOpenTask) }
            }
        }
    }
}

@Composable
private fun RowScope.DayCell(
    cell: CalCell,
    tasks: List<Task>,
    isToday: Boolean,
    state: BoardUiState,
    onOpenTask: (Task) -> Unit,
) {
    val c = Tessera.colors
    Column(
        Modifier.weight(1f).heightIn(min = 88.dp)
            .background(if (cell.inMonth) c.surface else c.surfaceAlt)
            .padding(4.dp),
        verticalArrangement = Arrangement.spacedBy(3.dp),
    ) {
        if (isToday) {
            Box(Modifier.size(20.dp).clip(CircleShape).background(accentGradient(c.primary)), contentAlignment = Alignment.Center) {
                Text("${cell.day}", color = c.onPrimary, fontSize = 12.sp, fontWeight = FontWeight.SemiBold)
            }
        } else {
            Text("${cell.day}", color = if (cell.inMonth) c.text2 else c.text3, fontSize = 12.sp, modifier = Modifier.padding(start = 2.dp))
        }
        tasks.take(4).forEach { task -> CalChip(task, onOpenTask) }
        if (tasks.size > 4) {
            Text("+${tasks.size - 4}", color = c.text3, fontSize = 11.sp, modifier = Modifier.padding(start = 2.dp))
        }
    }
}

/** A task chip in a day cell: a priority-coloured left bar + truncated title. */
@Composable
private fun CalChip(task: Task, onOpenTask: (Task) -> Unit) {
    val c = Tessera.colors
    val accent = PriorityColors.getOrElse(task.priority) { PriorityColors[0] }
    Row(
        Modifier.fillMaxWidth().clip(RoundedCornerShape(4.dp)).background(c.hover)
            .clickableNoRipple { onOpenTask(task) }
            .padding(start = 0.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Box(Modifier.width(3.dp).height(16.dp).clip(RoundedCornerShape(2.dp)).background(accentGradient(accent)))
        Spacer(Modifier.width(4.dp))
        Text(
            task.title,
            color = if (task.isCompleted) c.text3 else c.text1,
            fontSize = 12.sp,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
            textDecoration = if (task.isCompleted) TextDecoration.LineThrough else null,
            modifier = Modifier.weight(1f).padding(vertical = 2.dp, horizontal = 2.dp),
        )
    }
}

// ── Eisenhower matrix view ────────────────────────────────────────────────

private class EisenQuad(val index: Int, val title: String, val hint: String, val accent: Color)

private val EisenhowerQuads = listOf(
    EisenQuad(0, "Срочно и важно", "Сделать сейчас", Color(0xFFEF5D5D)),
    EisenQuad(1, "Важно, не срочно", "Запланировать", Color(0xFF5B8DEF)),
    EisenQuad(2, "Срочно, не важно", "Делегировать", Color(0xFFE6A43B)),
    EisenQuad(3, "Не срочно, не важно", "Может подождать", Color(0xFF9AA0AA)),
)

private const val EISEN_URGENT_DAYS = 7

/** Derived quadrant (no override): importance from priority, urgency from due-date.
 *  Encoding matches the backend (0 urgent+important … 3 not-urgent+not-important). */
private fun deriveQuadrant(task: Task): Int {
    val important = task.priority >= 3
    val due = parseInstantMillis(task.dueDate)
    val urgent = due != null && due <= System.currentTimeMillis() + EISEN_URGENT_DAYS * 86_400_000L
    return if (important) (if (urgent) 0 else 1) else (if (urgent) 2 else 3)
}
private fun quadrantOf(task: Task): Int = task.eisenhowerQuadrant ?: deriveQuadrant(task)

/**
 * The Eisenhower matrix (mirrors the web `BoardMatrixView`): a 2×2 Важно×Срочно
 * grid showing only OPEN tasks (the matrix is about what's left to do). A card's
 * quadrant is derived from priority + due-date proximity, or pinned manually via the
 * per-card menu («Вернуть на авто» clears the override). On the phone a quadrant can
 * be expanded full-screen (they're cramped at 2×2).
 */
@Composable
fun BoardMatrixView(state: BoardUiState, vm: BoardViewModel, onOpenTask: (Task) -> Unit) {
    val c = Tessera.colors
    val focusManager = LocalFocusManager.current
    // Open tasks only — completed cards belong to «done», not the triage matrix.
    val tasks = state.applyFilterSort(state.tasks).filter { !it.isCompleted }
    val byQuad = tasks.groupBy { quadrantOf(it) }
    var expanded by remember { mutableStateOf<Int?>(null) }
    BackHandler(enabled = expanded != null) { expanded = null }

    Column(
        Modifier.fillMaxSize().padding(8.dp)
            // Tap on empty matrix area commits/cancels an open inline field (blur),
            // mirroring the board's tap-away behaviour.
            .pointerInput(Unit) { detectTapGestures { focusManager.clearFocus() } },
    ) {
        AnimatedContent(
            targetState = expanded,
            transitionSpec = {
                (fadeIn(tween(200)) + scaleIn(tween(200), initialScale = 0.93f)) togetherWith fadeOut(tween(140))
            },
            label = "matrixExpand",
        ) { focus ->
            if (focus != null) {
                Column(Modifier.fillMaxSize()) {
                    QuadrantCell(
                        EisenhowerQuads[focus], byQuad[focus].orEmpty(), state, vm, onOpenTask,
                        expanded = true, onToggleExpand = { expanded = null },
                        modifier = Modifier.fillMaxSize(),
                    )
                }
            } else {
                Column(Modifier.fillMaxSize()) {
                    // Column headers: Срочно | Несрочно (offset past the row-label gutter).
                    Row(Modifier.fillMaxWidth().padding(start = 20.dp, bottom = 4.dp)) {
                        MatrixColHeader("Срочно", c.text1, Modifier.weight(1f))
                        MatrixColHeader("Несрочно", c.text2, Modifier.weight(1f))
                    }
                    listOf(Triple("Важно", 0, 1), Triple("Неважно", 2, 3)).forEach { (rowLabel, left, right) ->
                        Row(Modifier.fillMaxWidth().weight(1f).padding(vertical = 4.dp)) {
                            Box(Modifier.width(20.dp).fillMaxHeight(), contentAlignment = Alignment.Center) {
                                Text(
                                    rowLabel,
                                    color = c.text2,
                                    fontSize = 13.sp,
                                    fontWeight = FontWeight.SemiBold,
                                    maxLines = 1,
                                    softWrap = false,
                                    // Measure unconstrained then rotate, so the 20dp
                                    // gutter doesn't ellipsize before it's vertical.
                                    modifier = Modifier.requiredWidth(120.dp).rotate(270f),
                                    textAlign = TextAlign.Center,
                                )
                            }
                            QuadrantCell(
                                EisenhowerQuads[left], byQuad[left].orEmpty(), state, vm, onOpenTask,
                                onToggleExpand = { expanded = left }, modifier = Modifier.weight(1f),
                            )
                            Spacer(Modifier.width(8.dp))
                            QuadrantCell(
                                EisenhowerQuads[right], byQuad[right].orEmpty(), state, vm, onOpenTask,
                                onToggleExpand = { expanded = right }, modifier = Modifier.weight(1f),
                            )
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun MatrixColHeader(text: String, color: Color, modifier: Modifier) {
    Text(
        text,
        color = color,
        fontSize = 13.sp,
        fontWeight = FontWeight.SemiBold,
        textAlign = TextAlign.Center,
        modifier = modifier,
    )
}

@Composable
private fun QuadrantCell(
    quad: EisenQuad,
    tasks: List<Task>,
    state: BoardUiState,
    vm: BoardViewModel,
    onOpenTask: (Task) -> Unit,
    onToggleExpand: () -> Unit,
    modifier: Modifier = Modifier,
    expanded: Boolean = false,
) {
    val c = Tessera.colors
    var adding by remember { mutableStateOf(false) }
    // The quadrant's actual background = a soft same-hue wash over the surface. Held
    // as one flat colour so the bottom fade can dissolve INTO it (a gradient to a
    // bare `Transparent` would pass through transparent-black → a grey/black smear).
    val quadBg = quad.accent.copy(alpha = 0.06f).compositeOver(c.surface)
    Column(
        modifier.fillMaxHeight()
            .clip(RoundedCornerShape(RadiusLg))
            // Same rounded top-accent frame the kanban columns use (corner-wrapping
            // coloured strip + 1px border + surface fill).
            .topAccentFrame(
                accent = quad.accent,
                surface = quadBg,
                border = c.border,
                barHeight = 3.dp,
                radius = RadiusLg,
                gradient = true,
            ),
    ) {
        Spacer(Modifier.height(4.dp)) // clear the coloured top strip drawn by the frame
        Row(
            Modifier.fillMaxWidth().padding(start = 10.dp, end = 4.dp, top = 4.dp, bottom = 4.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Column(Modifier.weight(1f)) {
                Text(quad.title, color = c.text1, fontSize = 13.sp, fontWeight = FontWeight.SemiBold, maxLines = 1, overflow = TextOverflow.Ellipsis)
                Text(quad.hint, color = c.text3, fontSize = 11.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
            }
            Text("${tasks.size}", color = c.text3, fontSize = 12.sp, fontWeight = FontWeight.SemiBold)
            IonIconButton(if (expanded) Ion.CLOSE else Ion.EXPAND, onClick = onToggleExpand, boxSize = 30.dp, iconSize = 16.dp, tint = c.text3)
            IonIconButton(Ion.ADD, onClick = { adding = true }, boxSize = 30.dp, iconSize = 17.dp, tint = c.text3)
        }
        Box(Modifier.weight(1f).fillMaxWidth()) {
            LazyColumn(
                Modifier.fillMaxSize(),
                contentPadding = PaddingValues(start = 8.dp, end = 8.dp, top = 2.dp, bottom = 8.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                items(tasks, key = { it.id }) { task ->
                    // animateItem fades the card out of its old quadrant and into the
                    // new one when its quadrant changes (the menu move) — the closest
                    // thing to an animated move without cross-list DnD.
                    MatrixCard(task, quadrantOf(task), state, vm, onOpenTask, Modifier.animateItem())
                }
            }
            // When the quick-add is open below, fade the list's bottom cards into it
            // (a soft dissolve, mirrors the web mask) instead of a hard cut edge.
            if (adding) {
                Box(
                    Modifier.align(Alignment.BottomCenter).fillMaxWidth().height(28.dp)
                        .background(Brush.verticalGradient(listOf(quadBg.copy(alpha = 0f), quadBg))),
                )
            }
        }
        // Quick-add sits BELOW the list (always visible, bordered), not buried at the
        // scroll bottom of a long quadrant.
        if (adding) {
            Box(Modifier.fillMaxWidth().padding(start = 8.dp, end = 8.dp, bottom = 8.dp)) {
                InlineCreateField(
                    placeholder = "Название задачи",
                    onCommit = { title ->
                        vm.createTaskInQuadrant(title, quad.index)
                        adding = false
                    },
                    onDismiss = { adding = false },
                )
            }
        }
    }
}

/**
 * A COMPACT matrix card (deliberately not the full board `TaskCard`, which is too
 * wide for the cramped 2×2 grid and collapsed under weight). Priority bar + 2-line
 * title + a small meta line, tap to open, kebab to move between quadrants.
 */
@Composable
private fun MatrixCard(
    task: Task,
    currentQuad: Int,
    state: BoardUiState,
    vm: BoardViewModel,
    onOpenTask: (Task) -> Unit,
    modifier: Modifier = Modifier,
) {
    val c = Tessera.colors
    val accent = PriorityColors.getOrElse(task.priority) { PriorityColors[0] }
    val subs = state.subtasksOf(task.id)
    var menu by remember { mutableStateOf(false) }
    val shape = RoundedCornerShape(RadiusMd)
    Row(
        modifier.fillMaxWidth()
            .clip(shape)
            // Same rounded accent frame the board cards use (corner-wrapping bar +
            // 1px border drawn on the surface), not a flat straight-edged bar.
            .leftAccentFrame(
                accent = if (task.priority > 0) accent else c.border,
                surface = c.cardSurface,
                border = c.border,
                barWidth = if (task.priority > 0) 3.dp else 1.dp,
                topRadius = RadiusMd,
                bottomRadius = RadiusMd,
                gradient = task.priority > 0,
            )
            .clickableNoRipple { onOpenTask(task) },
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Column(Modifier.weight(1f).padding(start = 11.dp, end = 9.dp, top = 8.dp, bottom = 8.dp)) {
            Text(
                task.title,
                color = if (task.isCompleted) c.text3 else c.text1,
                fontSize = 13.sp,
                lineHeight = 17.sp,
                maxLines = 2,
                overflow = TextOverflow.Ellipsis,
                textDecoration = if (task.isCompleted) TextDecoration.LineThrough else null,
            )
            val due = shortDate(task.dueDate)
            if (task.number != null || due.isNotEmpty() || subs.isNotEmpty()) {
                Row(Modifier.padding(top = 3.dp), verticalAlignment = Alignment.CenterVertically) {
                    task.number?.let {
                        Text("#$it", color = c.text3, fontSize = 11.sp)
                        Spacer(Modifier.width(8.dp))
                    }
                    if (due.isNotEmpty()) {
                        IonIcon(Ion.CALENDAR, size = 11.dp, tint = c.text3)
                        Spacer(Modifier.width(3.dp))
                        Text(due, color = c.text3, fontSize = 11.sp)
                        if (subs.isNotEmpty()) Spacer(Modifier.width(8.dp))
                    }
                    // Subtask indicator (always shown when the task has subtasks) so
                    // it's clear at a glance the card has a checklist.
                    if (subs.isNotEmpty()) {
                        IonIcon(Ion.LIST, size = 12.dp, tint = c.text3)
                        Spacer(Modifier.width(3.dp))
                        Text("${subs.count { it.isCompleted }}/${subs.size}", color = c.text3, fontSize = 11.sp)
                    }
                }
            }
            // Expanded: list the subtasks as compact rows (mirrors the board's
            // subtasks-expanded toggle).
            if (state.subtasksExpanded && subs.isNotEmpty()) {
                Column(Modifier.padding(top = 5.dp), verticalArrangement = Arrangement.spacedBy(3.dp)) {
                    subs.forEach { sub ->
                        Row(
                            Modifier.fillMaxWidth().clickableNoRipple { onOpenTask(sub) },
                            verticalAlignment = Alignment.CenterVertically,
                        ) {
                            Box(
                                Modifier.size(7.dp).clip(CircleShape)
                                    .background(if (sub.isCompleted) accentGradient(accent) else SolidColor(c.border)),
                            )
                            Spacer(Modifier.width(6.dp))
                            Text(
                                sub.title,
                                color = if (sub.isCompleted) c.text3 else c.text2,
                                fontSize = 12.sp,
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis,
                                textDecoration = if (sub.isCompleted) TextDecoration.LineThrough else null,
                            )
                        }
                    }
                }
            }
        }
        Box {
            IonIconButton(Ion.ELLIPSIS_V, onClick = { menu = true }, boxSize = 30.dp, iconSize = 15.dp, tint = c.text3)
            TDropdown(expanded = menu, onDismiss = { menu = false }) {
                EisenhowerQuads.forEach { q ->
                    TMenuItem(
                        label = q.title,
                        onClick = {
                            vm.setEisenhower(task.id, q.index)
                            menu = false
                        },
                        trailing = if (q.index == currentQuad) {
                            { IonIcon(Ion.CHECK, size = 16.dp, tint = c.primary) }
                        } else {
                            null
                        },
                    )
                }
                if (task.eisenhowerQuadrant != null) {
                    TMenuDivider()
                    TMenuItem(label = "Вернуть на авто", icon = Ion.REFRESH, onClick = {
                        vm.setEisenhower(task.id, null)
                        menu = false
                    })
                }
            }
        }
    }
}

private fun columnLanes(state: BoardUiState): List<Lane> =
    state.sortedColumns.map { col -> Lane(col.id, col.name, col.color, state.visibleTasksIn(col.id), canAdd = true) }

private fun tagLanes(state: BoardUiState): List<Lane> {
    // In namespace mode only tags carrying the prefix become columns; "Без тега"
    // then collects tasks with no tag *in that namespace* (web `rebuildLists`).
    val tags: List<Tag> = state.tags.values
        .filter { state.tagPrefix.isEmpty() || it.name.startsWith(state.tagPrefix) }
        .sortedBy { it.name }
    val byTag = tags.map { tag ->
        Lane(tag.id, tag.name, tag.color, state.applyFilterSort(state.tasks.filter { tag.id in it.tagIds }), canAdd = false)
    }
    val tagIds = tags.map { it.id }.toSet()
    val untagged = state.applyFilterSort(state.tasks.filter { task -> task.tagIds.none { it in tagIds } })
    return byTag + Lane("none", "Без тега", null, untagged, canAdd = false)
}

/**
 * A snapping fling for the (non-lazy) board row: after a flick or drag-release it
 * lets the natural fling project, then animates to the nearest column boundary so
 * a column locks flush to the left edge. [stepPx] is one column + inter-column
 * gap. Columns are uniform width, so a single step is all we need.
 */
@Composable
private fun rememberColumnSnapFling(scrollState: ScrollState, stepPx: Float): FlingBehavior {
    val decay = rememberSplineBasedDecay<Float>()
    return remember(scrollState, stepPx, decay) {
        object : FlingBehavior {
            override suspend fun ScrollScope.performFling(initialVelocity: Float): Float {
                if (stepPx <= 0f || scrollState.maxValue == 0) return initialVelocity
                val start = scrollState.value.toFloat()
                val projected = decay.calculateTargetValue(start, initialVelocity)
                val maxIndex = ceil(scrollState.maxValue / stepPx).toInt()
                // Snap to the nearest column, but never skip past more than ONE
                // column from where the gesture was released — otherwise a quick
                // flick projects several columns ahead and "jumps over" one.
                val current = (start / stepPx).roundToInt()
                val target = (
                    (projected / stepPx).roundToInt()
                        .coerceIn(current - 1, current + 1)
                        .coerceIn(0, maxIndex) * stepPx
                    )
                    .coerceIn(0f, scrollState.maxValue.toFloat())
                var last = start
                animate(start, target, initialVelocity, spring(stiffness = Spring.StiffnessMediumLow)) { value, _ ->
                    last += scrollBy(value - last)
                }
                return 0f
            }
        }
    }
}

/** While a card is held, auto-scrolls the columns when the finger nears an edge. */
@Composable
private fun KanbanAutoScroll(drag: BoardDragState, scrollState: ScrollState, boardWidth: Dp) {
    val density = LocalDensity.current
    val active = drag.dragging != null || drag.draggingColumn != null
    LaunchedEffect(active) {
        if (!active) return@LaunchedEffect
        val edge = with(density) { 72.dp.toPx() }
        val widthPx = with(density) { boardWidth.toPx() }
        // Columns are full-width, so reaching a left/right neighbour needs the row
        // to scroll as the finger nears an edge — same as card drag.
        while (drag.dragging != null || drag.draggingColumn != null) {
            val x = drag.pointer.x - drag.rootOffset.x
            val dx = when {
                x < edge -> -14f
                x > widthPx - edge -> 14f
                else -> 0f
            }
            if (dx != 0f) scrollState.scrollBy(dx)
            withFrameNanos { }
        }
    }
}

/** A faded copy of the dragged card, shown at the drop slot as a live preview. */
@Composable
private fun DraggedPreview(task: Task?, state: BoardUiState, vm: BoardViewModel) {
    if (task == null) return
    Box(Modifier.fillMaxWidth().alpha(0.45f)) {
        TaskCard(task = task, state = state, vm = vm, onOpen = {}, compact = true)
    }
}

/**
 * While a column is dragged: a full faded ghost clone of the column tracks the
 * finger (like the web), plus a vertical insertion line at the target slot. The
 * original stays in place dimmed.
 */
@Composable
private fun ColumnDragLayer(
    drag: BoardDragState,
    lanes: List<Lane>,
    state: BoardUiState,
    vm: BoardViewModel,
    colWidth: Dp,
    boardWidth: Dp,
) {
    val col = drag.draggingColumn ?: return
    val lane = lanes.firstOrNull { it.id == col.id } ?: return
    val density = LocalDensity.current
    val laneColor = parseHexColor(lane.color, Tessera.colors.text3)
    val (before, after) = drag.resolveColumnDrop(lanes.map { it.id }) ?: (null to null)
    val ref = drag.columnBounds[col.id]
    // columnBounds are in PIXELS — so the edge offset must be a dp converted to px
    // (a bare `6f` would be ~2dp). For a middle slot the real pixel midpoint
    // centres the line in the 12dp gap; at an edge there's no opposite column, so
    // push the line a comfortable gap off the column.
    val bRight = before?.let { drag.columnBounds[it]?.right }
    val aLeft = after?.let { drag.columnBounds[it]?.left }
    // At an edge there's no opposite column — centre the line in the visible gap
    // between the edge column and the board edge (so it's balanced, not flush).
    val boardLeft = drag.rootOffset.x
    val boardRight = drag.rootOffset.x + with(density) { boardWidth.toPx() }
    val lineCenterX = when {
        bRight != null && aLeft != null -> (bRight + aLeft) / 2f
        aLeft != null -> (boardLeft + aLeft) / 2f
        bRight != null -> (bRight + boardRight) / 2f
        else -> null
    }
    Box(Modifier.fillMaxSize()) {
        if (lineCenterX != null && ref != null) {
            // The line is the dragged column's colour, inset a FIXED amount from
            // the column's top/bottom (not proportional — so columns of different
            // heights get the same inset). Clamped to stay on screen.
            val pad = with(density) { 8.dp.toPx() }
            val lineH = with(density) { (ref.height - 2f * pad).coerceAtLeast(0f).toDp() }
            Box(
                Modifier
                    .offset {
                        val minX = 2.dp.toPx()
                        val maxX = boardWidth.toPx() - 5.dp.toPx()
                        val x = (lineCenterX - drag.rootOffset.x - 1.5.dp.toPx()).coerceIn(minX, maxX)
                        IntOffset(x.roundToInt(), (ref.top - drag.rootOffset.y + pad).roundToInt())
                    }
                    .width(3.dp).height(lineH)
                    .clip(RoundedCornerShape(2.dp)).background(accentGradient(laneColor)),
            )
        }
        Box(
            Modifier
                .offset { IntOffset(drag.overlayTopLeft.x.roundToInt(), drag.overlayTopLeft.y.roundToInt()) }
                .width(colWidth)
                .graphicsLayer { alpha = 0.85f },
        ) {
            ColumnGhost(lane, state, vm)
        }
    }
}

/** A read-only, faded rendering of a column (header + cards) for the drag ghost. */
@Composable
private fun ColumnGhost(lane: Lane, state: BoardUiState, vm: BoardViewModel) {
    val c = Tessera.colors
    val laneColor = parseHexColor(lane.color, c.text3)
    val laneHasColor = !lane.color.isNullOrBlank()
    Column(
        Modifier.fillMaxWidth().clip(RoundedCornerShape(RadiusLg))
            .topAccentFrame(accent = laneColor, surface = c.surfaceAlt, border = c.border, barHeight = 4.dp, radius = RadiusLg, gradient = laneHasColor),
    ) {
        Spacer(Modifier.height(4.dp))
        Row(
            Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 10.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            IonIcon(Ion.CONTRAST, size = 16.dp, tint = laneColor, gradient = laneHasColor)
            Spacer(Modifier.width(8.dp))
            Text(lane.title, color = c.text1, fontSize = 15.sp, fontWeight = FontWeight.SemiBold, maxLines = 1, modifier = Modifier.weight(1f))
            Text("${lane.tasks.size}", color = c.text3, fontSize = 13.sp)
        }
        Column(
            Modifier.fillMaxWidth().padding(start = 8.dp, end = 8.dp, bottom = 10.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            lane.tasks.forEach { task -> TaskCard(task = task, state = state, vm = vm, onOpen = {}) }
        }
    }
}

// ── Timeline view ─────────────────────────────────────────────────────────
// A horizontal time-axis (a reference tracker Timeline): swimlanes of start→due bars on a
// scrollable day grid with a «today» line. Read + tap-to-open — rescheduling is
// done in the task's due popover (which edits both start and due). In-bar drag is
// deliberately skipped (same call as the matrix: the board drag system is 1D-lane
// tuned, and a horizontally-scrolling track fights a horizontal drag gesture).

private const val TL_DAY_MS = 86_400_000L
private val TL_DAY_W = 30.dp
private val TL_ROW_H = 38.dp
private val TL_LEFT_W = 168.dp

// Header = month band + day band; lane (group) header band. The left fixed column
// and the right scrolling grid of every row must share the same height or the grid
// "slides" out of alignment with the day numbers.
private val TL_MONTH_H = 20.dp
private val TL_DAYS_H = 34.dp
private val TL_HEAD_H = 54.dp // TL_MONTH_H + TL_DAYS_H
private val TL_LANE_H = 30.dp

// Cap the day window in px so a row's `axisW`-wide track Box can't exceed Compose's
// Constraints width limit (~262k px; "Can't represent a width of …"). Now that BOTH
// views are virtualized (LazyColumn → only SHORT per-row boxes; the 0.29.1 crash was a
// TALL axisW×bodyH box that no longer exists), the cap can be large — it then triggers
// only on multi-year boards at extreme zoom, so `rangeStart` stops shifting mid-zoom
// (the date "jumps" at medium/large scales). Keep a safe margin under the limit.
private const val TL_MAX_AXIS_PX = 200_000f
private const val TL_MIN_DAYS = 30

private fun tlDayFloor(ms: Long): Long =
    Calendar.getInstance().apply {
        timeInMillis = ms
        set(Calendar.HOUR_OF_DAY, 0)
        set(Calendar.MINUTE, 0)
        set(Calendar.SECOND, 0)
        set(Calendar.MILLISECOND, 0)
    }.timeInMillis

private data class TLane(val key: String, val label: String, val color: Color?, val tasks: List<Task>)

/** Flattened timeline body rows, so the body can virtualize via LazyColumn (only
 *  visible rows compose — the ~200-task perf fix, mirrors the kanban cards). */
private sealed interface TlBodyRow
private data class TlLaneHeaderRow(val lane: TLane) : TlBodyRow
private data class TlTaskRow(val task: Task) : TlBodyRow

/** Summed estimate of a lane's tasks, formatted in the board's unit, or null when none set. */
private fun laneEffort(lane: TLane, state: BoardUiState): String? {
    val total = website.msdnna.tessera.util.Estimation.sum(lane.tasks.map { it.estimate }) ?: return null
    return website.msdnna.tessera.util.Estimation.format(total, state.estimation).ifBlank { null }
}

@Composable
fun BoardTimelineView(state: BoardUiState, vm: BoardViewModel, onOpenTask: (Task) -> Unit) {
    val c = Tessera.colors
    val density = LocalDensity.current
    val scope = rememberCoroutineScope()
    // px-per-day, pinch-zoomable (and via the −/+ buttons). `viewportPx` is the
    // scrolling-track width and `anchorMs` is the absolute date kept under the
    // viewport centre across a zoom (−1 = not set → centre today). Anchoring on a
    // date (not a day index) survives the cap window shifting `rangeStart` on zoom.
    var dayW by remember { mutableStateOf(TL_DAY_W) }
    var viewportPx by remember { mutableStateOf(0) }
    var anchorMs by remember { mutableStateOf(-1L) }
    // Track-relative x the anchor date should stay at through a zoom (-1 = viewport
    // centre). Set to the pinch focal point so zoom converges where the fingers are.
    var anchorScreenX by remember { mutableStateOf(-1f) }
    // Collapsible left task/group column — animated to 0 to give the chart full width.
    var leftCollapsed by remember { mutableStateOf(false) }
    val leftW by animateDpAsState(if (leftCollapsed) 0.dp else TL_LEFT_W, label = "tlLeftW")

    // Memoised so zoom/scroll recompositions don't re-filter+sort 200 tasks each frame.
    val tasks = remember(state.tasks, state.filter, state.sortLevels) { state.applyFilterSort(state.tasks) }
    val scheduled = tasks.filter { it.startDate != null || it.dueDate != null }
    val unscheduled = tasks.filter { it.startDate == null && it.dueDate == null }

    // Effective day-floored span of a task (a one-ended task is a 1-day bar).
    fun span(t: Task): Pair<Long, Long> {
        val s = parseInstantMillis(t.startDate)
        val d = parseInstantMillis(t.dueDate)
        val a = s ?: d ?: 0L
        val b = d ?: s ?: 0L
        return tlDayFloor(a) to tlDayFloor(b)
    }

    val todayMs = tlDayFloor(System.currentTimeMillis())
    // Memoised: the span()/estimate loop parses date strings for every task — doing it
    // each zoom frame (200 tasks) was a big hitch. Depends only on the task set, not dayW.
    val (loBase, hiBase) = remember(scheduled, state.estimation, todayMs) {
        var lo = todayMs
        var hi = todayMs
        scheduled.forEach {
            val (a, b) = span(it)
            lo = minOf(lo, a)
            hi = maxOf(hi, b)
            website.msdnna.tessera.util.Estimation.toDays(it.estimate, state.estimation)?.let { gd ->
                hi = maxOf(hi, a + ceil(gd).toLong() * TL_DAY_MS)
            }
        }
        (lo - 3 * TL_DAY_MS) to (hi + 7 * TL_DAY_MS)
    }
    var lo = loBase
    var hi = hiBase
    val dayWpx = with(density) { dayW.toPx() }
    // Cap the window so the track can't exceed Compose's Constraints limit; keep the
    // recent/future end (anchored at hi) and drop the far past.
    val maxDays = (TL_MAX_AXIS_PX / dayWpx).toInt().coerceAtLeast(TL_MIN_DAYS)
    var rawCount = ((hi - lo) / TL_DAY_MS).toInt() + 1
    if (rawCount > maxDays) {
        lo = hi - (maxDays - 1).toLong() * TL_DAY_MS
        rawCount = maxDays
    }
    val rangeStart = lo
    val dayCount = rawCount

    // Bars starting before the capped window pin to the left edge (still visible, no overflow).
    fun dayIndex(ms: Long): Int =
        (((tlDayFloor(ms) - rangeStart) / TL_DAY_MS).toInt()).coerceIn(0, dayCount - 1)
    val axisW = dayW * dayCount
    val todayLeft = dayW * dayIndex(todayMs) + dayW * 0.5f
    val gridColor = c.border.copy(alpha = 0.45f)
    // Precompute day-cell metadata once per window — rebuilding Calendar objects for
    // every day on each zoom frame was the main pinch-lag culprit.
    val dayCells = remember(rangeStart, dayCount, todayMs) { buildDayCells(rangeStart, dayCount, todayMs) }
    val monthBands = remember(rangeStart, dayCount) { buildMonthBands(rangeStart, dayCount) }

    // Swimlanes follow the shared composer-bar grouping (status / tag[+prefix]) —
    // no separate timeline control (mirrors web; avoids duplicate grouping).
    fun tagColor(hex: String?): Color? = hex?.takeIf { it.isNotBlank() }?.let { parseHexColor(it, c.primary) }
    val lanes = remember(scheduled, state.groupMode, state.tagPrefix, state.tags, state.columns, state.members) {
        val mode = state.groupMode
        val members = state.membersMap
        val map = LinkedHashMap<String, Pair<Pair<String, Color?>, MutableList<Task>>>()
        fun bucket(key: String, label: String, color: Color?) =
            map.getOrPut(key) { (label to color) to mutableListOf() }.second
        // Status grouping seeds lanes in column order so empty columns still show.
        if (mode == "status") {
            for (col in state.sortedColumns) bucket(col.id, col.name, tagColor(col.color))
        }
        for (t in scheduled) {
            when (mode) {
                "tag" -> {
                    val id = t.tagIds.firstOrNull { tid ->
                        val tag = state.tags[tid]
                        tag != null && (state.tagPrefix.isEmpty() || tag.name.startsWith(state.tagPrefix))
                    }
                    val tag = id?.let { state.tags[it] }
                    bucket(id ?: "∅", tag?.name ?: "Без тега", tagColor(tag?.color)).add(t)
                }

                "assignee" -> {
                    val id = t.assigneeIds.firstOrNull()
                    val m = id?.let { members[it] }
                    bucket(id ?: "∅", m?.name ?: "Не назначено", null).add(t)
                }

                "none" -> bucket("all", "Все задачи", null).add(t)

                else -> {
                    val col = state.sortedColumns.find { it.id == t.columnId }
                    bucket(t.columnId.ifBlank { "∅" }, col?.name ?: "—", tagColor(col?.color)).add(t)
                }
            }
        }
        // Lane tasks keep the incoming (composer-sorted) order; re-sorting by start
        // here would override an explicit «Сорт: Статус» etc. (mirrors web fix).
        map.entries
            .map { (k, v) -> TLane(k, v.first.first, v.first.second, v.second.toList()) }
            .filter { it.tasks.isNotEmpty() || mode == "status" }
            .sortedBy { if (it.key == "∅") 1 else 0 }
    }
    // Flattened rows for the virtualized body (lane header + its task rows).
    val bodyRows = remember(lanes) {
        buildList {
            for (lane in lanes) {
                add(TlLaneHeaderRow(lane))
                for (t in lane.tasks) add(TlTaskRow(t))
            }
        }
    }

    val overdue = remember(scheduled) { scheduled.count { it.dueDate != null && !it.isCompleted && isOverdue(it.dueDate) } }
    val hScroll = rememberScrollState()

    // Apply a new day-width while keeping the date at [focalTrackX] (a track-relative x)
    // pinned. The compensating scroll is dispatched SYNCHRONOUSLY in the SAME frame as
    // the dayW change (dispatchRawDelta) — doing it in an async re-anchor made the chart
    // visibly shift then snap back ("jitter") on every zoom step (buttons + pinch).
    fun zoomTo(newDayW: androidx.compose.ui.unit.Dp, focalTrackX: Float) {
        val clamped = newDayW.coerceIn(TL_DAY_W_MIN, TL_DAY_W_MAX)
        if (clamped == dayW || viewportPx <= 0) return
        val oldPx = with(density) { dayW.toPx() }
        val newPx = with(density) { clamped.toPx() }
        val focalDay = (hScroll.value + focalTrackX) / oldPx
        anchorScreenX = focalTrackX
        anchorMs = rangeStart + (focalDay * TL_DAY_MS).toLong()
        dayW = clamped
        hScroll.dispatchRawDelta(focalDay * (newPx - oldPx))
    }

    // Pinch-zoom: the FIRST applied step fixes the focal under the finger centroid;
    // later steps reuse it. 1-finger scroll passes through (pinchZoom).
    fun applyZoom(zoomChange: Float, centroidX: Float, isStart: Boolean) {
        val focalTrackX = if (isStart && viewportPx > 0) {
            val leftWpx = with(density) { leftW.toPx() }
            (centroidX - leftWpx).coerceIn(0f, viewportPx.toFloat())
        } else if (anchorScreenX >= 0f) {
            anchorScreenX
        } else {
            viewportPx / 2f
        }
        zoomTo(dayW * zoomChange, focalTrackX)
    }
    // Anchor on the initial open / viewport resize / window-origin (rangeStart) change —
    // NOT on dayW (zoomTo handles zoom synchronously). awaitTrackLayout waits for the
    // track to measure so maxValue is valid. viewportPx MUST be the visible width (its
    // onSizeChanged sits BEFORE horizontalScroll, else it reports content width → the
    // centre offset would scale with zoom/filters → wrong month).
    LaunchedEffect(viewportPx, rangeStart) {
        if (viewportPx <= 0) return@LaunchedEffect
        awaitTrackLayout(hScroll)
        val px = with(density) { dayW.toPx() }
        val centerMs = if (anchorMs >= 0L) anchorMs else todayMs
        val day = (centerMs - rangeStart).toFloat() / TL_DAY_MS
        val sx = if (anchorScreenX >= 0f) anchorScreenX else viewportPx / 2f
        hScroll.scrollTo((day * px - sx).roundToInt().coerceIn(0, hScroll.maxValue))
    }

    fun scrollToToday() {
        anchorMs = todayMs
        anchorScreenX = -1f
        val px = with(density) { dayW.toPx() }
        scope.launch {
            awaitTrackLayout(hScroll)
            val target = (dayIndex(todayMs) * px - viewportPx / 2f).roundToInt().coerceIn(0, hScroll.maxValue)
            hScroll.animateScrollTo(target)
        }
    }

    Column(Modifier.fillMaxSize().background(c.bg).padding(horizontal = 8.dp, vertical = 6.dp)) {
        // ── toolbar ──
        Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
            Box(
                Modifier.clip(RoundedCornerShape(RadiusSm)).border(1.dp, c.border, RoundedCornerShape(RadiusSm))
                    .clickableNoRipple { scrollToToday() }.padding(horizontal = 12.dp, vertical = 6.dp),
            ) { Text("Сегодня", color = c.text2, fontSize = 13.sp) }
            Spacer(Modifier.width(8.dp))
            ZoomBtn("−") { zoomTo(dayW - 6.dp, viewportPx / 2f) }
            Spacer(Modifier.width(4.dp))
            ZoomBtn("+") { zoomTo(dayW + 6.dp, viewportPx / 2f) }
            Spacer(Modifier.width(8.dp))
            // collapse / expand the left task column (animated)
            Box(
                Modifier.size(28.dp).clip(RoundedCornerShape(RadiusSm))
                    .border(1.dp, c.border, RoundedCornerShape(RadiusSm))
                    .clickableNoRipple { leftCollapsed = !leftCollapsed },
                contentAlignment = Alignment.Center,
            ) {
                IonIcon(
                    Ion.CHEVRON_FORWARD, size = 16.dp, tint = c.text2,
                    modifier = if (!leftCollapsed) Modifier.rotate(180f) else Modifier,
                )
            }
            Spacer(Modifier.weight(1f))
            if (overdue > 0) {
                TimelineCounter("$overdue просрочено", Color(0xFFE0533D), c.primary.copy(alpha = 0f))
                Spacer(Modifier.width(6.dp))
            }
            if (unscheduled.isNotEmpty()) TimelineCounter("${unscheduled.size} без дат", c.text3, c.surfaceAlt)
        }
        Spacer(Modifier.height(8.dp))

        // ── chart area: rounded + bordered, like the web .tl-scroll ──
        Column(
            Modifier.weight(1f).clip(RoundedCornerShape(12.dp))
                .border(1.dp, c.border, RoundedCornerShape(12.dp)),
        ) {
            // ── header: sticky months + days (horizontal-scrolls with the body) ──
            Row {
                Box(
                    Modifier.width(leftW).height(TL_HEAD_H).background(c.surfaceAlt).clipToBounds()
                        .padding(horizontal = 10.dp, vertical = 8.dp),
                    contentAlignment = Alignment.BottomStart,
                ) { Text("Задача", color = c.text3, fontSize = 12.sp, fontWeight = FontWeight.SemiBold, maxLines = 1) }
                Box(Modifier.weight(1f).height(TL_HEAD_H).onSizeChanged { viewportPx = it.width }.clipToBounds()) {
                    TimelineAxisCanvas(dayCells, monthBands, dayW, hScroll, Modifier.fillMaxSize())
                }
            }

            // ── body (pinch-to-zoom; canPan=false so 1-finger scroll passes through) ──
            if (lanes.isEmpty()) {
                Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                    Text("Нет задач со сроками.\nЗадайте начало или срок в карточке.", color = c.text3, fontSize = 14.sp)
                }
            } else {
                LazyColumn(Modifier.weight(1f).pinchZoom { z, cx, s -> applyZoom(z, cx, s) }) {
                    timelineBodyItems(
                        bodyRows, leftW, dayW, axisW, dayWpx, todayLeft, gridColor,
                        hScroll, viewportPx, state, { span(it) }, { dayIndex(it) }, onOpenTask,
                    )
                }
            }
        }

        // ── unscheduled ──
        if (unscheduled.isNotEmpty()) {
            Spacer(Modifier.height(8.dp))
            Row(
                Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text("Без дат", color = c.text3, fontSize = 12.sp, fontWeight = FontWeight.SemiBold)
                Spacer(Modifier.width(8.dp))
                unscheduled.forEach { t ->
                    val accent = PriorityColors.getOrElse(t.priority) { PriorityColors[0] }
                    Row(
                        Modifier.padding(end = 6.dp).clip(RoundedCornerShape(RadiusSm))
                            .background(c.surfaceAlt).clickableNoRipple { onOpenTask(t) }
                            .padding(horizontal = 8.dp, vertical = 5.dp),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        Box(Modifier.width(3.dp).height(14.dp).clip(RoundedCornerShape(2.dp)).background(accentGradient(accent)))
                        Spacer(Modifier.width(6.dp))
                        Text(t.title, color = c.text1, fontSize = 12.sp, maxLines = 1)
                    }
                }
            }
        }
    }
}

/** One dependency arrow, blocker's finish → blocked's start (positions in Dp). */
private data class GanttArrow(val x1: Dp, val y1: Dp, val x2: Dp, val y2: Dp)

/**
 * Gantt view = the Timeline (time axis / bars / zoom / today-line / swimlanes)
 * plus task **dependencies** drawn as finish-to-start arrows over the track.
 * Unlike web there is no in-bar drag-to-link (board drag is 1D-lane-tuned, the
 * h-scrolling track fights an h-drag — same call as the timeline/matrix): links
 * are created/removed in the task modal's «Связи» tab (tap a bar to open it).
 *
 * Shares the timeline's virtualized body (`timelineBodyItems` in a LazyColumn) and
 * overlays the arrows on a single Canvas. LazyColumn doesn't compose off-screen rows,
 * so arrow endpoints come from the precomputed `rowTops`/`itemTopsPx` + the list's
 * scroll offset (not from composed bars), clipped to the visible track.
 */
@Composable
fun BoardGanttView(state: BoardUiState, vm: BoardViewModel, onOpenTask: (Task) -> Unit) {
    val c = Tessera.colors
    val density = LocalDensity.current
    val scope = rememberCoroutineScope()
    // See BoardTimelineView for the pinch-zoom anchoring model (the `zoom`/anchor
    // holder is defined below, once rangeStart is known).
    var dayW by remember { mutableStateOf(TL_DAY_W) }
    var viewportPx by remember { mutableStateOf(0) }
    var anchorMs by remember { mutableStateOf(-1L) }
    // Track-relative x the anchor date should stay at through a zoom (-1 = viewport
    // centre). Set to the pinch focal point so zoom converges where the fingers are.
    var anchorScreenX by remember { mutableStateOf(-1f) }
    // Collapsible left task/group column — animated to 0 to give the chart full width.
    var leftCollapsed by remember { mutableStateOf(false) }
    val leftW by animateDpAsState(if (leftCollapsed) 0.dp else TL_LEFT_W, label = "tlLeftW")

    // Memoised so zoom/scroll recompositions don't re-filter+sort 200 tasks each frame.
    val tasks = remember(state.tasks, state.filter, state.sortLevels) { state.applyFilterSort(state.tasks) }
    val scheduled = tasks.filter { it.startDate != null || it.dueDate != null }
    val unscheduled = tasks.filter { it.startDate == null && it.dueDate == null }

    fun span(t: Task): Pair<Long, Long> {
        val s = parseInstantMillis(t.startDate)
        val d = parseInstantMillis(t.dueDate)
        val a = s ?: d ?: 0L
        val b = d ?: s ?: 0L
        return tlDayFloor(a) to tlDayFloor(b)
    }

    val todayMs = tlDayFloor(System.currentTimeMillis())
    // Memoised: the span()/estimate loop parses date strings for every task — doing it
    // each zoom frame (200 tasks) was a big hitch. Depends only on the task set, not dayW.
    val (loBase, hiBase) = remember(scheduled, state.estimation, todayMs) {
        var lo = todayMs
        var hi = todayMs
        scheduled.forEach {
            val (a, b) = span(it)
            lo = minOf(lo, a)
            hi = maxOf(hi, b)
            website.msdnna.tessera.util.Estimation.toDays(it.estimate, state.estimation)?.let { gd ->
                hi = maxOf(hi, a + ceil(gd).toLong() * TL_DAY_MS)
            }
        }
        (lo - 3 * TL_DAY_MS) to (hi + 7 * TL_DAY_MS)
    }
    var lo = loBase
    var hi = hiBase
    val dayWpx = with(density) { dayW.toPx() }
    // Cap the window so the track (a single axisW×bodyH box + arrow Canvas) can't
    // exceed Compose's Constraints limit; keep the recent/future end (anchored at hi).
    val maxDays = (TL_MAX_AXIS_PX / dayWpx).toInt().coerceAtLeast(TL_MIN_DAYS)
    var rawCount = ((hi - lo) / TL_DAY_MS).toInt() + 1
    if (rawCount > maxDays) {
        lo = hi - (maxDays - 1).toLong() * TL_DAY_MS
        rawCount = maxDays
    }
    val rangeStart = lo
    val dayCount = rawCount

    // Bars starting before the capped window pin to the left edge (still visible, no overflow).
    fun dayIndex(ms: Long): Int =
        (((tlDayFloor(ms) - rangeStart) / TL_DAY_MS).toInt()).coerceIn(0, dayCount - 1)
    val axisW = dayW * dayCount
    val todayLeft = dayW * dayIndex(todayMs) + dayW * 0.5f
    val gridColor = c.border.copy(alpha = 0.45f)
    val dayCells = remember(rangeStart, dayCount, todayMs) { buildDayCells(rangeStart, dayCount, todayMs) }
    val monthBands = remember(rangeStart, dayCount) { buildMonthBands(rangeStart, dayCount) }

    fun tagColor(hex: String?): Color? = hex?.takeIf { it.isNotBlank() }?.let { parseHexColor(it, c.primary) }
    val lanes = remember(scheduled, state.groupByTag, state.tagPrefix, state.tags, state.columns) {
        val map = LinkedHashMap<String, Pair<Pair<String, Color?>, MutableList<Task>>>()
        fun bucket(key: String, label: String, color: Color?) =
            map.getOrPut(key) { (label to color) to mutableListOf() }.second
        if (!state.groupByTag) {
            for (col in state.sortedColumns) bucket(col.id, col.name, tagColor(col.color))
        }
        for (t in scheduled) {
            if (state.groupByTag) {
                val id = t.tagIds.firstOrNull { tid ->
                    val tag = state.tags[tid]
                    tag != null && (state.tagPrefix.isEmpty() || tag.name.startsWith(state.tagPrefix))
                }
                val tag = id?.let { state.tags[it] }
                bucket(id ?: "∅", tag?.name ?: "Без тега", tagColor(tag?.color)).add(t)
            } else {
                val col = state.sortedColumns.find { it.id == t.columnId }
                bucket(t.columnId.ifBlank { "∅" }, col?.name ?: "—", tagColor(col?.color)).add(t)
            }
        }
        map.entries
            .map { (k, v) -> TLane(k, v.first.first, v.first.second, v.second.toList()) }
            .filter { it.tasks.isNotEmpty() || !state.groupByTag }
            .sortedBy { if (it.key == "∅") 1 else 0 }
    }

    // Flattened rows for the virtualized body (shared with the timeline).
    val bodyRows = remember(lanes) {
        buildList {
            for (lane in lanes) {
                add(TlLaneHeaderRow(lane))
                for (t in lane.tasks) add(TlTaskRow(t))
            }
        }
    }
    // Row-top (Dp) of every task, walking lanes in render order, so the arrow
    // Canvas geometry matches the bar rows (offsets independent of composition).
    val rowTops = remember(lanes) {
        val m = LinkedHashMap<String, Dp>()
        var y = 0.dp
        for (lane in lanes) {
            y += TL_LANE_H
            for (t in lane.tasks) {
                m[t.id] = y
                y += TL_ROW_H
            }
        }
        m
    }
    // Absolute px-top of every LazyColumn item (lane headers + tasks), so the arrow
    // Canvas can map a row to its on-screen Y from the list's scroll offset.
    val itemTopsPx = remember(bodyRows, density) {
        val laneH = with(density) { TL_LANE_H.toPx() }
        val rowH = with(density) { TL_ROW_H.toPx() }
        val out = FloatArray(bodyRows.size + 1)
        var y = 0f
        for (i in bodyRows.indices) {
            out[i] = y
            y += if (bodyRows[i] is TlLaneHeaderRow) laneH else rowH
        }
        out[bodyRows.size] = y
        out
    }

    // Normalise the board's blocking edges to blocker→blocked and project them to
    // arrow endpoints (finish of blocker → start of blocked). Skip dangling edges.
    val arrows = run {
        val byId = scheduled.associateBy { it.id }
        val seen = HashSet<String>()
        state.dependencies.mapNotNull { d ->
            val key = "${d.blockerId}>${d.blockedId}"
            if (!seen.add(key)) return@mapNotNull null
            val tb = byId[d.blockerId] ?: return@mapNotNull null
            val tk = byId[d.blockedId] ?: return@mapNotNull null
            val yb = rowTops[d.blockerId] ?: return@mapNotNull null
            val yk = rowTops[d.blockedId] ?: return@mapNotNull null
            val (ab, bb) = span(tb)
            val (ak, _) = span(tk)
            val i0b = dayIndex(ab)
            val i1b = dayIndex(bb)
            val i0k = dayIndex(ak)
            GanttArrow(
                x1 = dayW * (i1b + 1) - 2.dp, // blocker bar right edge
                y1 = yb + 19.dp, // bar centre (top 7 + height/2 12)
                x2 = dayW * i0k, // blocked bar left edge
                y2 = yk + 19.dp,
            )
        }
    }
    val arrowColor = c.primary

    val overdue = remember(scheduled) { scheduled.count { it.dueDate != null && !it.isCompleted && isOverdue(it.dueDate) } }
    val hScroll = rememberScrollState()

    // Apply a new day-width while keeping the date at [focalTrackX] (a track-relative x)
    // pinned. The compensating scroll is dispatched SYNCHRONOUSLY in the SAME frame as
    // the dayW change (dispatchRawDelta) — doing it in an async re-anchor made the chart
    // visibly shift then snap back ("jitter") on every zoom step (buttons + pinch).
    fun zoomTo(newDayW: androidx.compose.ui.unit.Dp, focalTrackX: Float) {
        val clamped = newDayW.coerceIn(TL_DAY_W_MIN, TL_DAY_W_MAX)
        if (clamped == dayW || viewportPx <= 0) return
        val oldPx = with(density) { dayW.toPx() }
        val newPx = with(density) { clamped.toPx() }
        val focalDay = (hScroll.value + focalTrackX) / oldPx
        anchorScreenX = focalTrackX
        anchorMs = rangeStart + (focalDay * TL_DAY_MS).toLong()
        dayW = clamped
        hScroll.dispatchRawDelta(focalDay * (newPx - oldPx))
    }

    // Pinch-zoom: the FIRST applied step fixes the focal under the finger centroid;
    // later steps reuse it. 1-finger scroll passes through (pinchZoom).
    fun applyZoom(zoomChange: Float, centroidX: Float, isStart: Boolean) {
        val focalTrackX = if (isStart && viewportPx > 0) {
            val leftWpx = with(density) { leftW.toPx() }
            (centroidX - leftWpx).coerceIn(0f, viewportPx.toFloat())
        } else if (anchorScreenX >= 0f) {
            anchorScreenX
        } else {
            viewportPx / 2f
        }
        zoomTo(dayW * zoomChange, focalTrackX)
    }
    // Anchor on the initial open / viewport resize / window-origin (rangeStart) change —
    // NOT on dayW (zoomTo handles zoom synchronously). awaitTrackLayout waits for the
    // track to measure so maxValue is valid. viewportPx MUST be the visible width (its
    // onSizeChanged sits BEFORE horizontalScroll, else it reports content width → the
    // centre offset would scale with zoom/filters → wrong month).
    LaunchedEffect(viewportPx, rangeStart) {
        if (viewportPx <= 0) return@LaunchedEffect
        awaitTrackLayout(hScroll)
        val px = with(density) { dayW.toPx() }
        val centerMs = if (anchorMs >= 0L) anchorMs else todayMs
        val day = (centerMs - rangeStart).toFloat() / TL_DAY_MS
        val sx = if (anchorScreenX >= 0f) anchorScreenX else viewportPx / 2f
        hScroll.scrollTo((day * px - sx).roundToInt().coerceIn(0, hScroll.maxValue))
    }

    fun scrollToToday() {
        anchorMs = todayMs
        anchorScreenX = -1f
        val px = with(density) { dayW.toPx() }
        scope.launch {
            awaitTrackLayout(hScroll)
            val target = (dayIndex(todayMs) * px - viewportPx / 2f).roundToInt().coerceIn(0, hScroll.maxValue)
            hScroll.animateScrollTo(target)
        }
    }

    Column(Modifier.fillMaxSize().background(c.bg).padding(horizontal = 8.dp, vertical = 6.dp)) {
        // ── toolbar ──
        Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
            Box(
                Modifier.clip(RoundedCornerShape(RadiusSm)).border(1.dp, c.border, RoundedCornerShape(RadiusSm))
                    .clickableNoRipple { scrollToToday() }.padding(horizontal = 12.dp, vertical = 6.dp),
            ) { Text("Сегодня", color = c.text2, fontSize = 13.sp) }
            Spacer(Modifier.width(8.dp))
            ZoomBtn("−") { zoomTo(dayW - 6.dp, viewportPx / 2f) }
            Spacer(Modifier.width(4.dp))
            ZoomBtn("+") { zoomTo(dayW + 6.dp, viewportPx / 2f) }
            Spacer(Modifier.width(8.dp))
            // collapse / expand the left task column (animated)
            Box(
                Modifier.size(28.dp).clip(RoundedCornerShape(RadiusSm))
                    .border(1.dp, c.border, RoundedCornerShape(RadiusSm))
                    .clickableNoRipple { leftCollapsed = !leftCollapsed },
                contentAlignment = Alignment.Center,
            ) {
                IonIcon(
                    Ion.CHEVRON_FORWARD, size = 16.dp, tint = c.text2,
                    modifier = if (!leftCollapsed) Modifier.rotate(180f) else Modifier,
                )
            }
            Spacer(Modifier.weight(1f))
            if (overdue > 0) {
                TimelineCounter("$overdue просрочено", Color(0xFFE0533D), c.primary.copy(alpha = 0f))
                Spacer(Modifier.width(6.dp))
            }
            if (arrows.isNotEmpty()) {
                TimelineCounter("${arrows.size} связей", c.text3, c.surfaceAlt)
                Spacer(Modifier.width(6.dp))
            }
            if (unscheduled.isNotEmpty()) TimelineCounter("${unscheduled.size} без дат", c.text3, c.surfaceAlt)
        }
        Spacer(Modifier.height(8.dp))

        // ── chart area: rounded + bordered, like the web .tl-scroll ──
        Column(
            Modifier.weight(1f).clip(RoundedCornerShape(12.dp))
                .border(1.dp, c.border, RoundedCornerShape(12.dp)),
        ) {
            // ── header: sticky months + days (h-scrolls with the body) ──
            Row {
                Box(
                    Modifier.width(leftW).height(TL_HEAD_H).background(c.surfaceAlt).clipToBounds()
                        .padding(horizontal = 10.dp, vertical = 8.dp),
                    contentAlignment = Alignment.BottomStart,
                ) { Text("Задача", color = c.text3, fontSize = 12.sp, fontWeight = FontWeight.SemiBold, maxLines = 1) }
                Box(Modifier.weight(1f).height(TL_HEAD_H).onSizeChanged { viewportPx = it.width }.clipToBounds()) {
                    TimelineAxisCanvas(dayCells, monthBands, dayW, hScroll, Modifier.fillMaxSize())
                }
            }

            if (lanes.isEmpty()) {
                Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                    Text("Нет задач со сроками.\nЗадайте начало или срок в карточке.", color = c.text3, fontSize = 14.sp)
                }
            } else {
                // Body = shared virtualized timeline rows + an arrow Canvas overlay.
                // (LazyColumn won't compose off-screen rows, so arrows are drawn from the
                //  precomputed rowTops + the list's scroll offset, not from composed bars.)
                val gLazy = rememberLazyListState()
                Box(Modifier.weight(1f)) {
                    LazyColumn(
                        Modifier.fillMaxSize().pinchZoom { z, cx, s -> applyZoom(z, cx, s) },
                        state = gLazy,
                    ) {
                        timelineBodyItems(
                            bodyRows, leftW, dayW, axisW, dayWpx, todayLeft, gridColor,
                            hScroll, viewportPx, state, { span(it) }, { dayIndex(it) }, onOpenTask,
                        )
                    }
                    // dependency arrows over the visible rows (clipped to the track area)
                    Canvas(Modifier.fillMaxSize()) {
                        val first = gLazy.layoutInfo.visibleItemsInfo.firstOrNull() ?: return@Canvas
                        val viewportTopAbs = itemTopsPx[first.index] - first.offset
                        val leftWpx = leftW.toPx()
                        val hOff = hScroll.value.toFloat()
                        clipRect(left = leftWpx, top = 0f, right = size.width, bottom = size.height) {
                            arrows.forEach { s ->
                                val x1 = leftWpx + s.x1.toPx() - hOff
                                val y1 = s.y1.toPx() - viewportTopAbs
                                val x2 = leftWpx + s.x2.toPx() - hOff
                                val y2 = s.y2.toPx() - viewportTopAbs
                                if (maxOf(y1, y2) < 0f || minOf(y1, y2) > size.height) return@forEach
                                val dx = maxOf(22.dp.toPx(), kotlin.math.abs(x2 - x1) * 0.4f)
                                val path = Path().apply {
                                    moveTo(x1, y1)
                                    cubicTo(x1 + dx, y1, x2 - dx, y2, x2, y2)
                                }
                                drawPath(path, color = arrowColor.copy(alpha = 0.7f), style = Stroke(width = 1.6.dp.toPx()))
                                val hw = 7.dp.toPx()
                                val hh = 4.dp.toPx()
                                val head = Path().apply {
                                    moveTo(x2, y2)
                                    lineTo(x2 - hw, y2 - hh)
                                    lineTo(x2 - hw, y2 + hh)
                                    close()
                                }
                                drawPath(head, color = arrowColor)
                            }
                        }
                    }
                }
            }
        }

        // ── unscheduled ──
        if (unscheduled.isNotEmpty()) {
            Spacer(Modifier.height(8.dp))
            Row(
                Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text("Без дат", color = c.text3, fontSize = 12.sp, fontWeight = FontWeight.SemiBold)
                Spacer(Modifier.width(8.dp))
                unscheduled.forEach { t ->
                    val accent = PriorityColors.getOrElse(t.priority) { PriorityColors[0] }
                    Row(
                        Modifier.padding(end = 6.dp).clip(RoundedCornerShape(RadiusSm))
                            .background(c.surfaceAlt).clickableNoRipple { onOpenTask(t) }
                            .padding(horizontal = 8.dp, vertical = 5.dp),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        Box(Modifier.width(3.dp).height(14.dp).clip(RoundedCornerShape(2.dp)).background(accentGradient(accent)))
                        Spacer(Modifier.width(6.dp))
                        Text(t.title, color = c.text1, fontSize = 12.sp, maxLines = 1)
                    }
                }
            }
        }
    }
}

@Composable
private fun TimelineCounter(text: String, fg: Color, bg: Color) {
    Box(
        Modifier.clip(RoundedCornerShape(20.dp))
            .background(if (fg == Color(0xFFE0533D)) Color(0xFFE0533D).copy(alpha = 0.12f) else bg)
            .padding(horizontal = 10.dp, vertical = 3.dp),
    ) { Text(text, color = fg, fontSize = 11.sp) }
}

/** A month label spanning [span] consecutive day-cells starting at [startIdx]. */
private data class TlMonthBand(val label: String, val startIdx: Int, val span: Int)

private fun buildMonthBands(rangeStart: Long, dayCount: Int): List<TlMonthBand> {
    val out = ArrayList<TlMonthBand>()
    val cal = Calendar.getInstance()
    for (i in 0 until dayCount) {
        cal.timeInMillis = rangeStart + i * TL_DAY_MS
        val label = "${TlMonths[cal.get(Calendar.MONTH)]} ${cal.get(Calendar.YEAR)}"
        val last = out.lastOrNull()
        if (last != null && last.label == label) out[out.lastIndex] = last.copy(span = last.span + 1)
        else out.add(TlMonthBand(label, i, 1))
    }
    return out
}

private fun axisPaint(color: Color, sizeSp: Float, density: Density, bold: Boolean, center: Boolean): Paint =
    Paint(Paint.ANTI_ALIAS_FLAG).apply {
        this.color = color.toArgb()
        textSize = with(density) { sizeSp.sp.toPx() }
        textAlign = if (center) Paint.Align.CENTER else Paint.Align.LEFT
        if (bold) typeface = Typeface.DEFAULT_BOLD
    }

/**
 * The whole day-axis header (month band + day numbers + weekday + today circle +
 * weekend bg) drawn in ONE Canvas, only for the visible window (reads `scroll.value`
 * in the draw phase). Replaces ~300+ composables that re-laid-out on every zoom step
 * — the zoom/collapse hitch. Text via the native Canvas (cheap, no per-cell layout).
 */
@Composable
private fun TimelineAxisCanvas(
    days: List<TlDayCell>,
    months: List<TlMonthBand>,
    dayW: androidx.compose.ui.unit.Dp,
    scroll: ScrollState,
    modifier: Modifier,
) {
    val c = Tessera.colors
    val density = LocalDensity.current
    val dayNum = remember(c.text1) { axisPaint(c.text1, 11f, density, bold = false, center = true) }
    val dayNumToday = remember(c.onPrimary) { axisPaint(c.onPrimary, 11f, density, bold = false, center = true) }
    val weekday = remember(c.text3) { axisPaint(c.text3, 9f, density, bold = false, center = true) }
    val monthPaint = remember(c.text2) { axisPaint(c.text2, 11f, density, bold = true, center = false) }
    Canvas(modifier) {
        val dayWpx = dayW.toPx()
        if (dayWpx <= 0f || days.isEmpty()) return@Canvas
        val scrollX = scroll.value.toFloat()
        val w = size.width
        val monthH = TL_MONTH_H.toPx()
        val daysTop = monthH
        val daysH = size.height - monthH
        val canvas = drawContext.canvas.nativeCanvas

        // ── month band ──
        for (b in months) {
            val x0 = b.startIdx * dayWpx - scrollX
            val bw = b.span * dayWpx
            if (x0 + bw < 0f || x0 > w) continue
            drawRect(c.surfaceAlt, topLeft = Offset(x0, 0f), size = Size(bw, monthH))
            drawLine(c.border, Offset(x0 + bw, 0f), Offset(x0 + bw, monthH), strokeWidth = 1f)
            val lx = maxOf(x0, 0f) + 6.dp.toPx()
            val ly = monthH / 2f - (monthPaint.descent() + monthPaint.ascent()) / 2f
            canvas.drawText(b.label, lx, ly, monthPaint)
        }

        // ── day cells (visible range only) ──
        val from = (scrollX / dayWpx).toInt().coerceIn(0, days.size - 1)
        val to = ((scrollX + w) / dayWpx).toInt().coerceIn(0, days.size - 1)
        val circleR = 9.dp.toPx()
        val numCy = daysTop + daysH * 0.36f
        val wdCy = daysTop + daysH * 0.78f
        for (i in from..to) {
            val cell = days[i]
            val x0 = i * dayWpx - scrollX
            val cx = x0 + dayWpx / 2f
            drawRect(if (cell.weekend) c.bg else c.surface, topLeft = Offset(x0, daysTop), size = Size(dayWpx, daysH))
            if (cell.isToday) drawCircle(c.primary, radius = circleR, center = Offset(cx, numCy))
            val np = if (cell.isToday) dayNumToday else dayNum
            canvas.drawText("${cell.dom}", cx, numCy - (np.descent() + np.ascent()) / 2f, np)
            canvas.drawText(cell.weekday, cx, wdCy - (weekday.descent() + weekday.ascent()) / 2f, weekday)
        }
    }
}

/**
 * Wait until the horizontal track is laid out so [ScrollState.maxValue] is valid.
 * A single `withFrameNanos` resumes BEFORE the frame's layout pass, so the very
 * first scroll would see maxValue==0 and clamp to 0 (= the far-left day, which
 * varies with zoom/filters — the reported "Сегодня jumps to 2025" bug). Poll a few
 * frames; bail after 8 (a board that fits the viewport legitimately has maxValue 0).
 */
private suspend fun awaitTrackLayout(scroll: ScrollState) {
    var tries = 0
    while (scroll.maxValue == 0 && tries < 8) {
        withFrameNanos {}
        tries++
    }
}

/**
 * Shared virtualized body rows for the timeline AND Gantt (the Gantt overlays an
 * arrow Canvas on top). Lane-header + task rows, each `[fixed left | h-scrolling
 * track]`. Only visible rows compose (LazyColumn) — the ~200-task perf + open-freeze fix.
 */
@Suppress("LongParameterList")
private fun LazyListScope.timelineBodyItems(
    rows: List<TlBodyRow>,
    leftW: androidx.compose.ui.unit.Dp,
    dayW: androidx.compose.ui.unit.Dp,
    axisW: androidx.compose.ui.unit.Dp,
    dayWpx: Float,
    todayLeft: androidx.compose.ui.unit.Dp,
    gridColor: Color,
    hScroll: ScrollState,
    viewportPx: Int,
    state: BoardUiState,
    span: (Task) -> Pair<Long, Long>,
    dayIndex: (Long) -> Int,
    onOpenTask: (Task) -> Unit,
) {
    items(
        rows,
        key = { row ->
            when (row) {
                is TlLaneHeaderRow -> "L:${row.lane.key}"
                is TlTaskRow -> "T:${row.task.id}"
            }
        },
    ) { row ->
        val c = Tessera.colors
        when (row) {
            is TlLaneHeaderRow -> {
                val lane = row.lane
                Row {
                    Row(
                        Modifier.width(leftW).height(TL_LANE_H).background(c.surfaceAlt).clipToBounds()
                            .padding(horizontal = 10.dp),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        Box(
                            Modifier.size(9.dp).clip(RoundedCornerShape(3.dp))
                                .background(accentGradient(lane.color ?: c.primary)),
                        )
                        Spacer(Modifier.width(7.dp))
                        Text(
                            lane.label, color = c.text2, fontSize = 12.sp, fontWeight = FontWeight.SemiBold,
                            maxLines = 1, modifier = Modifier.weight(1f),
                        )
                        Spacer(Modifier.width(6.dp))
                        Text("${lane.tasks.size}", color = c.text3, fontSize = 11.sp)
                        laneEffort(lane, state)?.let { eff ->
                            Spacer(Modifier.width(6.dp))
                            IonIcon(Ion.TIME, size = 11.dp, tint = c.text2)
                            Spacer(Modifier.width(3.dp))
                            Text(eff, color = c.text2, fontSize = 11.sp, maxLines = 1)
                        }
                    }
                    Box(Modifier.weight(1f).horizontalScroll(hScroll)) {
                        Box(Modifier.width(axisW).height(TL_LANE_H).background(c.surfaceAlt).tlGrid(dayWpx, gridColor, hScroll, viewportPx)) {
                            Box(Modifier.offset(x = todayLeft).width(1.5.dp).fillMaxHeight().background(c.primary.copy(alpha = 0.55f)))
                        }
                    }
                }
            }

            is TlTaskRow -> {
                val t = row.task
                val (a, b) = span(t)
                val i0 = dayIndex(a)
                val i1 = dayIndex(b)
                val accent = PriorityColors.getOrElse(t.priority) { PriorityColors[0] }
                Row {
                    Row(
                        Modifier.width(leftW).height(TL_ROW_H).clipToBounds()
                            .clickableNoRipple { onOpenTask(t) }
                            .padding(horizontal = 10.dp),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        Box(
                            Modifier.width(3.dp).height(16.dp).clip(RoundedCornerShape(2.dp))
                                .background(accentGradient(accent)),
                        )
                        Spacer(Modifier.width(7.dp))
                        Text(
                            t.title,
                            color = if (t.isCompleted) c.text3 else c.text1,
                            fontSize = 13.sp, maxLines = 1,
                            textDecoration = if (t.isCompleted) TextDecoration.LineThrough else null,
                        )
                    }
                    Box(Modifier.weight(1f).horizontalScroll(hScroll)) {
                        Box(Modifier.width(axisW).height(TL_ROW_H).tlGrid(dayWpx, gridColor, hScroll, viewportPx)) {
                            Box(Modifier.offset(x = todayLeft).width(1.5.dp).fillMaxHeight().background(c.primary.copy(alpha = 0.4f)))
                            Estimation.toDays(t.estimate, state.estimation)?.let { gd ->
                                val lbl = Estimation.format(t.estimate, state.estimation)
                                GhostBar(dayW * i0, (dayW * gd.toFloat()).coerceAtLeast(dayW), accent, lbl)
                            }
                            Box(
                                Modifier.offset(x = dayW * i0, y = 7.dp)
                                    .width((dayW * (i1 - i0 + 1)) - 2.dp).height(24.dp)
                                    .clip(RoundedCornerShape(6.dp))
                                    .alpha(if (t.isCompleted) 0.5f else 1f)
                                    .background(accentGradient(accent))
                                    .clickableNoRipple { onOpenTask(t) }
                                    .padding(horizontal = 7.dp),
                                contentAlignment = Alignment.CenterStart,
                            ) {
                                Text(
                                    t.title, color = Color.White, fontSize = 12.sp,
                                    fontWeight = FontWeight.Medium, maxLines = 1,
                                )
                            }
                        }
                    }
                }
            }
        }
    }
}

/** Precomputed per-day header metadata, built once per window (cheap zoom frames). */
private data class TlDayCell(val dom: Int, val weekday: String, val weekend: Boolean, val isToday: Boolean)

private fun buildDayCells(rangeStart: Long, dayCount: Int, todayMs: Long): List<TlDayCell> {
    val out = ArrayList<TlDayCell>(dayCount)
    val cal = Calendar.getInstance()
    for (i in 0 until dayCount) {
        cal.timeInMillis = rangeStart + i * TL_DAY_MS
        val dow = cal.get(Calendar.DAY_OF_WEEK)
        out.add(
            TlDayCell(
                dom = cal.get(Calendar.DAY_OF_MONTH),
                weekday = TlWeekdays[(dow + 5) % 7],
                weekend = dow == Calendar.SATURDAY || dow == Calendar.SUNDAY,
                isToday = tlDayFloor(cal.timeInMillis) == todayMs,
            ),
        )
    }
    return out
}

/**
 * Ghost "estimate" envelope: a dashed bar starting at the task's span start and as
 * wide as the estimate implies (calendar days), drawn behind the real bar so you
 * can see whether the planned start→due window matches the effort. Mirrors web.
 */
@Composable
private fun GhostBar(
    barLeft: androidx.compose.ui.unit.Dp,
    estWidth: androidx.compose.ui.unit.Dp,
    color: Color,
    label: String,
) {
    val dash = remember { PathEffect.dashPathEffect(floatArrayOf(6f, 5f), 0f) }
    // Frame the estimate envelope with a small margin on every side (matching the
    // top/bottom inset over the bar), a touch more fill, and a slightly thicker dash.
    Box(
        Modifier.offset(x = barLeft - 3.dp, y = 4.dp).width(estWidth + 6.dp).height(30.dp).drawBehind {
            val r = CornerRadius(7.dp.toPx(), 7.dp.toPx())
            drawRoundRect(color = color.copy(alpha = 0.14f), cornerRadius = r)
            drawRoundRect(
                color = color.copy(alpha = 0.65f), cornerRadius = r,
                style = Stroke(width = 2.dp.toPx(), pathEffect = dash),
            )
        },
        contentAlignment = Alignment.CenterEnd,
    ) {
        // Estimate value + timer icon at the right edge of the dashed envelope.
        if (label.isNotBlank()) {
            Row(
                Modifier.padding(end = 6.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                IonIcon(Ion.TIME, size = 10.dp, tint = color)
                Spacer(Modifier.width(2.dp))
                Text(label, color = color, fontSize = 9.sp, fontWeight = FontWeight.SemiBold, maxLines = 1)
            }
        }
    }
}

private val TlMonths = listOf("янв", "фев", "мар", "апр", "май", "июн", "июл", "авг", "сен", "окт", "ноя", "дек")
private val TlWeekdays = listOf("пн", "вт", "ср", "чт", "пт", "сб", "вс")

private val TL_DAY_W_MIN = 14.dp
private val TL_DAY_W_MAX = 64.dp

/** A small −/+ zoom button for the timeline toolbar. */
@Composable
private fun ZoomBtn(glyph: String, onClick: () -> Unit) {
    val c = Tessera.colors
    Box(
        Modifier.size(28.dp).clip(RoundedCornerShape(RadiusSm)).border(1.dp, c.border, RoundedCornerShape(RadiusSm))
            .clickableNoRipple(onClick = onClick),
        contentAlignment = Alignment.Center,
    ) { Text(glyph, color = c.text2, fontSize = 17.sp, fontWeight = FontWeight.SemiBold) }
}

/**
 * Two-finger pinch zoom that coexists with nested scrolling. `transformable` keeps
 * losing the gesture to the row's `horizontalScroll` / the LazyColumn's vertical
 * scroll (two fingers get read as a pan). Here we watch the INITIAL pointer pass and,
 * once ≥2 fingers are down, consume their changes ourselves — so the scrollables never
 * see a drag for a pinch, while a 1-finger drag passes straight through to them.
 * Direction-agnostic (zoom = ratio of the finger-distance change).
 */
@Composable
private fun Modifier.pinchZoom(onZoom: (zoom: Float, centroidX: Float, isStart: Boolean) -> Unit): Modifier {
    val zoomFn = rememberUpdatedState(onZoom)
    return this.pointerInput(Unit) {
        awaitEachGesture {
            awaitFirstDown(requireUnconsumed = false, pass = PointerEventPass.Initial)
            // The focal anchor is computed ONCE, on the first applied 2-finger step of
            // this gesture (isStart), and never again — even if the pressed count briefly
            // dips below 2 (touch jitter). `anchored` latches for the whole gesture so the
            // focal point stays put; recomputing it each frame made the timeline shudder.
            var anchored = false
            do {
                val event = awaitPointerEvent(PointerEventPass.Initial)
                if (event.changes.count { it.pressed } >= 2) {
                    val zoom = event.calculateZoom()
                    if (zoom != 1f) {
                        zoomFn.value(zoom, event.calculateCentroid().x, !anchored)
                        anchored = true
                        event.changes.forEach { it.consume() }
                    }
                }
            } while (event.changes.any { it.pressed })
        }
    }
}

/**
 * Faint vertical day gridlines behind a timeline track (web parity). Only the
 * VISIBLE day window is drawn (the track is up to ~30k px wide; drawing every line
 * across it was ~700 drawLine calls per row per frame). `scroll.value` is read in
 * the draw phase (not composition) so scrolling re-draws but never recomposes.
 */
private fun Modifier.tlGrid(dayWpx: Float, color: Color, scroll: ScrollState, viewportPx: Int): Modifier = drawBehind {
    if (dayWpx <= 0f) return@drawBehind
    val fromX = scroll.value.toFloat()
    val end = minOf(fromX + viewportPx, size.width)
    var x = ceil(fromX / dayWpx).toInt().coerceAtLeast(1) * dayWpx
    while (x <= end + 0.5f) {
        drawLine(color, Offset(x, 0f), Offset(x, size.height), strokeWidth = 1f)
        x += dayWpx
    }
}

@Composable
fun BoardEmpty(message: String) {
    val c = Tessera.colors
    Box(Modifier.fillMaxSize().background(c.bg), contentAlignment = Alignment.Center) {
        Text(message, color = c.text3, fontSize = 14.sp)
    }
}
