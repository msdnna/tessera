package website.msdnna.tessera.ui.screens

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.Crossfade
import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.animation.expandHorizontally
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.shrinkHorizontally
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Text
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.material3.pulltorefresh.rememberPullToRefreshState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import website.msdnna.tessera.data.api.RetrofitClient
import website.msdnna.tessera.data.model.Board
import website.msdnna.tessera.ui.components.ErrorState
import website.msdnna.tessera.ui.components.IonIcon
import website.msdnna.tessera.ui.components.IonIconButton
import website.msdnna.tessera.ui.components.LoadingState
import website.msdnna.tessera.ui.components.TDropdown
import website.msdnna.tessera.ui.components.TMenuItem
import website.msdnna.tessera.ui.components.TTextField
import website.msdnna.tessera.ui.components.clickableNoRipple
import website.msdnna.tessera.ui.components.softShadow
import website.msdnna.tessera.ui.theme.RadiusLg
import website.msdnna.tessera.ui.theme.RadiusSm
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.accentGradient
import website.msdnna.tessera.ui.viewmodels.BoardActivity
import website.msdnna.tessera.ui.viewmodels.BoardUiState
import website.msdnna.tessera.ui.viewmodels.BoardViewMode
import website.msdnna.tessera.ui.viewmodels.BoardViewModel
import website.msdnna.tessera.ui.viewmodels.WorkspaceViewModel
import website.msdnna.tessera.util.Ion

/**
 * Board detail: a compact icon toolbar (view / group / sort / filter / subtasks
 * + a title-search field) over the selected board's cards, with pull-to-refresh.
 * Archive + tag management live in the app-bar overflow (hoisted to MainScreen).
 */
