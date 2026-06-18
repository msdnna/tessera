package website.msdnna.tessera.ui.components

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
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.layout.onGloballyPositioned
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.layout.positionInWindow
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import coil.compose.AsyncImage
import website.msdnna.tessera.data.api.RetrofitClient
import website.msdnna.tessera.data.model.Board
import website.msdnna.tessera.data.model.Project
import website.msdnna.tessera.data.model.ProjectGroup
import website.msdnna.tessera.data.model.User
import website.msdnna.tessera.ui.theme.RadiusSm
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.viewmodels.WorkspaceUiState
import website.msdnna.tessera.ui.viewmodels.WorkspaceViewModel
import website.msdnna.tessera.util.Ion

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
    val onOpenBoard: (Board) -> Unit,
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
    onOpenMembers: () -> Unit,
    onOpenGitlab: () -> Unit,
    onOpenNotifications: () -> Unit,
    onOpenSettings: () -> Unit,
    onOpenAdmin: () -> Unit,
    onOpenBoard: (Board) -> Unit,
    updateVersion: String? = null,
    onUpdate: () -> Unit = {},
) {
    val c = Tessera.colors
    val density = LocalDensity.current
    val focus = LocalFocusManager.current
    var addWorkspace by remember { mutableStateOf(false) }
    var wsMenu by remember { mutableStateOf(false) }
    var addMenu by remember { mutableStateOf(false) }
    var showTheme by remember { mutableStateOf(false) }
    var creating by remember { mutableStateOf<Creating?>(null) }
    var renaming by remember { mutableStateOf<String?>(null) }
    val drag = rememberSidebarDragState()
    val treeScroll = rememberScrollState()
    var viewportH by remember { mutableIntStateOf(0) }

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
        onOpenBoard, onDrop, focus,
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
                MtLogo(size = 22.dp)
                // Brand wordmark intentionally omitted — the spacer reserves room
                // for future header controls while keeping the icons right-aligned.
                Spacer(Modifier.weight(1f))
                IonIconButton(Ion.GIT_BRANCH, onClick = onOpenGitlab)
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
                        Text(state.current?.name ?: "—", color = c.text1, fontSize = 14.sp, maxLines = 1, modifier = Modifier.weight(1f))
                        IonIcon(Ion.CHEVRON_DOWN, size = 16.dp, tint = c.text3)
                    }
                    TDropdown(expanded = wsMenu, onDismiss = { wsMenu = false }) {
                        state.workspaces.forEach { ws ->
                            TMenuItem(ws.name, onClick = {
                                wsMenu = false
                                vm.selectWorkspace(ws.id)
                            })
                        }
                    }
                }
                Spacer(Modifier.width(6.dp))
                IonIconButton(Ion.ADD, onClick = { addWorkspace = true })
            }

            Spacer(Modifier.padding(top = 2.dp))
            NavRow(Ion.HOME, "Моя работа", active = activeNav == "home", onClick = onOpenHome)
            NavRow(Ion.ALARM, "Напоминания", active = activeNav == "reminders", onClick = onOpenReminders)
            NavRow(Ion.DOCUMENT_TEXT, "Заметки", active = activeNav == "notes", onClick = onOpenNotes)
            if (user?.isAdmin == true) {
                NavRow(
                    Ion.SHIELD_CHECKMARK,
                    "Администрирование",
                    active = activeNav == "admin",
                    onClick = onOpenAdmin,
                )
            }

            // "ПРОЕКТЫ" section header + add project/group (inline creators).
            Row(
                Modifier.fillMaxWidth().padding(start = 14.dp, end = 6.dp, top = 8.dp, bottom = 2.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text("ПРОЕКТЫ", color = c.text3, fontSize = 11.sp, fontWeight = FontWeight.SemiBold, modifier = Modifier.weight(1f))
                Box {
                    IonIconButton(Ion.ADD, onClick = { addMenu = true }, boxSize = 28.dp, iconSize = 16.dp, tint = c.text3)
                    TDropdown(expanded = addMenu, onDismiss = { addMenu = false }) {
                        TMenuItem("Проект", icon = Ion.GRID, onClick = {
                            addMenu = false
                            creating = Creating.Project(null)
                        })
                        TMenuItem("Группа", icon = Ion.FOLDER, onClick = {
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
                    InlineCreateRow("Название группы", onDismiss = { creating = null }) { ctx.commitGroup(it, null) }
                }
                state.projectsInGroup(null).forEach { key(it.id) { ProjectNode(it, 0, ctx) } }
                if (rootCreate is Creating.Project && rootCreate.groupId == null) {
                    InlineCreateRow("Название проекта", onDismiss = { creating = null }) { ctx.commitProject(it, null) }
                }
                if (state.groups.isEmpty() && state.projects.isEmpty() && creating == null && !state.loading) {
                    Text("Нет проектов. Нажмите +, чтобы создать.", color = c.text3, fontSize = 13.sp, modifier = Modifier.padding(16.dp))
                }
            }

            HorizontalDivider(color = c.border)
            if (updateVersion != null) SidebarUpdateRow(updateVersion, onUpdate)
            SidebarFooter(user, onOpenSettings, onLogout)
        }

        // Drag overlay (insertion line at projected depth + floating clone).
        val drop = if (drag.dragging != null) {
            resolveSidebarDrop(drag, flat, with(density) { IndentStep.toPx() }, drag.rootOffset.y)
        } else {
            null
        }
        SidebarDragOverlay(drag, drop) { depth -> indentDp(depth) }
        SidebarAutoScroll(drag, treeScroll, viewportH)
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
            "Новое пространство", confirmText = "Создать", placeholder = "Название",
            onConfirm = {
                vm.addWorkspace(it)
                addWorkspace = false
            },
            onDismiss = { addWorkspace = false },
        )
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
        out += SbNode(g.id, SbKind.GROUP, depth, g.parentId, g.name, g.icon, g.color)
        if (g.id in state.expandedGroups) {
            state.childGroups(g.id).forEach { walkGroup(it, depth + 1) }
            state.projectsInGroup(g.id).forEach {
                out += SbNode(it.id, SbKind.PROJECT, depth + 1, it.groupId, it.name, it.icon, it.color)
            }
        }
    }
    state.childGroups(null).forEach { walkGroup(it, 0) }
    state.projectsInGroup(null).forEach {
        out += SbNode(it.id, SbKind.PROJECT, 0, it.groupId, it.name, it.icon, it.color)
    }
    return out
}

