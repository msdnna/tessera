package website.msdnna.tessera.ui.components

import androidx.compose.animation.core.FastOutSlowInEasing
import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.gestures.scrollBy
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.key
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.runtime.withFrameNanos
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.draw.rotate
import androidx.compose.ui.focus.FocusManager
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.layout.onGloballyPositioned
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.layout.positionInWindow
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.platform.LocalResources
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.unit.toSize
import androidx.compose.ui.window.Dialog
import coil.compose.AsyncImage
import kotlin.math.roundToInt
import website.msdnna.tessera.R
import website.msdnna.tessera.data.api.RetrofitClient
import website.msdnna.tessera.data.model.Board
import website.msdnna.tessera.data.model.Project
import website.msdnna.tessera.data.model.ProjectGroup
import website.msdnna.tessera.data.model.User
import website.msdnna.tessera.data.model.Workspace
import website.msdnna.tessera.ui.theme.ConflictAmber
import website.msdnna.tessera.ui.theme.RadiusLg
import website.msdnna.tessera.ui.theme.RadiusSm
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.viewmodels.WorkspaceUiState
import website.msdnna.tessera.ui.viewmodels.WorkspaceViewModel
import website.msdnna.tessera.util.Ion
import website.msdnna.tessera.util.WhatsNewSpotlight
import website.msdnna.tessera.util.workspaceCaption

// ── Layout metrics ────────────────────────────────────────────────────────────
// Per-nesting-level indent. Must match the projection step used by the DnD code
// (a child container is offset by exactly this much) so dragging right by one
// level width nests one deeper.
private val IndentStep = 18.dp
private val GutterInset = 8.dp
private const val RootPad = 10f
private fun indentDp(depth: Int): Float = RootPad + depth * IndentStep.value

/** What inline creator is currently open, and where it should attach. */
private sealed interface Creating {
    data class Group(val parentId: String?) : Creating
    data class Project(val groupId: String?) : Creating
    data class Board(val projectId: String) : Creating
}

/** Host callbacks the tree needs. [onOpenBoard] opens a board; [onProjectGone]
 *  fires after a project is deleted/transferred so the host can leave its board
 *  if it's the one open (web navigate-home-on-delete parity). */
private class TreeHost(
    val onOpenBoard: (Board) -> Unit,
    val onProjectGone: (String) -> Unit,
    // Open a project's board scoped to a milestone ("backlog" = milestone-less tasks).
    val onOpenMilestone: (projectId: String, milestoneId: String) -> Unit,
)

/** Shared state threaded through the recursive tree nodes. */
private class TreeCtx(
    val state: WorkspaceUiState,
    val vm: WorkspaceViewModel,
    val drag: SidebarDragState,
    val flat: List<SbNode>,
    val creating: Creating?,
    val setCreating: (Creating?) -> Unit,
    val renaming: String?,
    val setRenaming: (String?) -> Unit,
    // Host callbacks bundled to keep the ctor within the parameter budget.
    val host: TreeHost,
    val onDrop: (SbNode) -> Unit,
    val focus: FocusManager,
) {
    /** Commits any open inline field (blur) before running a row action. */
    fun clickRow(action: () -> Unit) {
        focus.clearFocus()
        action()
    }
}