@Composable
fun BoardScreen(
    board: Board,
    workspaceId: String,
    initialTaskId: String? = null,
    onInitialTaskConsumed: () -> Unit = {},
    initialMilestoneId: String? = null,
    onInitialMilestoneConsumed: () -> Unit = {},
    archiveOpen: Boolean = false,
    tagsOpen: Boolean = false,
    onCloseArchive: () -> Unit = {},
    onCloseTags: () -> Unit = {},
    onTimelineLikeChanged: (Boolean) -> Unit = {},
) {
    val vm: BoardViewModel = viewModel(key = "board-${board.id}")
    val state by vm.state.collectAsStateWithLifecycle()
    val wsVm: WorkspaceViewModel = viewModel()
    val wsState by wsVm.state.collectAsStateWithLifecycle()
    // Shared (activity-scoped) conflicts VM — loaded by MainScreen; drives the card
    // «Конфликт» pill and opens the resolver focused on the tapped task.
    val conflictsVm: website.msdnna.tessera.ui.viewmodels.ConflictsViewModel = viewModel()
    val conflictsState by conflictsVm.state.collectAsStateWithLifecycle()
    // Live board-activity toasts (separate from the bell), fed by realtime events.
    val activity by vm.activity.collectAsStateWithLifecycle()

    LaunchedEffect(board.id, workspaceId) { vm.load(board.id, workspaceId) }

    // Timeline/Gantt own pinch-zoom + horizontal pan; tell the host to suppress the
    // pull-to-refresh and drawer-edge gestures so they don't steal the pinch.
    val timelineLike = state.viewMode == BoardViewMode.Timeline || state.viewMode == BoardViewMode.Gantt
    LaunchedEffect(timelineLike) { onTimelineLikeChanged(timelineLike) }
    DisposableEffect(Unit) { onDispose { onTimelineLikeChanged(false) } }

    var openTaskId by remember(board.id) { mutableStateOf<String?>(null) }
    val ptrState = rememberPullToRefreshState()

    LaunchedEffect(initialTaskId) {
        if (initialTaskId != null) {
            openTaskId = initialTaskId
            onInitialTaskConsumed()
        }
    }
    // Deep-link «Этап» filter from the «Этапы» screen — apply AFTER the board loads
    // (load() applies the saved view config, which would otherwise clobber it).
    var milestoneApplied by remember(board.id) { mutableStateOf(false) }
    LaunchedEffect(initialMilestoneId, state.loading) {
        if (initialMilestoneId != null && !state.loading && !milestoneApplied) {
            vm.setMilestoneFilter(initialMilestoneId)
            milestoneApplied = true
            onInitialMilestoneConsumed()
        }
    }
    LaunchedEffect(archiveOpen) { if (archiveOpen) vm.loadArchived() }

    // Composer collapsed by default so the tools stay aligned; expanding slides
    // the tools off and a tap anywhere outside the bar (the board area) collapses
    // it again — the same defocus-on-outside-tap as the tag editor.
    var composerExpanded by remember(board.id) { mutableStateOf(false) }
    // Local copy so an icon/colour edit reflects live in the customize panel; the
    // sidebar tree is refreshed separately via wsVm.updateBoard → loadBoards.
    var currentBoard by remember(board.id) { mutableStateOf(board) }
    Column(Modifier.fillMaxSize()) {
        BoardToolbar(
            state = state,
            vm = vm,
            board = currentBoard,
            onUpdateBoard = { icon, color, mode ->
                wsVm.updateBoard(currentBoard.projectId, currentBoard.id, currentBoard.name, icon, color, mode) {
                    currentBoard = it
                }
            },
            expanded = composerExpanded,
            setExpanded = { composerExpanded = it },
        )
        HorizontalDivider(color = Tessera.colors.border)

        if (state.milestones.isNotEmpty()) {
            SprintScopeBar(state = state, onSelect = vm::setMilestoneScope)
            HorizontalDivider(color = Tessera.colors.border)
        }

        Box(Modifier.fillMaxSize()) {
            val boardContent: @Composable () -> Unit = {
                when {
                    state.loading -> LoadingState()

                    state.error != null -> ErrorState(
                        message = state.error ?: "Ошибка",
                        onRetry = { vm.load(board.id, workspaceId) },
                    )

                    state.columns.isEmpty() -> BoardEmpty("На этой доске пока нет колонок")

                    else -> Crossfade(targetState = state.viewMode, animationSpec = tween(200), label = "viewMode") { mode ->
                        when (mode) {
                            BoardViewMode.Kanban -> KanbanView(
                                state = state,
                                vm = vm,
                                onOpenTask = { openTaskId = it.id },
                                conflictTaskIds = conflictsState.taskIds,
                                onOpenConflict = { conflictsVm.openResolver(it.id) },
                            )

                            BoardViewMode.List -> BoardListView(state = state, vm = vm, onOpenTask = { openTaskId = it.id })

                            BoardViewMode.Calendar -> BoardCalendarView(state = state, vm = vm, onOpenTask = { openTaskId = it.id })

                            BoardViewMode.Matrix -> BoardMatrixView(state = state, vm = vm, onOpenTask = { openTaskId = it.id })

                            BoardViewMode.Timeline -> BoardTimelineView(state = state, vm = vm, onOpenTask = { openTaskId = it.id })

                            BoardViewMode.Gantt -> BoardGanttView(state = state, vm = vm, onOpenTask = { openTaskId = it.id })
                        }
                    }
                }
            }
            // Pull-to-refresh everywhere except timeline/Gantt (its vertical drags
            // would fight the pinch-zoom / horizontal pan).
            if (timelineLike) {
                Box(Modifier.fillMaxSize()) { boardContent() }
            } else {
                PullToRefreshBox(
                    isRefreshing = state.refreshing,
                    onRefresh = vm::pullRefresh,
                    modifier = Modifier.fillMaxSize(),
                    state = ptrState,
                    indicator = { BoardRefreshIndicator(distanceFraction = { ptrState.distanceFraction }, refreshing = state.refreshing) },
                ) { boardContent() }
            }
            if (composerExpanded) {
                Box(Modifier.fillMaxSize().clickableNoRipple { composerExpanded = false })
            }
        }
    }

    openTaskId?.let { id ->
        val project = wsState.projects.find { it.id == board.projectId }
        val group = project?.groupId?.let { gid -> wsState.groups.find { it.id == gid } }
        val breadcrumb = listOfNotNull(
            wsState.current?.name,
            group?.name,
            project?.name,
            board.name,
        ).filter { it.isNotBlank() }
        TaskModal(
            initialTaskId = id,
            workspaceId = workspaceId,
            projectId = board.projectId,
            tags = state.tagList,
            prefixNames = state.prefixNames,
            members = state.members,
            gitlabMembers = state.gitlabMembers,
            milestones = state.milestones,
            parentCandidates = state.tasks.filter { it.id != id && it.parentId == null },
            breadcrumb = breadcrumb,
            estimation = state.estimation,
            onClose = { changed ->
                openTaskId = null
                if (changed) vm.reload()
            },
        )
    }

    if (archiveOpen) ArchiveModal(state = state, vm = vm, onDismiss = onCloseArchive)
    if (tagsOpen) TagManagerModal(state = state, vm = vm, onDismiss = onCloseTags)

    BoardActivityOverlay(
        items = activity,
        boardId = board.id,
        onOpen = { openTaskId = it },
        onDismiss = vm::dismissActivity,
    )
}

