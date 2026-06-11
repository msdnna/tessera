package website.msdnna.tessera.ui.screens

import androidx.compose.animation.core.Spring
import androidx.compose.animation.core.animate
import androidx.compose.animation.core.calculateTargetValue
import androidx.compose.animation.core.spring
import androidx.compose.animation.rememberSplineBasedDecay
import androidx.compose.foundation.ScrollState
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.gestures.FlingBehavior
import androidx.compose.foundation.gestures.ScrollScope
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
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
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
import androidx.compose.runtime.setValue
import androidx.compose.runtime.withFrameNanos
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.rotate
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.layout.onGloballyPositioned
import androidx.compose.ui.layout.positionInWindow
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import java.util.Calendar
import kotlin.math.ceil
import kotlin.math.roundToInt
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
import website.msdnna.tessera.ui.components.rememberBoardDragState
import website.msdnna.tessera.ui.components.topAccentFrame
import website.msdnna.tessera.ui.theme.PriorityColors
import website.msdnna.tessera.ui.theme.RadiusLg
import website.msdnna.tessera.ui.theme.RadiusMd
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.accentGradient
import website.msdnna.tessera.ui.viewmodels.BoardUiState
import website.msdnna.tessera.ui.viewmodels.BoardViewModel
import website.msdnna.tessera.util.Ion
import website.msdnna.tessera.util.isoDateKey
import website.msdnna.tessera.util.parseHexColor
import website.msdnna.tessera.util.shortDate

/** A kanban lane: title, swatch, count, cards, and (status lanes) a + button. */
private class Lane(val id: String, val title: String, val color: String?, val tasks: List<Task>, val canAdd: Boolean)

@Composable
fun KanbanView(
    state: BoardUiState,
    vm: BoardViewModel,
    onOpenTask: (Task) -> Unit,
) {
    val lanes = if (state.groupByTag) tagLanes(state) else columnLanes(state)
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
                            .dragDim(lane.id == draggingColId)
                            .verticalScroll(rememberScrollState()),
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
                            Column(
                                Modifier.fillMaxWidth().padding(start = 8.dp, end = 8.dp, bottom = 10.dp),
                                verticalArrangement = Arrangement.spacedBy(8.dp),
                            ) {
                                lane.tasks.forEach { task ->
                                    // A faded copy of the dragged card marks the landing
                                    // slot (the dragged card itself collapses + floats).
                                    if (colDrop?.columnId == lane.id && colDrop.afterId == task.id) {
                                        DraggedPreview(drag.dragging, state, vm)
                                    }
                                    // NB: never remove the dragged card from composition
                                    // — its long-press gesture lives on its own node, so
                                    // disposing it cancels the drag. It collapses instead.
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
                                if (lane.canAdd) {
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

    val tasks = state.applyFilterSort(state.tasks)
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

private fun columnLanes(state: BoardUiState): List<Lane> =
    state.sortedColumns.map { col -> Lane(col.id, col.name, col.color, state.visibleTasksIn(col.id), canAdd = true) }

private fun tagLanes(state: BoardUiState): List<Lane> {
    val tags: List<Tag> = state.tags.values.sortedBy { it.name }
    val byTag = tags.map { tag ->
        Lane(tag.id, tag.name, tag.color, state.applyFilterSort(state.tasks.filter { tag.id in it.tagIds }), canAdd = false)
    }
    val untagged = state.applyFilterSort(state.tasks.filter { it.tagIds.isEmpty() })
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

@Composable
fun BoardEmpty(message: String) {
    val c = Tessera.colors
    Box(Modifier.fillMaxSize().background(c.bg), contentAlignment = Alignment.Center) {
        Text(message, color = c.text3, fontSize = 14.sp)
    }
}
