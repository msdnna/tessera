package website.msdnna.tessera.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawing
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.DrawerValue
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.ModalDrawerSheet
import androidx.compose.material3.ModalNavigationDrawer
import androidx.compose.material3.Text
import androidx.compose.material3.rememberDrawerState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LifecycleEventEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.model.Board
import website.msdnna.tessera.data.model.User
import website.msdnna.tessera.data.repository.BoardRepository
import website.msdnna.tessera.ui.components.IonIconButton
import website.msdnna.tessera.ui.components.Sidebar
import website.msdnna.tessera.ui.components.TDropdown
import website.msdnna.tessera.ui.components.TMenuItem
import website.msdnna.tessera.ui.components.UpdateDialog
import website.msdnna.tessera.ui.components.clickableNoRipple
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.accentGradient
import website.msdnna.tessera.ui.viewmodels.NotificationViewModel
import website.msdnna.tessera.ui.viewmodels.UpdateViewModel
import website.msdnna.tessera.ui.viewmodels.WorkspaceViewModel
import website.msdnna.tessera.util.Ion

/** Top-level destinations. Boards carry their model; the rest are singletons. */
sealed interface MainDest {
    data object Home : MainDest
    data object Notes : MainDest
    data object Reminders : MainDest
    data class BoardView(val board: Board) : MainDest
}

/**
 * App shell: a drawer-hosted sidebar + topbar. Hosts the top-level destinations
 * (Home / Notes / Reminders / a board), the bell + notifications feed, and the
 * search overlay. Acts as the navigation hub — opening a task from any of these
 * fetches its board and switches to it.
 */