/**
 * A transient bottom-left stack of board-activity toasts (web BoardActivityToasts
 * parity): who created/moved/completed a task on this board, with Open + Copy-link
 * quick actions. Fed from realtime events; not the bell notification centre.
 */
@Composable
private fun BoardActivityOverlay(
    items: List<BoardActivity>,
    boardId: String,
    onOpen: (String) -> Unit,
    onDismiss: (Long) -> Unit,
) {
    if (items.isEmpty()) return
    val clipboard = LocalClipboardManager.current
    Box(Modifier.fillMaxSize().padding(12.dp), contentAlignment = Alignment.BottomStart) {
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            items.forEach { a ->
                ActivityToast(
                    activity = a,
                    onOpen = { onOpen(a.taskId) },
                    onCopy = {
                        val num = a.number?.toString() ?: a.taskId
                        clipboard.setText(AnnotatedString("${RetrofitClient.serverRoot}/board/$boardId?task=$num"))
                    },
                    onClose = { onDismiss(a.key) },
                )
            }
        }
    }
}

/** Verb → (label, icon, accent colour) for an activity toast. */
private fun activityVerbMeta(verb: String): Triple<String, String, Color> = when (verb) {
    "created" -> Triple("создал(а) задачу", Ion.ADD, Color(0xFF7C5CFF))
    "completed" -> Triple("завершил(а) задачу", Ion.CHECK_CIRCLE, Color(0xFF18A058))
    "reopened" -> Triple("вернул(а) в работу", Ion.ELLIPSE, Color(0xFFE0922F))
    else -> Triple("переместил(а) задачу", Ion.CHEVRON_FORWARD, Color(0xFF2F80ED))
}

/**
 * Sprint scope bar (web parity): server-side navigation between sprints on boards
 * that have milestones. Picks «Все задачи» / «Бэклог» / a milestone; the selection
 * re-scopes the board on the server (for large GitLab imports). A removable chip
 * marks an active sprint scope.
 */
