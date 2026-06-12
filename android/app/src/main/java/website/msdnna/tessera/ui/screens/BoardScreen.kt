package website.msdnna.tessera.ui.screens

import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
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
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.material3.pulltorefresh.rememberPullToRefreshState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import website.msdnna.tessera.data.model.Board
import website.msdnna.tessera.ui.components.IonIcon
import website.msdnna.tessera.ui.components.IonIconButton
import website.msdnna.tessera.ui.components.TDropdown
import website.msdnna.tessera.ui.components.TMenuItem
import website.msdnna.tessera.ui.components.TTextField
import website.msdnna.tessera.ui.components.TesseraLoader
import website.msdnna.tessera.ui.components.clickableNoRipple
import website.msdnna.tessera.ui.components.softShadow
import website.msdnna.tessera.ui.theme.RadiusSm
import website.msdnna.tessera.ui.theme.Tessera
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
    archiveOpen: Boolean = false,
    tagsOpen: Boolean = false,
    onCloseArchive: () -> Unit = {},
    onCloseTags: () -> Unit = {},
) {
    val vm: BoardViewModel = viewModel(key = "board-${board.id}")
    val state by vm.state.collectAsStateWithLifecycle()
    val wsVm: WorkspaceViewModel = viewModel()
    val wsState by wsVm.state.collectAsStateWithLifecycle()

    LaunchedEffect(board.id, workspaceId) { vm.load(board.id, workspaceId) }

    var openTaskId by remember(board.id) { mutableStateOf<String?>(null) }
    val ptrState = rememberPullToRefreshState()

    LaunchedEffect(initialTaskId) {
        if (initialTaskId != null) {
            openTaskId = initialTaskId
            onInitialTaskConsumed()
        }
    }
    LaunchedEffect(archiveOpen) { if (archiveOpen) vm.loadArchived() }

    Column(Modifier.fillMaxSize()) {
        BoardToolbar(state = state, vm = vm)
        HorizontalDivider(color = Tessera.colors.border)

        PullToRefreshBox(
            isRefreshing = state.refreshing,
            onRefresh = vm::pullRefresh,
            modifier = Modifier.fillMaxSize(),
            state = ptrState,
            indicator = { BoardRefreshIndicator(distanceFraction = { ptrState.distanceFraction }, refreshing = state.refreshing) },
        ) {
            when {
                state.loading -> Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                    TesseraLoader()
                }

                state.error != null -> BoardEmpty(state.error ?: "Ошибка")

                state.columns.isEmpty() -> BoardEmpty("На этой доске пока нет колонок")

                else -> when (state.viewMode) {
                    BoardViewMode.Kanban -> KanbanView(state = state, vm = vm, onOpenTask = { openTaskId = it.id })
                    BoardViewMode.List -> BoardListView(state = state, vm = vm, onOpenTask = { openTaskId = it.id })
                    BoardViewMode.Calendar -> BoardCalendarView(state = state, vm = vm, onOpenTask = { openTaskId = it.id })
                }
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
            tags = state.tagList,
            members = state.members,
            parentCandidates = state.tasks.filter { it.id != id && it.parentId == null },
            breadcrumb = breadcrumb,
            onClose = { changed ->
                openTaskId = null
                if (changed) vm.reload()
            },
        )
    }

    if (archiveOpen) ArchiveModal(state = state, vm = vm, onDismiss = onCloseArchive)
    if (tagsOpen) TagManagerModal(state = state, vm = vm, onDismiss = onCloseTags)
}

/**
 * Board toolbar (web `KanbanBoard` parity): the composer bar — grouping / sort /
 * filter chips + an add menu + the title search — fills the row, with a subtasks
 * toggle and a saved-views popover pinned to the right. View-mode switching lives
 * in the app bar. Active right-side controls show the accent gradient on the glyph.
 */
@Composable
private fun BoardToolbar(state: BoardUiState, vm: BoardViewModel) {
    val c = Tessera.colors
    var viewsMenu by remember { mutableStateOf(false) }
    Row(
        Modifier.fillMaxWidth().background(c.surface).padding(horizontal = 10.dp, vertical = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        BoardComposerBar(state = state, vm = vm, modifier = Modifier.weight(1f))
        // Expand subtasks — web GitBranchOutline.
        ToolIcon(Ion.GIT_BRANCH, active = state.subtasksExpanded) { vm.toggleSubtasksExpanded() }
        // Saved server-side views — popover (web folder button).
        Box {
            ToolIcon(Ion.FOLDER, active = state.currentViewName != null) { viewsMenu = true }
            TDropdown(expanded = viewsMenu, onDismiss = { viewsMenu = false }) {
                SavedViewsPopover(state = state, vm = vm, onClose = { viewsMenu = false })
            }
        }
    }
}

/** A 36dp icon toolbar button; the glyph carries the accent gradient when
 *  [active], with no background highlight. */
@Composable
private fun ToolIcon(icon: String, active: Boolean, onClick: () -> Unit) {
    val c = Tessera.colors
    Box(
        Modifier.size(36.dp).clip(RoundedCornerShape(RadiusSm)).clickableNoRipple(onClick = onClick),
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