@Composable
fun MainScreen(
    user: User?,
    isDark: Boolean,
    accentKey: String,
    openTaskId: String?,
    onOpenTaskHandled: () -> Unit,
    onAccentChange: (String) -> Unit,
    onToggleDark: () -> Unit,
    onLogout: () -> Unit,
    wsVm: WorkspaceViewModel = viewModel(),
    notifVm: NotificationViewModel = viewModel(),
    updateVm: UpdateViewModel = viewModel(),
) {
    val c = Tessera.colors
    val drawerState = rememberDrawerState(DrawerValue.Closed)
    val scope = rememberCoroutineScope()
    val state by wsVm.state.collectAsStateWithLifecycle()
    val notifState by notifVm.state.collectAsStateWithLifecycle()
    val updateState by updateVm.state.collectAsStateWithLifecycle()
    val updateAvailable by updateVm.available.collectAsStateWithLifecycle()
    val boardRepo = remember { BoardRepository() }

    var dest by remember { mutableStateOf<MainDest>(MainDest.Home) }
    var pendingTaskId by remember { mutableStateOf<String?>(null) }
    var notesPreselectId by remember { mutableStateOf<String?>(null) }
    var searchOpen by remember { mutableStateOf(false) }
    var bellOpen by remember { mutableStateOf(false) }
    var membersOpen by remember { mutableStateOf(false) }
    var boardArchiveOpen by remember { mutableStateOf(false) }
    var boardTagsOpen by remember { mutableStateOf(false) }

    // Navigate to a task by board+task: fetch the board, switch to it, queue the open.
    fun openTask(boardId: String, taskId: String) {
        bellOpen = false
        searchOpen = false
        scope.launch {
            val board = runCatching { boardRepo.board(boardId) }.getOrNull() ?: return@launch
            dest = MainDest.BoardView(board)
            pendingTaskId = taskId
        }
    }

    // Reminder deep-link gives only a task id → resolve its board first.
    LaunchedEffect(openTaskId) {
        val id = openTaskId ?: return@LaunchedEffect
        onOpenTaskHandled()
        val boardId = runCatching { boardRepo.taskBoardId(id) }.getOrNull()
        if (boardId != null) openTask(boardId, id)
    }

    // Re-check for updates whenever the app returns to the foreground — the
    // initial check runs in UpdateViewModel.init, but a release published while
    // the app was merely backgrounded would otherwise be missed until a cold
    // start. check() no-ops if a prompt is already shown or was dismissed.
    LifecycleEventEffect(Lifecycle.Event.ON_RESUME) { updateVm.check() }

    // Restore the last-open destination once on launch (a reminder deep-link wins).
    var restoreDone by remember { mutableStateOf(false) }
    LaunchedEffect(Unit) {
        if (openTaskId == null) {
            val saved = AppContainer.prefs.lastDest.first()
            when {
                saved == "notes" -> dest = MainDest.Notes

                saved == "reminders" -> dest = MainDest.Reminders

                saved.startsWith("board:") -> {
                    val id = saved.removePrefix("board:")
                    runCatching { boardRepo.board(id) }.getOrNull()?.let { dest = MainDest.BoardView(it) }
                }
            }
        }
        restoreDone = true
    }
    // Persist the destination once the initial restore has run (so the restore's
    // own read isn't clobbered by the default Home being written first).
    LaunchedEffect(dest, restoreDone) {
        if (!restoreDone) return@LaunchedEffect
        AppContainer.prefs.setLastDest(
            when (val d = dest) {
                is MainDest.Home -> "home"
                is MainDest.Notes -> "notes"
                is MainDest.Reminders -> "reminders"
                is MainDest.BoardView -> "board:${d.board.id}"
            },
        )
    }

    ModalNavigationDrawer(
        drawerState = drawerState,
        drawerContent = {
            ModalDrawerSheet(drawerContainerColor = c.surface, modifier = Modifier.width(280.dp)) {
                fun navTo(d: MainDest) {
                    dest = d
                    scope.launch { drawerState.close() }
                }
                Sidebar(
                    vm = wsVm,
                    state = state,
                    user = user,
                    isDark = isDark,
                    accentKey = accentKey,
                    activeNav = navKeyOf(dest),
                    onAccentChange = onAccentChange,
                    onToggleDark = onToggleDark,
                    onLogout = onLogout,
                    onOpenHome = { navTo(MainDest.Home) },
                    onOpenReminders = { navTo(MainDest.Reminders) },
                    onOpenNotes = { navTo(MainDest.Notes) },
                    onOpenMembers = {
                        membersOpen = true
                        scope.launch { drawerState.close() }
                    },
                    onOpenBoard = { board -> navTo(MainDest.BoardView(board)) },
                    updateVersion = updateAvailable?.let { "v${it.version}" },
                    onUpdate = {
                        scope.launch { drawerState.close() }
                        updateVm.startDownload()
                    },
                )
            }
        },
    ) {
        Box(Modifier.fillMaxSize()) {
            Column(
                Modifier.fillMaxSize().background(c.bg).windowInsetsPadding(WindowInsets.safeDrawing),
            ) {
                TopBar(
                    title = titleFor(dest),
                    unread = notifState.unread,
                    bellOpen = bellOpen,
                    notifState = notifState,
                    isBoard = dest is MainDest.BoardView,
                    boardId = (dest as? MainDest.BoardView)?.board?.id,
                    onMenu = { scope.launch { drawerState.open() } },
                    onSearch = { searchOpen = true },
                    onBellToggle = { bellOpen = !bellOpen },
                    onBellDismiss = { bellOpen = false },
                    onNotification = { n ->
                        notifVm.markRead(n)
                        bellOpen = false
                        if (n.opensTask) openTask(n.taskBoardId!!, n.taskId!!)
                    },
                    onMarkAll = { notifVm.markAllRead() },
                    onArchive = { boardArchiveOpen = true },
                    onTags = { boardTagsOpen = true },
                )
                HorizontalDivider(color = c.border)

                Box(Modifier.fillMaxSize()) {
                    when (val d = dest) {
                        is MainDest.Home -> HomeScreen(
                            workspaceId = state.currentId,
                            userName = user?.name.orEmpty(),
                            userId = user?.id.orEmpty(),
                            onOpenTask = ::openTask,
                        )

                        is MainDest.Notes -> NotesScreen(
                            workspaceId = state.currentId,
                            preselectNoteId = notesPreselectId,
                            onPreselectConsumed = { notesPreselectId = null },
                        )

                        is MainDest.Reminders -> RemindersScreen()

                        is MainDest.BoardView -> BoardScreen(
                            board = d.board,
                            workspaceId = state.currentId,
                            initialTaskId = pendingTaskId,
                            onInitialTaskConsumed = { pendingTaskId = null },
                            archiveOpen = boardArchiveOpen,
                            tagsOpen = boardTagsOpen,
                            onCloseArchive = { boardArchiveOpen = false },
                            onCloseTags = { boardTagsOpen = false },
                        )
                    }
                }
            }

            if (membersOpen) {
                MembersModal(workspaceId = state.currentId, onDismiss = { membersOpen = false })
            }

            if (searchOpen) {
                SearchOverlay(
                    workspaceId = state.currentId,
                    onClose = { searchOpen = false },
                    onOpenTask = ::openTask,
                    onOpenNote = { noteId ->
                        notesPreselectId = noteId
                        dest = MainDest.Notes
                        searchOpen = false
                    },
                )
            }

            UpdateDialog(
                state = updateState,
                onUpdate = { updateVm.startDownload() },
                onInstall = { updateVm.install() },
                onDismiss = { updateVm.dismiss() },
            )
        }
    }
}

private val ViewModes = listOf(
    website.msdnna.tessera.ui.viewmodels.BoardViewMode.Kanban,
    website.msdnna.tessera.ui.viewmodels.BoardViewMode.List,
    website.msdnna.tessera.ui.viewmodels.BoardViewMode.Calendar,
)
private val ViewSegments = listOf(
    website.msdnna.tessera.ui.components.SegmentOption("Доска", Ion.GRID),
    website.msdnna.tessera.ui.components.SegmentOption("Список", Ion.LIST),
    website.msdnna.tessera.ui.components.SegmentOption("Календарь", Ion.CALENDAR),
)