@Composable
private fun SprintScopeBar(
    state: website.msdnna.tessera.ui.viewmodels.BoardUiState,
    onSelect: (String?) -> Unit,
) {
    val c = Tessera.colors
    var open by remember { mutableStateOf(false) }
    val scope = state.milestoneScope
    val label = when (scope) {
        null -> "Все задачи"
        "backlog" -> "Бэклог"
        else -> state.milestonesMap[scope]?.title ?: "Спринт"
    }
    val active = scope != null
    Row(
        Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Box {
            Row(
                Modifier.clip(RoundedCornerShape(RadiusSm))
                    .background(if (active) accentGradient(c.primary) else SolidColor(c.surface))
                    .border(1.dp, if (active) Color.Transparent else c.border, RoundedCornerShape(RadiusSm))
                    .clickableNoRipple { open = true }
                    .padding(horizontal = 10.dp, vertical = 6.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                IonIcon(Ion.GIT_MERGE, size = 14.dp, tint = if (active) c.onPrimary else c.text3)
                Spacer(Modifier.width(6.dp))
                Text("Спринт: $label", color = if (active) c.onPrimary else c.text2, fontSize = 13.sp, fontWeight = FontWeight.Medium)
                Spacer(Modifier.width(4.dp))
                IonIcon(Ion.CHEVRON_DOWN, size = 12.dp, tint = if (active) c.onPrimary else c.text3)
            }
            TDropdown(expanded = open, onDismiss = { open = false }) {
                TMenuItem("Все задачи", onClick = {
                    open = false
                    onSelect(null)
                })
                TMenuItem("Бэклог (без спринта)", onClick = {
                    open = false
                    onSelect("backlog")
                })
                state.milestones.forEach { m ->
                    TMenuItem(m.title, onClick = {
                        open = false
                        onSelect(m.id)
                    })
                }
            }
        }
        if (active) {
            Spacer(Modifier.width(8.dp))
            IonIcon(
                Ion.CLOSE, size = 16.dp, tint = c.text3,
                modifier = Modifier.clip(CircleShape).clickableNoRipple { onSelect(null) },
            )
        }
    }
}

@Composable
private fun ActivityToast(
    activity: BoardActivity,
    onOpen: () -> Unit,
    onCopy: () -> Unit,
    onClose: () -> Unit,
) {
    val c = Tessera.colors
    val (verbText, icon, color) = activityVerbMeta(activity.verb)
    var copied by remember(activity.key) { mutableStateOf(false) }
    val shape = RoundedCornerShape(RadiusLg)
    Row(
        Modifier.widthIn(max = 320.dp).softShadow(shape, elevation = 6.dp).clip(shape)
            .background(c.surface).border(1.dp, c.border, shape).padding(10.dp),
        verticalAlignment = Alignment.Top,
    ) {
        Box(
            Modifier.size(30.dp).clip(CircleShape).background(color.copy(alpha = 0.16f)),
            contentAlignment = Alignment.Center,
        ) {
            IonIcon(icon, size = 17.dp, tint = color)
        }
        Spacer(Modifier.width(10.dp))
        Column(Modifier.weight(1f)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(
                    if (activity.self) "Вы" else activity.actorName.ifBlank { "Кто-то" },
                    color = c.text2, fontSize = 12.sp, fontWeight = FontWeight.SemiBold,
                )
                Spacer(Modifier.width(4.dp))
                Text(verbText, color = c.text3, fontSize = 12.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
            }
            Text(
                activity.title,
                color = c.text1, fontSize = 13.sp, fontWeight = FontWeight.Medium,
                maxLines = 2, overflow = TextOverflow.Ellipsis,
                modifier = Modifier.padding(top = 2.dp, bottom = 6.dp),
            )
            Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                Box(
                    Modifier.clip(RoundedCornerShape(RadiusSm)).background(accentGradient(c.primary))
                        .clickableNoRipple(onClick = onOpen).padding(horizontal = 10.dp, vertical = 4.dp),
                ) {
                    Text("Открыть", color = c.onPrimary, fontSize = 12.sp)
                }
                Box(
                    Modifier.clip(RoundedCornerShape(RadiusSm)).border(1.dp, c.border, RoundedCornerShape(RadiusSm))
                        .clickableNoRipple {
                            onCopy()
                            copied = true
                        }
                        .padding(horizontal = 10.dp, vertical = 4.dp),
                ) {
                    Text(if (copied) "Скопировано" else "Ссылка", color = c.text2, fontSize = 12.sp)
                }
            }
        }
        Spacer(Modifier.width(4.dp))
        IonIcon(
            Ion.CLOSE, size = 15.dp, tint = c.text3,
            modifier = Modifier.clip(CircleShape).clickableNoRipple(onClick = onClose),
        )
    }
}

/**
 * Board toolbar (web `KanbanBoard` parity): the composer bar — grouping / sort /
 * filter chips + an add menu + the title search — fills the row, with a subtasks
 * toggle and a saved-views popover pinned to the right. View-mode switching lives
 * in the app bar. Active right-side controls show the accent gradient on the glyph.
 */