/** A primary-destination row (Моя работа / Напоминания / Заметки). */
@Composable
private fun NavRow(icon: String, label: String, active: Boolean, onClick: () -> Unit) {
    val c = Tessera.colors
    Row(
        Modifier.fillMaxWidth().padding(horizontal = 8.dp, vertical = 1.dp)
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

@Composable
private fun GroupNode(group: ProjectGroup, depth: Int, ctx: TreeCtx) {
    val expanded = group.id in ctx.state.expandedGroups
    val node = SbNode(group.id, SbKind.GROUP, depth, group.parentId, group.name, group.icon, group.color)
    TreeRow(
        node = node,
        expanded = expanded,
        hasChildren = true,
        fallbackFolder = true,
        ctx = ctx,
        onClick = { ctx.clickRow { ctx.vm.toggleGroup(group.id) } },
        menu = { close ->
            TMenuItem("Добавить проект", icon = Ion.ADD, onClick = {
                close()
                ctx.vm.ensureGroupExpanded(group.id)
                ctx.setCreating(Creating.Project(group.id))
            })
            TMenuItem("Добавить подгруппу", icon = Ion.FOLDER, onClick = {
                close()
                ctx.vm.ensureGroupExpanded(group.id)
                ctx.setCreating(Creating.Group(group.id))
            })
            TMenuItem("Переименовать", icon = Ion.PENCIL, onClick = {
                close()
                ctx.setRenaming(group.id)
            })
            TMenuDivider()
            ColumnScopePicker(
                color = group.color,
                icon = group.icon,
                onColor = { ctx.vm.setGroupColor(group, it) },
                onIcon = { ctx.vm.setGroupIcon(group, it) },
            )
        },
        deleteMessage = "Удалить «${group.name}»? Подгруппы вложатся выше, проекты станут без группы.",
        onDelete = { ctx.vm.deleteGroup(group.id) },
    )
    if (expanded) {
        IndentedChildren {
            ctx.state.childGroups(group.id).forEach { key(it.id) { GroupNode(it, depth + 1, ctx) } }
            val create = ctx.creating
            if (create is Creating.Group && create.parentId == group.id) {
                InlineCreateRow("Название группы", onDismiss = { ctx.setCreating(null) }) { ctx.commitGroup(it, group.id) }
            }
            ctx.state.projectsInGroup(group.id).forEach { key(it.id) { ProjectNode(it, depth + 1, ctx) } }
            if (create is Creating.Project && create.groupId == group.id) {
                InlineCreateRow("Название проекта", onDismiss = { ctx.setCreating(null) }) { ctx.commitProject(it, group.id) }
            }
        }
    }
}

@Composable
private fun ProjectNode(project: Project, depth: Int, ctx: TreeCtx) {
    val c = Tessera.colors
    val expanded = project.id in ctx.state.expandedProjects
    val node = SbNode(project.id, SbKind.PROJECT, depth, project.groupId, project.name, project.icon, project.color)
    TreeRow(
        node = node,
        expanded = expanded,
        hasChildren = true,
        fallbackFolder = false,
        ctx = ctx,
        onClick = { ctx.clickRow { ctx.vm.toggleProject(project.id) } },
        menu = { close ->
            TMenuItem("Добавить доску", icon = Ion.GRID, onClick = {
                close()
                ctx.vm.ensureProjectExpanded(project.id)
                ctx.setCreating(Creating.Board(project.id))
            })
            TMenuItem("Переименовать", icon = Ion.PENCIL, onClick = {
                close()
                ctx.setRenaming(project.id)
            })
            TMenuDivider()
            ColumnScopePicker(
                color = project.color,
                icon = project.icon,
                onColor = { ctx.vm.setProjectColor(project, it) },
                onIcon = { ctx.vm.setProjectIcon(project, it) },
            )
        },
        deleteMessage = "Проект «${project.name}» будет удалён со всеми досками и задачами. Действие необратимо.",
        onDelete = { ctx.vm.deleteProject(project.id) },
        confirmName = project.name,
    )
    if (expanded) {
        IndentedChildren {
            val boards = ctx.state.boardsByProject[project.id]
            if (boards == null) {
                Text("Загрузка…", color = c.text3, fontSize = 12.sp, modifier = Modifier.padding(start = 6.dp, top = 4.dp, bottom = 4.dp))
            } else {
                boards.forEach { b -> key(b.id) { BoardRow(b, project.id, ctx) } }
                if (boards.isEmpty() && ctx.creating !is Creating.Board) {
                    Text("Нет досок", color = c.text3, fontSize = 12.sp, modifier = Modifier.padding(start = 6.dp, top = 4.dp, bottom = 4.dp))
                }
            }
            val create = ctx.creating
            if (create is Creating.Board && create.projectId == project.id) {
                InlineCreateRow("Название доски", onDismiss = { ctx.setCreating(null) }) {
                    ctx.vm.addBoard(project.id, it)
                    ctx.setCreating(null)
                }
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
            .then(if (!renaming) Modifier.clickableNoRipple { ctx.clickRow { ctx.onOpenBoard(board) } } else Modifier)
            .padding(end = 6.dp, top = 7.dp, bottom = 7.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Spacer(Modifier.width(16.dp)) // chevron slot (boards have no children)
        Spacer(Modifier.width(4.dp))
        Box(Modifier.size(20.dp), contentAlignment = Alignment.Center) {
            IonIcon(Ion.GRID, size = 16.dp, tint = c.text3)
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
                TMenuItem("Переименовать", icon = Ion.PENCIL, onClick = {
                    menu = false
                    ctx.setRenaming(board.id)
                })
                TMenuItem("Удалить", icon = Ion.TRASH, danger = true, onClick = {
                    menu = false
                    confirmDelete = true
                })
            }
            TConfirmPopover(
                expanded = confirmDelete,
                message = "Удалить доску «${board.name}»?",
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
            ProjectIcon(name = node.name, icon = node.icon, color = node.color, size = 20.dp, fallbackFolder = fallbackFolder)
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
                TMenuItem("Удалить", icon = Ion.TRASH, danger = true, onClick = {
                    menuOpen = false
                    confirmDelete = true
                })
            }
            // High-risk nodes (projects) require typing the name; others use a
            // quick popconfirm.
            if (confirmName != null) {
                if (confirmDelete) {
                    TConfirmByNameDialog(
                        title = "Удалить проект",
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
            Text("Доступно обновление", color = c.text1, fontSize = 13.sp, fontWeight = FontWeight.Medium, maxLines = 1)
            Text(version, color = c.text3, fontSize = 12.sp, maxLines = 1)
        }
        TButton("Обновить", onClick = onUpdate)
    }
}

@Composable
private fun SidebarFooter(user: User?, onOpenSettings: () -> Unit, onLogout: () -> Unit) {
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
                Text(user?.name?.ifBlank { "Пользователь" } ?: "Пользователь", color = c.text1, fontSize = 14.sp, maxLines = 1)
                val email = user?.email.orEmpty()
                if (email.isNotBlank()) Text(email, color = c.text3, fontSize = 12.sp, maxLines = 1)
                Text("v${website.msdnna.tessera.BuildConfig.VERSION_NAME}", color = c.text3, fontSize = 11.sp, maxLines = 1)
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