/**
 * Board name + layout switcher (web mobile header): tapping the title OR the
 * chevron opens a horizontal Доска / Список / Календарь segmented selector.
 * Resolves the SAME [website.msdnna.tessera.ui.viewmodels.BoardViewModel] the
 * board uses (shared by key), so the switch reflects immediately.
 */
@Composable
private fun BoardTitleSwitcher(boardId: String, title: String, modifier: Modifier = Modifier) {
    val c = Tessera.colors
    val vm: website.msdnna.tessera.ui.viewmodels.BoardViewModel = viewModel(key = "board-$boardId")
    val st by vm.state.collectAsStateWithLifecycle()
    var menu by remember { mutableStateOf(false) }
    Box(modifier) {
        Row(
            Modifier.clickableNoRipple { menu = true },
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(
                title,
                color = c.text1,
                fontSize = 17.sp,
                fontWeight = FontWeight.SemiBold,
                maxLines = 1,
                modifier = Modifier.weight(1f, fill = false),
            )
            Spacer(Modifier.width(4.dp))
            website.msdnna.tessera.ui.components.IonIcon(Ion.CHEVRON_DOWN, size = 18.dp, tint = c.text2)
        }
        TDropdown(expanded = menu, onDismiss = { menu = false }, bare = true) {
            website.msdnna.tessera.ui.components.HSegmentedSelector(
                options = ViewSegments,
                selectedIndex = ViewModes.indexOf(st.viewMode).coerceAtLeast(0),
                onSelect = { i ->
                    menu = false
                    vm.setViewMode(ViewModes[i])
                },
                modifier = Modifier.width(220.dp).padding(top = 4.dp),
            )
        }
    }
}

private fun titleFor(dest: MainDest): String = when (dest) {
    is MainDest.Home -> "Моя работа"
    is MainDest.Notes -> "Заметки"
    is MainDest.Reminders -> "Напоминания"
    is MainDest.BoardView -> dest.board.name
}

/** A stable key for the sidebar's active-row highlight. */
private fun navKeyOf(dest: MainDest): String = when (dest) {
    is MainDest.Home -> "home"
    is MainDest.Notes -> "notes"
    is MainDest.Reminders -> "reminders"
    is MainDest.BoardView -> "board"
}

@Composable
private fun TopBar(
    title: String,
    unread: Int,
    bellOpen: Boolean,
    notifState: website.msdnna.tessera.ui.viewmodels.NotificationUiState,
    isBoard: Boolean,
    boardId: String?,
    onMenu: () -> Unit,
    onSearch: () -> Unit,
    onBellToggle: () -> Unit,
    onBellDismiss: () -> Unit,
    onNotification: (website.msdnna.tessera.data.model.Notification) -> Unit,
    onMarkAll: () -> Unit,
    onArchive: () -> Unit,
    onTags: () -> Unit,
) {
    val c = Tessera.colors
    var boardMenu by remember { mutableStateOf(false) }
    Row(
        Modifier.fillMaxWidth().background(c.surface).padding(horizontal = 8.dp, vertical = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        IonIconButton(Ion.MENU, onClick = onMenu, boxSize = 40.dp)
        Spacer(Modifier.width(4.dp))
        if (boardId != null) {
            BoardTitleSwitcher(boardId, title, Modifier.weight(1f))
        } else {
            Text(
                title,
                color = c.text1,
                fontSize = 17.sp,
                fontWeight = FontWeight.SemiBold,
                maxLines = 1,
                modifier = Modifier.weight(1f),
            )
        }
        IonIconButton(Ion.SEARCH, onClick = onSearch, boxSize = 40.dp)
        Box {
            Box {
                IonIconButton(Ion.NOTIFICATIONS, onClick = onBellToggle, boxSize = 40.dp)
                if (unread > 0) {
                    Box(
                        Modifier.align(Alignment.TopEnd).padding(top = 4.dp, end = 4.dp)
                            .size(16.dp).clip(CircleShape).background(accentGradient(c.primary)),
                        contentAlignment = Alignment.Center,
                    ) {
                        Text(
                            if (unread > 9) "9+" else unread.toString(),
                            color = c.onPrimary,
                            fontSize = 9.sp,
                            fontWeight = FontWeight.Bold,
                        )
                    }
                }
            }
            NotificationsPanel(
                expanded = bellOpen,
                state = notifState,
                onItemClick = onNotification,
                onMarkAll = onMarkAll,
                onDismiss = onBellDismiss,
            )
        }
        if (isBoard) {
            Box {
                IonIconButton(Ion.ELLIPSIS_V, onClick = { boardMenu = true }, boxSize = 40.dp)
                TDropdown(expanded = boardMenu, onDismiss = { boardMenu = false }) {
                    TMenuItem("Архив доски", icon = Ion.ARCHIVE, onClick = {
                        boardMenu = false
                        onArchive()
                    })
                    TMenuItem("Управление тегами", icon = Ion.PRICETAGS, onClick = {
                        boardMenu = false
                        onTags()
                    })
                }
            }
        }
    }
}