@Composable
private fun BoardToolbar(
    state: BoardUiState,
    vm: BoardViewModel,
    board: Board,
    onUpdateBoard: (icon: String, color: String, iconMode: String) -> Unit,
    expanded: Boolean,
    setExpanded: (Boolean) -> Unit,
) {
    val c = Tessera.colors
    var viewsMenu by remember { mutableStateOf(false) }
    var customizeOpen by remember { mutableStateOf(false) }
    if (customizeOpen) {
        BoardCustomizePanel(state, vm, board, onUpdateBoard) { customizeOpen = false }
    }
    Row(
        Modifier.fillMaxWidth().background(c.surface).padding(horizontal = 10.dp, vertical = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        BoardComposerBar(
            state = state,
            vm = vm,
            expanded = expanded,
            setExpanded = setExpanded,
            modifier = Modifier.weight(1f),
        )
        AnimatedVisibility(
            visible = !expanded,
            enter = expandHorizontally() + fadeIn(),
            exit = shrinkHorizontally() + fadeOut(),
        ) {
            Row(
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                // "Авто": dependency-graph row ordering — Gantt only (web GitNetworkOutline).
                if (state.viewMode == BoardViewMode.Gantt) {
                    ToolIcon(Ion.GIT_NETWORK, active = state.autoActive) { vm.toggleAutoSort() }
                }
                // Expand subtasks — web GitBranchOutline. Hidden on the time-axis views
                // (timeline/Gantt show one row per task; no subtask expansion there).
                val timelineLike = state.viewMode == BoardViewMode.Timeline || state.viewMode == BoardViewMode.Gantt
                if (!timelineLike) {
                    ToolIcon(Ion.GIT_BRANCH, active = state.subtasksExpanded) { vm.toggleSubtasksExpanded() }
                }
                // Saved server-side views — popover (web folder button).
                Box {
                    ToolIcon(Ion.FOLDER, active = state.currentViewName != null) { viewsMenu = true }
                    TDropdown(expanded = viewsMenu, onDismiss = { viewsMenu = false }) {
                        SavedViewsPopover(state = state, vm = vm, onClose = { viewsMenu = false })
                    }
                }
                // Board appearance: card density / fields / columns (web gear panel).
                ToolIcon(Ion.SETTINGS, active = customizeOpen) { customizeOpen = true }
            }
        }
    }
}

/** A 36dp icon toolbar button on a flat neutral fill (web quaternary parity); the
 *  background stays grey, only the glyph picks up the accent gradient when [active]. */
@Composable
private fun ToolIcon(icon: String, active: Boolean, onClick: () -> Unit) {
    val c = Tessera.colors
    Box(
        Modifier.size(36.dp).clip(RoundedCornerShape(RadiusSm))
            .background(c.surfaceAlt)
            .clickableNoRipple(onClick = onClick),
        contentAlignment = Alignment.Center,
    ) {
        IonIcon(icon, size = 19.dp, tint = if (active) c.primary else c.text2, gradient = active)
    }
}

/**
 * Pull-to-refresh indicator in the app's style: a floating circle with the
 * accent refresh glyph — rotating with the pull, spinning while refreshing.
 */
@Composable
private fun androidx.compose.foundation.layout.BoxScope.BoardRefreshIndicator(
    distanceFraction: () -> Float,
    refreshing: Boolean,
) {
    val c = Tessera.colors
    val spin = rememberInfiniteTransition(label = "ptr-spin")
    val angle by spin.animateFloat(
        initialValue = 0f,
        targetValue = 360f,
        animationSpec = infiniteRepeatable(tween(900, easing = LinearEasing)),
        label = "ptr-angle",
    )
    val frac = distanceFraction().coerceIn(0f, 1.5f)
    if (!refreshing && frac <= 0f) return
    val density = LocalDensity.current
    Box(
        Modifier.align(Alignment.TopCenter)
            .graphicsLayer {
                translationY = with(density) { (if (refreshing) 14.dp else 14.dp * frac.coerceAtMost(1f)).toPx() }
                alpha = if (refreshing) 1f else frac.coerceIn(0f, 1f)
            }
            .size(34.dp)
            .softShadow(CircleShape, elevation = 4.dp)
            .clip(CircleShape)
            .background(c.surface),
        contentAlignment = Alignment.Center,
    ) {
        IonIcon(
            Ion.REFRESH,
            size = 18.dp,
            tint = c.primary,
            gradient = true,
            modifier = Modifier.graphicsLayer { rotationZ = if (refreshing) angle else frac * 260f },
        )
    }
}
