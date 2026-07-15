package website.msdnna.tessera.ui.screens

import android.app.Activity
import android.widget.Toast
import androidx.activity.compose.BackHandler
import androidx.compose.animation.Crossfade
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
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
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.LocalContext
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
import website.msdnna.tessera.data.realtime.DeviceNotifier
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
    data object Milestones : MainDest
    data object GitLabSettings : MainDest
    data object GitLabJournal : MainDest
    data object Notifications : MainDest
    data object Settings : MainDest
    data object Admin : MainDest
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
    conflictsVm: website.msdnna.tessera.ui.viewmodels.ConflictsViewModel = viewModel(),
) {
    val c = Tessera.colors
    val drawerState = rememberDrawerState(DrawerValue.Closed)
    // Timeline/Gantt own pinch-zoom + horizontal pan → suppress the drawer edge-swipe
    // there so it doesn't steal the gesture (set by BoardScreen).
    var boardTimelineLike by remember { mutableStateOf(false) }
    val scope = rememberCoroutineScope()
    val state by wsVm.state.collectAsStateWithLifecycle()
    val notifState by notifVm.state.collectAsStateWithLifecycle()
    val conflictsState by conflictsVm.state.collectAsStateWithLifecycle()
    val updateState by updateVm.state.collectAsStateWithLifecycle()
    val updateAvailable by updateVm.available.collectAsStateWithLifecycle()
    val boardRepo = remember { BoardRepository() }
    val gitlabRepo = remember { website.msdnna.tessera.data.repository.GitlabRepository() }

    var dest by remember { mutableStateOf<MainDest>(MainDest.Home) }
    var pendingTaskId by remember { mutableStateOf<String?>(null) }
    // Deep-link from the «Этапы» screen: open a board filtered to this milestone.
    var pendingMilestoneId by remember { mutableStateOf<String?>(null) }
    // The GitLab-integration project (if any), so the milestone manager only offers
    // «В GitLab» on milestones of that project (resolved from the integration board).
    var glProjectId by remember { mutableStateOf<String?>(null) }
    var notesPreselectId by remember { mutableStateOf<String?>(null) }
    var searchOpen by remember { mutableStateOf(false) }
    var bellOpen by remember { mutableStateOf(false) }
    var membersOpen by remember { mutableStateOf(false) }
    var boardArchiveOpen by remember { mutableStateOf(false) }
    var boardTagsOpen by remember { mutableStateOf(false) }

    // Navigation back-stack of visited destinations (the current `dest` is the top,
    // held separately). The system Back gesture pops this instead of closing the
    // app; at the root it asks for a confirming second press. The initial-restore
    // below sets `dest` directly so it doesn't seed a phantom Home entry.
    val backStack = remember { mutableStateListOf<MainDest>() }
    fun navTo(d: MainDest) {
        if (d == dest) return
        // Going back to the immediately-previous screen collapses the stack rather
        // than growing it (A→B→A behaves like a Back), keeping history tidy.
        if (backStack.isNotEmpty() && backStack.last() == d) backStack.removeAt(backStack.lastIndex)
        else backStack.add(dest)
        dest = d
    }

    // Navigate to a task by board+task: fetch the board, switch to it, queue the open.
    fun openTask(boardId: String, taskId: String) {
        bellOpen = false
        searchOpen = false
        scope.launch {
            val board = runCatching { boardRepo.board(boardId) }.getOrNull() ?: return@launch
            navTo(MainDest.BoardView(board))
            pendingTaskId = taskId
        }
    }

    // Deep-link «Этап» → open the project's (first) board, filtered to the milestone.
    fun openBoardWithMilestone(projectId: String, milestoneId: String) {
        scope.launch {
            val board = state.boardsByProject[projectId]?.firstOrNull()
                ?: runCatching { boardRepo.boards(projectId) }.getOrNull()?.firstOrNull()
                ?: return@launch
            navTo(MainDest.BoardView(board))
            pendingMilestoneId = milestoneId
        }
    }

    // Load GitLab write-back conflicts for the current workspace (count badges + resolver).
    LaunchedEffect(state.currentId) { conflictsVm.load(state.currentId) }

    // Resolve the GitLab-integration project once the workspace + its boards are known.
    LaunchedEffect(state.currentId, state.boardsByProject) {
        glProjectId = if (state.currentId.isBlank()) {
            null
        } else {
            // Multiple bindings are possible; resolve the project of the first
            // (prefer an enabled one) so milestone gating has a target.
            val integs = runCatching { gitlabRepo.integrations(state.currentId).integrations }.getOrNull().orEmpty()
            val boardId = (integs.firstOrNull { it.enabled } ?: integs.firstOrNull())?.boardId
            if (boardId == null) {
                null
            } else {
                state.boardsByProject.entries.firstOrNull { e -> e.value.any { it.id == boardId } }?.key
            }
        }
    }

    val backContext = LocalContext.current
    var lastBackAt by remember { mutableStateOf(0L) }
    // Drawer open → Back closes it. Otherwise Back pops the nav stack, and at the
    // root it minimises the app on a confirming second press (with a hint toast),
    // never killing it on the first gesture. Dialog-based modals (task/members/…)
    // and the search/note overlays consume Back via their own handlers first.
    BackHandler(enabled = drawerState.isOpen) { scope.launch { drawerState.close() } }
    BackHandler(enabled = !drawerState.isOpen) {
        if (backStack.isNotEmpty()) {
            dest = backStack.removeAt(backStack.lastIndex)
        } else {
            val now = System.currentTimeMillis()
            if (now - lastBackAt < 2000L) {
                (backContext as? Activity)?.moveTaskToBack(true)
            } else {
                lastBackAt = now
                Toast.makeText(backContext, "Нажмите «Назад» ещё раз для выхода", Toast.LENGTH_SHORT).show()
            }
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

                saved == "milestones" -> dest = MainDest.Milestones

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
                is MainDest.Milestones -> "milestones"
                is MainDest.GitLabSettings -> "gitlab"
                is MainDest.GitLabJournal -> "gitlab"
                is MainDest.Notifications -> "notifications"
                is MainDest.Settings -> "settings"
                is MainDest.Admin -> "admin"
                is MainDest.BoardView -> "board:${d.board.id}"
            },
        )
    }

    // This device's stable id, for the notification-settings «это устройство» badge.
    var deviceId by remember { mutableStateOf("") }
    LaunchedEffect(Unit) { deviceId = runCatching { AppContainer.prefs.ensureDeviceId() }.getOrDefault("") }

    // Raise a system notification when the server routes one to this device (the
    // device-channel path while the app is open). Context lives here, in the UI.
    val pushContext = LocalContext.current
    LaunchedEffect(Unit) {
        notifVm.devicePush.collect { DeviceNotifier.show(pushContext, it) }
    }

    ModalNavigationDrawer(
        drawerState = drawerState,
        gesturesEnabled = !boardTimelineLike || drawerState.isOpen,
        drawerContent = {
            ModalDrawerSheet(drawerContainerColor = c.surface, modifier = Modifier.width(280.dp)) {
                // Sidebar navigation: push onto the back-stack and close the drawer.
                fun go(d: MainDest) {
                    navTo(d)
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
                    onOpenHome = { go(MainDest.Home) },
                    onOpenReminders = { go(MainDest.Reminders) },
                    onOpenNotes = { go(MainDest.Notes) },
                    onOpenMilestones = { go(MainDest.Milestones) },
                    onOpenMembers = {
                        membersOpen = true
                        scope.launch { drawerState.close() }
                    },
                    onOpenGitlab = { go(MainDest.GitLabSettings) },
                    conflictCount = conflictsState.count,
                    onOpenNotifications = { go(MainDest.Notifications) },
                    onOpenSettings = { go(MainDest.Settings) },
                    onOpenAdmin = { go(MainDest.Admin) },
                    onOpenBoard = { board -> go(MainDest.BoardView(board)) },
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
                    isIntegration = dest is MainDest.GitLabSettings,
                    projectBoards = (dest as? MainDest.BoardView)?.board
                        ?.let { state.boardsByProject[it.projectId] }
                        .orEmpty(),
                    onSelectBoard = { board -> navTo(MainDest.BoardView(board)) },
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
                    Crossfade(targetState = dest, animationSpec = tween(220), label = "dest") { d ->
                        when (d) {
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

                            is MainDest.Milestones -> MilestonesScreen(
                                workspaceId = state.currentId,
                                projects = state.projects,
                                workspace = state.current,
                                glProjectId = glProjectId,
                                onOpenMilestone = { projectId, milestoneId ->
                                    openBoardWithMilestone(projectId, milestoneId)
                                },
                            )

                            is MainDest.GitLabSettings -> GitLabSettingsScreen(
                                workspaceId = state.currentId,
                                onOpenJournal = { navTo(MainDest.GitLabJournal) },
                                conflictCount = conflictsState.count,
                                onOpenConflicts = { conflictsVm.openResolver() },
                            )

                            is MainDest.GitLabJournal -> GitLabJournalScreen(workspaceId = state.currentId)

                            is MainDest.Notifications -> NotificationSettingsScreen(deviceId = deviceId)

                            is MainDest.Settings -> ProfileScreen()

                            is MainDest.Admin -> AdminScreen()

                            is MainDest.BoardView -> BoardScreen(
                                board = d.board,
                                workspaceId = state.currentId,
                                initialTaskId = pendingTaskId,
                                onInitialTaskConsumed = { pendingTaskId = null },
                                initialMilestoneId = pendingMilestoneId,
                                onInitialMilestoneConsumed = { pendingMilestoneId = null },
                                archiveOpen = boardArchiveOpen,
                                tagsOpen = boardTagsOpen,
                                onCloseArchive = { boardArchiveOpen = false },
                                onCloseTags = { boardTagsOpen = false },
                                onTimelineLikeChanged = { boardTimelineLike = it },
                            )
                        }
                    }
                }
            }

            if (membersOpen) {
                MembersModal(workspaceId = state.currentId, onDismiss = { membersOpen = false })
            }

            if (conflictsState.resolverOpen) {
                ConflictResolverModal(vm = conflictsVm, onDismiss = { conflictsVm.closeResolver() })
            }

            if (searchOpen) {
                SearchOverlay(
                    workspaceId = state.currentId,
                    onClose = { searchOpen = false },
                    onOpenTask = ::openTask,
                    onOpenNote = { noteId ->
                        notesPreselectId = noteId
                        navTo(MainDest.Notes)
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
    website.msdnna.tessera.ui.viewmodels.BoardViewMode.Timeline,
    website.msdnna.tessera.ui.viewmodels.BoardViewMode.Gantt,
    website.msdnna.tessera.ui.viewmodels.BoardViewMode.Matrix,
)

// View-mode segments use the custom icon pack (web layout-*): outline when idle,
// filled when the segment is active.
private fun viewSeg(label: String, base: String) =
    website.msdnna.tessera.ui.components.SegmentOption(label, "layout_${base}_outline", "layout_${base}_filled")

private val ViewSegments = listOf(
    viewSeg("Доска", "kanban"),
    viewSeg("Список", "list"),
    viewSeg("Календарь", "calendar"),
    viewSeg("Таймлайн", "timeline"),
    viewSeg("Гант", "gantt"),
    viewSeg("Матрица", "matrix"),
)

/**
 * Board name + layout switcher (web mobile header): tapping the title OR the
 * chevron opens a horizontal Доска / Список / Календарь segmented selector,
 * followed by the other boards of the current project for a quick jump. Resolves
 * the SAME [website.msdnna.tessera.ui.viewmodels.BoardViewModel] the board uses
 * (shared by key), so the layout switch reflects immediately.
 */
@Composable
private fun BoardTitleSwitcher(
    boardId: String,
    title: String,
    boards: List<Board>,
    onSelectBoard: (Board) -> Unit,
    modifier: Modifier = Modifier,
) {
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
        TDropdown(expanded = menu, onDismiss = { menu = false }) {
            // Width grows with the number of view segments so labels («Календарь»,
            // «Таймлайн») don't get squeezed into letter-by-letter wraps.
            Column(Modifier.width((ViewSegments.size * 64).dp.coerceAtLeast(240.dp))) {
                website.msdnna.tessera.ui.components.HSegmentedSelector(
                    options = ViewSegments,
                    selectedIndex = ViewModes.indexOf(st.viewMode).coerceAtLeast(0),
                    onSelect = { i ->
                        menu = false
                        vm.setViewMode(ViewModes[i])
                    },
                    modifier = Modifier.fillMaxWidth().padding(horizontal = 8.dp, vertical = 2.dp),
                )
                if (boards.size > 1) {
                    Spacer(Modifier.height(8.dp))
                    HorizontalDivider(color = c.border)
                    Spacer(Modifier.height(6.dp))
                    Text(
                        "Доски проекта".uppercase(),
                        color = c.text3,
                        fontSize = 11.sp,
                        fontWeight = FontWeight.SemiBold,
                        modifier = Modifier.padding(horizontal = 14.dp, vertical = 4.dp),
                    )
                    boards.forEach { b ->
                        val current = b.id == boardId
                        Row(
                            Modifier.fillMaxWidth().clickableNoRipple {
                                menu = false
                                if (!current) onSelectBoard(b)
                            }.padding(horizontal = 14.dp, vertical = 10.dp),
                            verticalAlignment = Alignment.CenterVertically,
                        ) {
                            Text(
                                b.name,
                                color = if (current) c.primary else c.text1,
                                fontSize = 14.sp,
                                fontWeight = if (current) FontWeight.Medium else FontWeight.Normal,
                                maxLines = 1,
                                modifier = Modifier.weight(1f),
                            )
                            if (current) {
                                website.msdnna.tessera.ui.components.IonIcon(
                                    Ion.CHECK, size = 16.dp, tint = c.primary, gradient = true,
                                )
                            }
                        }
                    }
                }
            }
        }
    }
}

private val IntegrationProviders = listOf(
    Triple(Ion.GITLAB, "GitLab", true),
    Triple(Ion.GITHUB, "GitHub", false),
)

/**
 * Integrations header switcher: the current provider name + chevron opens a
 * picker of integration settings screens (GitLab now; GitHub etc. later). Mirrors
 * [BoardTitleSwitcher]'s affordance so settings screens feel like board screens.
 */
@Composable
private fun IntegrationTitleSwitcher(title: String, modifier: Modifier = Modifier) {
    val c = Tessera.colors
    var menu by remember { mutableStateOf(false) }
    Box(modifier) {
        Row(
            Modifier.clickableNoRipple { menu = true },
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(title, color = c.text1, fontSize = 17.sp, fontWeight = FontWeight.SemiBold, maxLines = 1)
            Spacer(Modifier.width(4.dp))
            website.msdnna.tessera.ui.components.IonIcon(Ion.CHEVRON_DOWN, size = 18.dp, tint = c.text2)
        }
        TDropdown(expanded = menu, onDismiss = { menu = false }) {
            IntegrationProviders.forEach { (icon, name, available) ->
                val current = name == title
                TMenuItem(
                    if (available) name else "$name · скоро",
                    icon = icon,
                    onClick = { menu = false },
                    trailing = {
                        if (current) {
                            website.msdnna.tessera.ui.components.IonIcon(
                                Ion.CHECK, size = 16.dp, tint = c.primary, gradient = true,
                            )
                        }
                    },
                )
            }
        }
    }
}

private fun titleFor(dest: MainDest): String = when (dest) {
    is MainDest.Home -> "Моя работа"
    is MainDest.Notes -> "Заметки"
    is MainDest.Reminders -> "Напоминания"
    is MainDest.Milestones -> "Этапы"
    is MainDest.GitLabSettings -> "GitLab"
    is MainDest.GitLabJournal -> "Журнал синхронизации"
    is MainDest.Notifications -> "Уведомления"
    is MainDest.Settings -> "Настройки"
    is MainDest.Admin -> "Администрирование"
    is MainDest.BoardView -> dest.board.name
}

/** A stable key for the sidebar's active-row highlight. */
private fun navKeyOf(dest: MainDest): String = when (dest) {
    is MainDest.Home -> "home"
    is MainDest.Notes -> "notes"
    is MainDest.Reminders -> "reminders"
    is MainDest.Milestones -> "milestones"
    is MainDest.GitLabSettings -> "gitlab"
    is MainDest.GitLabJournal -> "gitlab"
    is MainDest.Notifications -> "notifications"
    is MainDest.Settings -> "settings"
    is MainDest.Admin -> "admin"
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
    isIntegration: Boolean,
    projectBoards: List<Board>,
    onSelectBoard: (Board) -> Unit,
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
        when {
            boardId != null -> BoardTitleSwitcher(boardId, title, projectBoards, onSelectBoard, Modifier.weight(1f))

            isIntegration -> IntegrationTitleSwitcher(title, Modifier.weight(1f))

            else -> Text(
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