@Composable
fun Sidebar(
    vm: WorkspaceViewModel,
    state: WorkspaceUiState,
    user: User?,
    isDark: Boolean,
    accentKey: String,
    activeNav: String,
    onAccentChange: (String) -> Unit,
    onToggleDark: () -> Unit,
    onLogout: () -> Unit,
    onOpenHome: () -> Unit,
    onOpenReminders: () -> Unit,
    onOpenNotes: () -> Unit,
    onOpenDocuments: () -> Unit,
    onOpenMilestones: () -> Unit,
    onOpenHelp: () -> Unit,
    onOpenMembers: () -> Unit,
    onOpenGitlab: () -> Unit,
    conflictCount: Int = 0,
    onOpenNotifications: () -> Unit,
    onOpenSettings: () -> Unit,
    onOpenAdmin: () -> Unit,
    onOpenBoard: (Board) -> Unit,
    onProjectGone: (String) -> Unit = {},
    onOpenMilestone: (projectId: String, milestoneId: String) -> Unit = { _, _ -> },
    updateVersion: String? = null,
    onUpdate: () -> Unit = {},
    // Server version shown next to the app's in the footer (#2766), and the
    // one-shot hint to draw at a nav item — null while there is none to show.
    apiVersion: String = "",
    spotlight: WhatsNewSpotlight? = null,
    onDismissSpotlight: (String) -> Unit = {},
) {
    val c = Tessera.colors
    val density = LocalDensity.current
    val focus = LocalFocusManager.current
    val res = LocalResources.current
    var addWorkspace by remember { mutableStateOf(false) }
    var wsMenu by remember { mutableStateOf(false) }
    var wsEstimating by remember { mutableStateOf(false) }
    var confirmDeleteWs by remember { mutableStateOf(false) }
    var addMenu by remember { mutableStateOf(false) }
    var showTheme by remember { mutableStateOf(false) }
    var creating by remember { mutableStateOf<Creating?>(null) }
    var renaming by remember { mutableStateOf<String?>(null) }
    val drag = rememberSidebarDragState()
    val treeScroll = rememberScrollState()
    var viewportH by remember { mutableIntStateOf(0) }

    // Window rect of the row the current hint points at, reported by that row
    // itself. Cleared when the hint moves on, so a stale rect can never place the
    // arrow at a row that is no longer the target (or no longer in the tree).
    var spotTarget by remember { mutableStateOf<Rect?>(null) }
    val spotKey = spotlight?.navKey
    LaunchedEffect(spotKey) { spotTarget = null }
    // Handed only to the targeted row: it makes that row report its rect and nod.
    fun spotSink(navKey: String): ((Rect) -> Unit)? =
        if (spotKey == navKey) ({ rect: Rect -> spotTarget = rect }) else null

    val flat = remember(state.groups, state.projects, state.expandedGroups) { buildFlat(state) }

    val onDrop: (SbNode) -> Unit = onDrop@{ node ->
        if (!drag.movedFar) return@onDrop
        val step = with(density) { IndentStep.toPx() }
        val d = resolveSidebarDrop(drag, flat, step, drag.rootOffset.y) ?: return@onDrop
        when (node.kind) {
            SbKind.GROUP -> vm.moveGroup(node.id, d.parentId, d.beforeId, d.afterId)
            SbKind.PROJECT -> vm.moveProject(node.id, d.parentId, d.beforeId, d.afterId)
        }
    }

    val ctx = TreeCtx(
        state, vm, drag, flat,
        creating, { creating = it },
        renaming, { renaming = it },
        TreeHost(onOpenBoard, onProjectGone, onOpenMilestone), onDrop, focus,
    )

    Box(
        Modifier.fillMaxSize()
            .onGloballyPositioned { drag.rootOffset = it.positionInWindow() }
            .onSizeChanged { viewportH = it.height },
    ) {
        Column(Modifier.fillMaxSize().background(c.surface)) {
            // Brand row + people / palette.
            Row(
                Modifier.fillMaxWidth().padding(start = 12.dp, end = 6.dp, top = 12.dp, bottom = 4.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                // Expanded drawer shows the "tessera" wordmark alone (the mark is for
                // the collapsed/compact contexts); height matched to the header icons.
                BrandLockup(height = 22.dp, mark = false)
                Spacer(Modifier.weight(1f))
                Box {
                    IonIconButton(Ion.GIT_BRANCH, onClick = onOpenGitlab)
                    if (conflictCount > 0) {
                        Box(
                            Modifier.align(Alignment.TopEnd).padding(top = 2.dp, end = 2.dp)
                                .clip(CircleShape).background(ConflictAmber)
                                .padding(horizontal = 4.dp, vertical = 1.dp),
                        ) {
                            Text(
                                if (conflictCount > 9) "9+" else "$conflictCount",
                                color = Color.White,
                                fontSize = 9.sp,
                                fontWeight = FontWeight.SemiBold,
                            )
                        }
                    }
                }
                IonIconButton(Ion.NOTIFICATIONS, onClick = onOpenNotifications)
                IonIconButton(Ion.PEOPLE, onClick = onOpenMembers)
                IonIconButton(Ion.PALETTE, onClick = { showTheme = true })
            }

            // Workspace selector + new-workspace (the one create flow that keeps a modal).
            Row(
                Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 6.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Box(Modifier.weight(1f)) {
                    Row(
                        Modifier.fillMaxWidth().clip(RoundedCornerShape(RadiusSm))
                            .border(1.dp, c.border, RoundedCornerShape(RadiusSm))
                            .clickableNoRipple { wsMenu = true }.padding(horizontal = 10.dp, vertical = 8.dp),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        Text(
                            state.current?.let { workspaceCaption(res, it) } ?: "—",
                            color = c.text1, fontSize = 14.sp, maxLines = 1, modifier = Modifier.weight(1f),
                        )
                        IonIcon(Ion.CHEVRON_DOWN, size = 16.dp, tint = c.text3)
                    }
                    TDropdown(expanded = wsMenu, onDismiss = { wsMenu = false }) {
                        state.workspaces.forEach { ws ->
                            TMenuItem(workspaceCaption(res, ws), onClick = {
                                wsMenu = false
                                vm.selectWorkspace(ws.id)
                            })
                        }
                        if (state.current != null) {
                            TMenuDivider()
                            TMenuItem(stringResource(R.string.sidebar_estimation), icon = Ion.TIME, onClick = {
                                wsMenu = false
                                wsEstimating = true
                            })
                            // Owner-only, and never the last workspace (server re-checks).
                            if (state.current?.ownerId == user?.id && state.workspaces.size > 1) {
                                TMenuItem(stringResource(R.string.sidebar_ws_delete), icon = Ion.TRASH, danger = true, onClick = {
                                    wsMenu = false
                                    confirmDeleteWs = true
                                })
                            }
                        }
                    }
                }
                Spacer(Modifier.width(6.dp))
                IonIconButton(Ion.ADD, onClick = { addWorkspace = true })
            }

            Spacer(Modifier.padding(top = 2.dp))
            NavRow(Ion.HOME, stringResource(R.string.nav_home), activeNav == "home", onOpenHome, spotSink("home"))
            NavRow(Ion.ROCKET, stringResource(R.string.nav_milestones), activeNav == "milestones", onOpenMilestones, spotSink("milestones"))
            NavRow(Ion.ALARM, stringResource(R.string.nav_reminders), activeNav == "reminders", onOpenReminders, spotSink("reminders"))
            NavRow(Ion.DOCUMENT_TEXT, stringResource(R.string.nav_notes), activeNav == "notes", onOpenNotes, spotSink("notes"))
            NavRow(Ion.BOOK, stringResource(R.string.nav_documents), activeNav == "documents", onOpenDocuments, spotSink("documents"))
            NavRow(Ion.HELP_CIRCLE, stringResource(R.string.nav_help), activeNav == "help", onOpenHelp, spotSink("help"))
            if (user?.isAdmin == true) {
                NavRow(
                    Ion.SHIELD_CHECKMARK,
                    stringResource(R.string.nav_admin),
                    active = activeNav == "admin",
                    onClick = onOpenAdmin,
                    spotlight = spotSink("admin"),
                )
            }

            // "ПРОЕКТЫ" section header + add project/group (inline creators).
            Row(
                Modifier.fillMaxWidth().padding(start = 14.dp, end = 6.dp, top = 8.dp, bottom = 2.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(
                    stringResource(R.string.sidebar_projects_header),
                    color = c.text3, fontSize = 11.sp, fontWeight = FontWeight.SemiBold, modifier = Modifier.weight(1f),
                )
                Box {
                    IonIconButton(Ion.ADD, onClick = { addMenu = true }, boxSize = 28.dp, iconSize = 16.dp, tint = c.text3)
                    TDropdown(expanded = addMenu, onDismiss = { addMenu = false }) {
                        TMenuItem(stringResource(R.string.sidebar_add_project), icon = Ion.GRID, onClick = {
                            addMenu = false
                            creating = Creating.Project(null)
                        })
                        TMenuItem(stringResource(R.string.sidebar_add_group), icon = Ion.FOLDER, onClick = {
                            addMenu = false
                            creating = Creating.Group(null)
                        })
                    }
                }
            }

            // Tree. A tap on empty tree space commits/cancels an open inline
            // field (the field commits on blur; tapping away must clear focus).
            Column(
                Modifier.weight(1f).verticalScroll(treeScroll)
                    .pointerInput(Unit) { detectTapGestures { focus.clearFocus() } }
                    .padding(start = RootPad.dp, end = 0.dp, top = 6.dp, bottom = 6.dp),
            ) {
                state.childGroups(null).forEach { key(it.id) { GroupNode(it, 0, ctx) } }
                val rootCreate = creating
                if (rootCreate is Creating.Group && rootCreate.parentId == null) {
                    InlineCreateRow(stringResource(R.string.sidebar_group_name_hint), onDismiss = { creating = null }) {
                        ctx.commitGroup(it, null)
                    }
                }
                state.projectsInGroup(null).forEach { key(it.id) { ProjectNode(it, 0, ctx) } }
                if (rootCreate is Creating.Project && rootCreate.groupId == null) {
                    InlineCreateRow(stringResource(R.string.sidebar_project_name_hint), onDismiss = { creating = null }) {
                        ctx.commitProject(it, null)
                    }
                }
                if (state.groups.isEmpty() && state.projects.isEmpty() && creating == null && !state.loading) {
                    Text(
                        stringResource(R.string.sidebar_empty_tree),
                        color = c.text3, fontSize = 13.sp, modifier = Modifier.padding(16.dp),
                    )
                }
            }

            HorizontalDivider(color = c.border)
            if (updateVersion != null) SidebarUpdateRow(updateVersion, onUpdate)
            SidebarFooter(user, apiVersion, onOpenSettings, onLogout)
        }

        // Drag overlay (insertion line at projected depth + floating clone).
        val drop = if (drag.dragging != null) {
            resolveSidebarDrop(drag, flat, with(density) { IndentStep.toPx() }, drag.rootOffset.y)
        } else {
            null
        }
        SidebarDragOverlay(drag, drop) { depth -> indentDp(depth) }
        SidebarAutoScroll(drag, treeScroll, viewportH)

        // One-shot hint over the tree. The row reports its rect in window
        // coordinates (the drag overlay's convention), so shift it into this
        // Box's own space before drawing.
        if (spotlight != null) {
            SidebarSpotlight(
                spot = spotlight,
                target = spotTarget?.translate(-drag.rootOffset.x, -drag.rootOffset.y),
                onDismiss = { onDismissSpotlight(spotlight.navKey) },
            )
        }
    }

    if (showTheme) {
        ThemePicker(
            currentAccent = accentKey,
            isDark = isDark,
            onSelectAccent = onAccentChange,
            onToggleDark = onToggleDark,
            onDismiss = { showTheme = false },
        )
    }
    if (addWorkspace) {
        TInputDialog(
            stringResource(R.string.sidebar_ws_new_title),
            confirmText = stringResource(R.string.common_create),
            placeholder = stringResource(R.string.sidebar_ws_name_hint),
            onConfirm = {
                vm.addWorkspace(it)
                addWorkspace = false
            },
            onDismiss = { addWorkspace = false },
        )
    }
    state.current?.let { ws ->
        if (wsEstimating) {
            EstimationDialog(
                scope = "workspace",
                name = workspaceCaption(res, ws),
                current = ws.estimation,
                inherited = website.msdnna.tessera.util.Estimation.DEFAULT,
                onSave = { vm.setWorkspaceEstimation(ws.id, it) },
                onDismiss = { wsEstimating = false },
            )
        }
        if (confirmDeleteWs) {
            TConfirmByNameDialog(
                title = stringResource(R.string.sidebar_ws_delete),
                message = stringResource(R.string.sidebar_ws_delete_message, workspaceCaption(res, ws)),
                name = workspaceCaption(res, ws),
                onConfirm = {
                    confirmDeleteWs = false
                    vm.removeWorkspace(ws.id) { onOpenHome() }
                },
                onDismiss = { confirmDeleteWs = false },
            )
        }
    }
}

private fun TreeCtx.commitGroup(name: String, parentId: String?) {
    vm.addGroup(name, parentId)
    setCreating(null)
}

private fun TreeCtx.commitProject(name: String, groupId: String?) {
    vm.addProject(name, groupId)
    setCreating(null)
}

/** Flattens the visible group+project tree into render order for DnD projection. */
private fun buildFlat(state: WorkspaceUiState): List<SbNode> {
    val out = mutableListOf<SbNode>()
    fun walkGroup(g: ProjectGroup, depth: Int) {
        out += SbNode(g.id, SbKind.GROUP, depth, g.parentId, g.name, g.icon, g.color, g.iconMode)
        if (g.id in state.expandedGroups) {
            state.childGroups(g.id).forEach { walkGroup(it, depth + 1) }
            state.projectsInGroup(g.id).forEach {
                out += SbNode(it.id, SbKind.PROJECT, depth + 1, it.groupId, it.name, it.icon, it.color, it.iconMode)
            }
        }
    }
    state.childGroups(null).forEach { walkGroup(it, 0) }
    state.projectsInGroup(null).forEach {
        out += SbNode(it.id, SbKind.PROJECT, 0, it.groupId, it.name, it.icon, it.color, it.iconMode)
    }
    return out
}

/** A primary-destination row (Моя работа / Напоминания / Заметки). */
@Composable
private fun NavRow(
    icon: String,
    label: String,
    active: Boolean,
    onClick: () -> Unit,
    // Non-null only for the row the current spotlight points at (#2766): the row
    // reports its window rect to the overlay and sways towards the hint. The
    // reporting box stays still — measuring the swaying row would jitter the arrow.
    spotlight: ((Rect) -> Unit)? = null,
) {
    val c = Tessera.colors
    val sway = if (spotlight != null) navSway() else 0f
    Box(
        Modifier.fillMaxWidth().onGloballyPositioned { coords ->
            spotlight?.invoke(Rect(coords.positionInWindow(), coords.size.toSize()))
        },
    ) {
        Row(
            Modifier.fillMaxWidth().padding(horizontal = 8.dp, vertical = 1.dp)
                .offset { IntOffset(sway.roundToInt(), 0) }
                .clip(RoundedCornerShape(RadiusSm))
                .background(if (active) c.surfaceAlt else c.surface)
                .clickableNoRipple(onClick = onClick)
                .padding(horizontal = 8.dp, vertical = 9.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            IonIcon(icon, size = 18.dp, tint = if (active) c.primary else c.text2, gradient = active)
            Spacer(Modifier.width(10.dp))
            Text(
                label,
                color = if (active) c.text1 else c.text2,
                fontSize = 14.sp,
                fontWeight = if (active) FontWeight.SemiBold else FontWeight.Medium,
            )
        }
    }
}

/** The spotlighted row's slow sway, in px — small enough to notice without
 *  nagging (the web nods its item once; a phone drawer opens long after the
 *  hint was queued, so here it keeps breathing while the hint is up). */
@Composable
private fun navSway(): Float {
    val shift by rememberInfiniteTransition(label = "nav-sway").animateFloat(
        initialValue = 0f,
        targetValue = 1f,
        animationSpec = infiniteRepeatable(tween(760, easing = FastOutSlowInEasing), RepeatMode.Reverse),
        label = "nav-sway-x",
    )
    return shift * with(LocalDensity.current) { 5.dp.toPx() }
}

@Composable
private fun GroupNode(group: ProjectGroup, depth: Int, ctx: TreeCtx) {
    val expanded = group.id in ctx.state.expandedGroups
    val node = SbNode(group.id, SbKind.GROUP, depth, group.parentId, group.name, group.icon, group.color, group.iconMode)
    TreeRow(
        node = node,
        expanded = expanded,
        hasChildren = true,
        fallbackFolder = true,
        ctx = ctx,
        onClick = { ctx.clickRow { ctx.vm.toggleGroup(group.id) } },
        menu = { close ->
            TMenuItem(stringResource(R.string.sidebar_group_add_project), icon = Ion.ADD, onClick = {
                close()
                ctx.vm.ensureGroupExpanded(group.id)
                ctx.setCreating(Creating.Project(group.id))
            })
            TMenuItem(stringResource(R.string.sidebar_group_add_subgroup), icon = Ion.FOLDER, onClick = {
                close()
                ctx.vm.ensureGroupExpanded(group.id)
                ctx.setCreating(Creating.Group(group.id))
            })
            TMenuItem(stringResource(R.string.common_rename), icon = Ion.PENCIL, onClick = {
                close()
                ctx.setRenaming(group.id)
            })
            TMenuDivider()
            ColumnScopePicker(
                color = group.color,
                icon = group.icon,
                onColor = { ctx.vm.setGroupColor(group, it) },
                onIcon = { ctx.vm.setGroupIcon(group, it) },
                iconMode = group.iconMode,
                onIconMode = { ctx.vm.setGroupIconMode(group, it) },
            )
        },
        deleteMessage = stringResource(R.string.sidebar_group_delete_message, group.name),
        onDelete = { ctx.vm.deleteGroup(group.id) },
    )
    if (expanded) {
        IndentedChildren {
            ctx.state.childGroups(group.id).forEach { key(it.id) { GroupNode(it, depth + 1, ctx) } }
            val create = ctx.creating
            if (create is Creating.Group && create.parentId == group.id) {
                InlineCreateRow(stringResource(R.string.sidebar_group_name_hint), onDismiss = { ctx.setCreating(null) }) {
                    ctx.commitGroup(it, group.id)
                }
            }
            ctx.state.projectsInGroup(group.id).forEach { key(it.id) { ProjectNode(it, depth + 1, ctx) } }
            if (create is Creating.Project && create.groupId == group.id) {
                InlineCreateRow(stringResource(R.string.sidebar_project_name_hint), onDismiss = { ctx.setCreating(null) }) {
                    ctx.commitProject(it, group.id)
                }
            }
        }
    }
}

@Composable
private fun ProjectNode(project: Project, depth: Int, ctx: TreeCtx) {
    val c = Tessera.colors
    val expanded = project.id in ctx.state.expandedProjects
    val node = SbNode(project.id, SbKind.PROJECT, depth, project.groupId, project.name, project.icon, project.color, project.iconMode)
    var estimating by remember { mutableStateOf(false) }
    var transferring by remember { mutableStateOf(false) }
    TreeRow(
        node = node,
        expanded = expanded,
        hasChildren = true,
        fallbackFolder = false,
        ctx = ctx,
        onClick = { ctx.clickRow { ctx.vm.toggleProject(project.id) } },
        menu = { close ->
            TMenuItem(stringResource(R.string.sidebar_project_add_board), icon = Ion.GRID, onClick = {
                close()
                ctx.vm.ensureProjectExpanded(project.id)
                ctx.setCreating(Creating.Board(project.id))
            })
            TMenuItem(stringResource(R.string.common_rename), icon = Ion.PENCIL, onClick = {
                close()
                ctx.setRenaming(project.id)
            })
            TMenuItem(stringResource(R.string.sidebar_estimation), icon = Ion.TIME, onClick = {
                close()
                estimating = true
            })
            if (ctx.state.workspaces.size > 1) {
                TMenuItem(stringResource(R.string.sidebar_project_transfer), icon = Ion.SEND, warn = true, onClick = {
                    close()
                    transferring = true
                })
            }
            TMenuDivider()
            Text(
                stringResource(R.string.sidebar_project_tree_mode),
                color = c.text3, fontSize = 10.sp, fontWeight = FontWeight.SemiBold,
                modifier = Modifier.padding(start = 14.dp, top = 8.dp, bottom = 2.dp),
            )
            // Подписи собираются на каждую рекомпозицию, а не в module-level списке:
            // иначе они бы застыли на языке первого рендера.
            listOf(
                "boards" to stringResource(R.string.sidebar_tree_mode_boards),
                "milestones" to stringResource(R.string.sidebar_tree_mode_milestones),
                "both" to stringResource(R.string.sidebar_tree_mode_both),
            ).forEach { (mode, label) ->
                TMenuItem(
                    label,
                    onClick = {
                        close()
                        ctx.vm.setProjectTreeMode(project, mode)
                    },
                    trailing = {
                        if (project.treeMode == mode) IonIcon(Ion.CHECK, size = 16.dp, tint = c.primary, gradient = true)
                    },
                )
            }
            if (project.treeMode != "boards") {
                val closedOn = project.id in ctx.state.showClosedStages
                TMenuItem(
                    stringResource(R.string.sidebar_show_closed_milestones),
                    onClick = { ctx.vm.toggleShowClosedStages(project.id) },
                    trailing = {
                        if (closedOn) IonIcon(Ion.CHECK, size = 16.dp, tint = c.primary, gradient = true)
                    },
                )
            }
            TMenuDivider()
            ColumnScopePicker(
                color = project.color,
                icon = project.icon,
                onColor = { ctx.vm.setProjectColor(project, it) },
                onIcon = { ctx.vm.setProjectIcon(project, it) },
                iconMode = project.iconMode,
                onIconMode = { ctx.vm.setProjectIconMode(project, it) },
            )
        },
        deleteMessage = stringResource(R.string.sidebar_project_delete_message, project.name),
        onDelete = {
            ctx.vm.deleteProject(project.id)
            // Leave the project's board for Home if it's the one open (web parity).
            ctx.host.onProjectGone(project.id)
        },
        confirmName = project.name,
    )
    if (estimating) {
        EstimationDialog(
            scope = "project",
            name = project.name,
            current = project.estimation,
            inherited = ctx.state.current?.estimation ?: website.msdnna.tessera.util.Estimation.DEFAULT,
            onSave = { ctx.vm.setProjectEstimation(project.id, it) },
            onDismiss = { estimating = false },
        )
    }
    if (transferring) {
        val androidCtx = LocalContext.current
        // Тост показывается уже вне композиции, поэтому строку берём из тех же
        // ресурсов, что подменил AppLocale, а не из LocalContext (он на системной локали).
        val res = LocalResources.current
        TransferProjectDialog(
            project = project,
            targets = ctx.state.workspaces.filter { it.id != ctx.state.currentId },
            onTransfer = { targetId ->
                ctx.vm.transferProject(project.id, targetId) { stripped ->
                    ctx.host.onProjectGone(project.id)
                    val msg = if (stripped > 0) {
                        res.getString(R.string.sidebar_project_transferred_stripped, stripped)
                    } else {
                        res.getString(R.string.sidebar_project_transferred)
                    }
                    android.widget.Toast.makeText(androidCtx, msg, android.widget.Toast.LENGTH_SHORT).show()
                }
                transferring = false
            },
            onDismiss = { transferring = false },
        )
    }
    if (expanded) {
        IndentedChildren {
            val showMilestones = project.treeMode == "milestones" || project.treeMode == "both"
            val showBoards = project.treeMode == "boards" || project.treeMode == "both"
            if (showMilestones) {
                val loaded = ctx.state.milestonesByProject[project.id]
                if (loaded == null) {
                    Text(
                        stringResource(R.string.common_loading),
                        color = c.text3, fontSize = 12.sp,
                        modifier = Modifier.padding(start = 6.dp, top = 4.dp, bottom = 4.dp),
                    )
                } else {
                    // Backlog first, then open milestones (due asc, undated last), then
                    // closed (only when the per-project toggle is on) — mirrors web ProjectRow.
                    val showClosed = project.id in ctx.state.showClosedStages
                    val ms = loaded
                        .filter { showClosed || !it.isClosed }
                        .sortedWith(compareBy({ it.isClosed }, { it.dueDate ?: "￿" }))
                    MilestoneRow(project.id, null, stringResource(R.string.sidebar_backlog), ctx)
                    ms.forEach { m -> key(m.id) { MilestoneRow(project.id, m.id, m.title, ctx, closed = m.isClosed) } }
                }
            }
            if (showBoards) {
                val boards = ctx.state.boardsByProject[project.id]
                if (boards == null) {
                    Text(
                        stringResource(R.string.common_loading),
                        color = c.text3, fontSize = 12.sp,
                        modifier = Modifier.padding(start = 6.dp, top = 4.dp, bottom = 4.dp),
                    )
                } else {
                    boards.forEach { b -> key(b.id) { BoardRow(b, project.id, ctx) } }
                    if (boards.isEmpty() && ctx.creating !is Creating.Board) {
                        Text(
                            stringResource(R.string.sidebar_no_boards),
                            color = c.text3, fontSize = 12.sp,
                            modifier = Modifier.padding(start = 6.dp, top = 4.dp, bottom = 4.dp),
                        )
                    }
                }
                val create = ctx.creating
                if (create is Creating.Board && create.projectId == project.id) {
                    InlineCreateRow(stringResource(R.string.sidebar_board_name_hint), onDismiss = { ctx.setCreating(null) }) {
                        ctx.vm.addBoard(project.id, it)
                        ctx.setCreating(null)
                    }
                }
            }
        }
    }
}

/** A stage («Этап») row in the sidebar tree: taps open the project's board scoped to
 *  the milestone. [milestoneId] null → the "Бэклог" node (milestone-less tasks). */
@Composable
private fun MilestoneRow(projectId: String, milestoneId: String?, title: String, ctx: TreeCtx, closed: Boolean = false) {
    val c = Tessera.colors
    Row(
        Modifier.fillMaxWidth().animatePlacement().clip(RoundedCornerShape(RadiusSm))
            .clickableNoRipple { ctx.clickRow { ctx.host.onOpenMilestone(projectId, milestoneId ?: "backlog") } }
            .padding(end = 6.dp, top = 7.dp, bottom = 7.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Spacer(Modifier.width(16.dp)) // chevron slot (leaf row)
        Spacer(Modifier.width(4.dp))
        Box(Modifier.size(20.dp), contentAlignment = Alignment.Center) {
            IonIcon(if (milestoneId == null) Ion.GIT_BRANCH else Ion.ROCKET, size = 16.dp, tint = c.text3)
        }
        Spacer(Modifier.width(9.dp))
        Text(
            title,
            color = if (closed) c.text3 else c.text2,
            fontSize = 13.sp,
            maxLines = 1,
            modifier = Modifier.weight(1f),
        )
    }
}

/** Modal to pick a target workspace for a project transfer (web ProjectRow parity):
 *  a warning about what moves, a workspace list, and an orange-toned confirm. */
@Composable
private fun TransferProjectDialog(
    project: Project,
    targets: List<Workspace>,
    onTransfer: (String) -> Unit,
    onDismiss: () -> Unit,
) {
    val c = Tessera.colors
    var selected by remember { mutableStateOf<String?>(null) }
    Dialog(onDismissRequest = onDismiss) {
        Column(
            Modifier.fillMaxWidth().clip(RoundedCornerShape(RadiusLg)).background(c.surface).padding(18.dp),
        ) {
            Text(
                stringResource(R.string.sidebar_transfer_title),
                color = c.text1, fontSize = 16.sp, fontWeight = FontWeight.SemiBold,
            )
            Spacer(Modifier.height(8.dp))
            Text(
                stringResource(R.string.sidebar_transfer_message, project.name),
                color = c.text3, fontSize = 13.sp,
            )
            Spacer(Modifier.height(12.dp))
            Column(Modifier.heightIn(max = 260.dp).verticalScroll(rememberScrollState())) {
                targets.forEach { ws ->
                    val on = ws.id == selected
                    Row(
                        Modifier.fillMaxWidth().clip(RoundedCornerShape(RadiusSm))
                            .then(if (on) Modifier.background(c.primary.copy(alpha = 0.12f)) else Modifier)
                            .clickableNoRipple { selected = ws.id }
                            .padding(horizontal = 10.dp, vertical = 10.dp),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        Text(
                            workspaceCaption(LocalResources.current, ws),
                            color = c.text1, fontSize = 14.sp, modifier = Modifier.weight(1f),
                        )
                        if (on) IonIcon(Ion.CHECK, size = 16.dp, tint = c.primary, gradient = true)
                    }
                }
            }
            Spacer(Modifier.height(14.dp))
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End, verticalAlignment = Alignment.CenterVertically) {
                TButton(stringResource(R.string.common_cancel), kind = TButtonKind.Ghost, onClick = onDismiss)
                Spacer(Modifier.width(8.dp))
                TButton(
                    stringResource(R.string.sidebar_transfer_confirm),
                    enabled = selected != null,
                    onClick = { selected?.let(onTransfer) },
                )
            }
        }
    }
}

@Composable
private fun BoardRow(board: Board, projectId: String, ctx: TreeCtx) {
    val c = Tessera.colors
    var menu by remember { mutableStateOf(false) }
    var confirmDelete by remember { mutableStateOf(false) }
    val renaming = ctx.renaming == board.id
    Row(
        Modifier.fillMaxWidth().animatePlacement().clip(RoundedCornerShape(RadiusSm))
            .then(if (!renaming) Modifier.clickableNoRipple { ctx.clickRow { ctx.host.onOpenBoard(board) } } else Modifier)
            .padding(end = 6.dp, top = 7.dp, bottom = 7.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Spacer(Modifier.width(16.dp)) // chevron slot (boards have no children)
        Spacer(Modifier.width(4.dp))
        Box(Modifier.size(20.dp), contentAlignment = Alignment.Center) {
            // Board icon: the chosen glyph (badge/icon), else the default kanban glyph
            // (web board default), coloured/badged like projects & groups.
            ProjectIcon(
                board.name, board.icon, board.color,
                size = 18.dp, iconMode = board.iconMode, fallbackGlyph = "layout_kanban_outline",
            )
        }
        Spacer(Modifier.width(9.dp))
        if (renaming) {
            InlineTitleEditor(
                initial = board.name,
                onCommit = {
                    ctx.vm.renameBoard(projectId, board.id, it)
                    ctx.setRenaming(null)
                },
                onCancel = { ctx.setRenaming(null) },
                modifier = Modifier.weight(1f),
                fontSize = 13.sp,
            )
        } else {
            Text(board.name, color = c.text2, fontSize = 13.sp, maxLines = 1, modifier = Modifier.weight(1f))
        }
        Box {
            IonIconButton(Ion.ELLIPSIS_H, onClick = { menu = true }, boxSize = 26.dp, iconSize = 17.dp, tint = c.text3)
            TDropdown(expanded = menu, onDismiss = { menu = false }) {
                TMenuItem(stringResource(R.string.common_rename), icon = Ion.PENCIL, onClick = {
                    menu = false
                    ctx.setRenaming(board.id)
                })
                TMenuItem(stringResource(R.string.common_delete), icon = Ion.TRASH, danger = true, onClick = {
                    menu = false
                    confirmDelete = true
                })
            }
            TConfirmPopover(
                expanded = confirmDelete,
                message = stringResource(R.string.sidebar_board_delete_message, board.name),
                onConfirm = {
                    confirmDelete = false
                    ctx.vm.deleteBoard(projectId, board.id)
                },
                onDismiss = { confirmDelete = false },
            )
        }
    }
}

/** A group / project row: aligned chevron + icon + label, a themed "⋯" menu with
 *  a delete confirm-popover, long-press drag, and inline rename. */
@Composable
private fun TreeRow(
    node: SbNode,
    expanded: Boolean,
    hasChildren: Boolean,
    fallbackFolder: Boolean,
    ctx: TreeCtx,
    onClick: () -> Unit,
    menu: @Composable (close: () -> Unit) -> Unit,
    deleteMessage: String,
    onDelete: () -> Unit,
    confirmName: String? = null,
) {
    val c = Tessera.colors
    var menuOpen by remember { mutableStateOf(false) }
    var confirmDelete by remember { mutableStateOf(false) }
    val renaming = ctx.renaming == node.id
    val dragging = ctx.drag.dragging?.id == node.id

    Row(
        Modifier.fillMaxWidth()
            .animatePlacement()
            .sidebarDragDim(dragging)
            .clip(RoundedCornerShape(RadiusSm))
            .then(if (!renaming) Modifier.clickableNoRipple(onClick = onClick) else Modifier)
            .sidebarRowBounds(ctx.drag, node.id)
            .then(if (!renaming) Modifier.draggableSidebarRow(ctx.drag, node, onDrop = { ctx.onDrop(node) }) else Modifier)
            .padding(end = 6.dp, top = 7.dp, bottom = 7.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Box(Modifier.size(16.dp), contentAlignment = Alignment.Center) {
            if (hasChildren) {
                IonIcon(Ion.CHEVRON_FORWARD, size = 15.dp, tint = c.text3, modifier = Modifier.rotate(if (expanded) 90f else 0f))
            }
        }
        Spacer(Modifier.width(4.dp))
        Box(Modifier.size(20.dp), contentAlignment = Alignment.Center) {
            ProjectIcon(name = node.name, icon = node.icon, color = node.color, size = 20.dp, fallbackFolder = fallbackFolder, iconMode = node.iconMode)
        }
        Spacer(Modifier.width(9.dp))
        if (renaming) {
            InlineTitleEditor(
                initial = node.name,
                onCommit = {
                    commitRename(ctx, node, it)
                    ctx.setRenaming(null)
                },
                onCancel = { ctx.setRenaming(null) },
                modifier = Modifier.weight(1f),
                fontSize = 14.sp,
            )
        } else {
            Text(node.name, color = c.text1, fontSize = 14.sp, maxLines = 1, modifier = Modifier.weight(1f))
        }
        Box {
            IonIconButton(Ion.ELLIPSIS_H, onClick = { menuOpen = true }, boxSize = 26.dp, iconSize = 17.dp, tint = c.text3)
            TDropdown(expanded = menuOpen, onDismiss = { menuOpen = false }) {
                menu { menuOpen = false }
                TMenuDivider()
                TMenuItem(stringResource(R.string.common_delete), icon = Ion.TRASH, danger = true, onClick = {
                    menuOpen = false
                    confirmDelete = true
                })
            }
            // High-risk nodes (projects) require typing the name; others use a
            // quick popconfirm.
            if (confirmName != null) {
                if (confirmDelete) {
                    TConfirmByNameDialog(
                        title = stringResource(R.string.sidebar_project_delete_title),
                        message = deleteMessage,
                        name = confirmName,
                        onConfirm = {
                            confirmDelete = false
                            onDelete()
                        },
                        onDismiss = { confirmDelete = false },
                    )
                }
            } else {
                TConfirmPopover(
                    expanded = confirmDelete,
                    message = deleteMessage,
                    onConfirm = {
                        confirmDelete = false
                        onDelete()
                    },
                    onDismiss = { confirmDelete = false },
                )
            }
        }
    }
}

private fun commitRename(ctx: TreeCtx, node: SbNode, name: String) {
    if (name == node.name) return // unchanged — skip the round-trip (mirrors the web)
    when (node.kind) {
        SbKind.GROUP -> ctx.state.groups.firstOrNull { it.id == node.id }?.let { ctx.vm.renameGroup(it, name) }
        SbKind.PROJECT -> ctx.state.projects.firstOrNull { it.id == node.id }?.let { ctx.vm.renameProject(it, name) }
    }
}

/** Wraps a group's children in an indented column, drawing a left gutter line
 *  (the nesting guide) the full height of the children block. */
@Composable
private fun IndentedChildren(content: @Composable ColumnScope.() -> Unit) {
    val c = Tessera.colors
    Column(
        Modifier.fillMaxWidth()
            .drawBehind {
                val x = GutterInset.toPx()
                drawLine(c.border, Offset(x, 0f), Offset(x, size.height), strokeWidth = 1.dp.toPx())
            }
            .padding(start = IndentStep),
        content = content,
    )
}

/** An inline single-line creator placed in the tree (no modal). */
@Composable
private fun InlineCreateRow(placeholder: String, onDismiss: () -> Unit, onCommit: (String) -> Unit) {
    Box(Modifier.fillMaxWidth().padding(end = 8.dp, top = 3.dp, bottom = 3.dp)) {
        InlineCreateField(
            placeholder = placeholder,
            onCommit = onCommit,
            onDismiss = onDismiss,
        )
    }
}

/**
 * Persistent "update available" entry — mirrors the update dialog so the user
 * can still grab an update after tapping "Позже" (which only hides the dialog
 * for the session). Shown only when [UpdateViewModel.available] is non-null.
 */
@Composable
private fun SidebarUpdateRow(version: String, onUpdate: () -> Unit) {
    val c = Tessera.colors
    Row(
        Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        IonIcon(Ion.DOWNLOAD, size = 18.dp, tint = c.primary, gradient = true)
        Spacer(Modifier.width(10.dp))
        Column(Modifier.weight(1f)) {
            Text(
                stringResource(R.string.update_available),
                color = c.text1, fontSize = 13.sp, fontWeight = FontWeight.Medium, maxLines = 1,
            )
            Text(version, color = c.text3, fontSize = 12.sp, maxLines = 1)
        }
        TButton(stringResource(R.string.update_action), onClick = onUpdate)
    }
}

@Composable
private fun SidebarFooter(user: User?, apiVersion: String, onOpenSettings: () -> Unit, onLogout: () -> Unit) {
    val c = Tessera.colors
    Row(
        Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 10.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        // Tapping the user (avatar/name) opens account settings.
        Row(
            Modifier.weight(1f).clickableNoRipple(onClick = onOpenSettings),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            val avatarUrl = user?.avatarUrl?.takeIf { it.isNotBlank() }
            if (avatarUrl != null) {
                AsyncImage(
                    model = "${RetrofitClient.serverRoot}$avatarUrl",
                    contentDescription = null,
                    modifier = Modifier.size(32.dp).clip(CircleShape),
                )
            } else {
                ProjectIcon(name = user?.name.orEmpty().ifBlank { "?" }, icon = "", color = "", size = 32.dp)
            }
            Spacer(Modifier.width(10.dp))
            Column(Modifier.weight(1f)) {
                val fallbackName = stringResource(R.string.sidebar_user_fallback)
                Text(user?.name?.ifBlank { fallbackName } ?: fallbackName, color = c.text1, fontSize = 14.sp, maxLines = 1)
                val email = user?.email.orEmpty()
                if (email.isNotBlank()) Text(email, color = c.text3, fontSize = 12.sp, maxLines = 1)
                // App version, plus the server's own once it answers /version —
                // when they disagree it's the first thing worth seeing (#2766).
                val versions = "v${website.msdnna.tessera.BuildConfig.VERSION_NAME}" +
                    if (apiVersion.isNotBlank()) " · API $apiVersion" else ""
                Text(versions, color = c.text3, fontSize = 11.sp, maxLines = 1)
            }
        }
        IonIconButton(Ion.SETTINGS, onClick = onOpenSettings)
        IonIconButton(Ion.LOGOUT, onClick = onLogout)
    }
}

/** Auto-scrolls the tree while a node is dragged near the top/bottom edge. */
@Composable
private fun SidebarAutoScroll(drag: SidebarDragState, scroll: androidx.compose.foundation.ScrollState, viewportH: Int) {
    val density = LocalDensity.current
    LaunchedEffect(drag.dragging != null) {
        if (drag.dragging == null) return@LaunchedEffect
        val edge = with(density) { 64.dp.toPx() }
        while (drag.dragging != null) {
            val y = drag.pointer.y - drag.rootOffset.y
            val dy = when {
                y < edge -> -12f
                y > viewportH - edge -> 12f
                else -> 0f
            }
            if (dy != 0f) scroll.scrollBy(dy)
            withFrameNanos { }
        }
    }
}
