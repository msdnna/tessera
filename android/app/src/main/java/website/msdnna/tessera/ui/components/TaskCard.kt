package website.msdnna.tessera.ui.components

import android.content.Intent
import android.net.Uri
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.lerp
import androidx.compose.ui.layout.Layout
import androidx.compose.ui.layout.layout
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Constraints
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.zIndex
import coil.compose.AsyncImage
import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.api.RetrofitClient
import website.msdnna.tessera.data.model.BoardColumn
import website.msdnna.tessera.data.model.Tag
import website.msdnna.tessera.data.model.Task
import website.msdnna.tessera.ui.TestTags
import website.msdnna.tessera.ui.theme.ConflictAmber
import website.msdnna.tessera.ui.theme.PriorityColors
import website.msdnna.tessera.ui.theme.PriorityLabels
import website.msdnna.tessera.ui.theme.RadiusLg
import website.msdnna.tessera.ui.theme.RadiusSm
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.accentGradient
import website.msdnna.tessera.ui.viewmodels.BoardUiState
import website.msdnna.tessera.ui.viewmodels.BoardViewModel
import website.msdnna.tessera.util.Ion
import website.msdnna.tessera.util.buildMentionItems
import website.msdnna.tessera.util.buildTagGroups
import website.msdnna.tessera.util.divergedColumn
import website.msdnna.tessera.util.dueShort
import website.msdnna.tessera.util.isOverdue
import website.msdnna.tessera.util.onColor
import website.msdnna.tessera.util.parseHexColor
import website.msdnna.tessera.util.readableHue
import website.msdnna.tessera.util.shortDate
import website.msdnna.tessera.util.tagParts

/**
 * A kanban card mirroring the web `TaskCard`: completion check, title, #number,
 * a "⋯" quick-actions menu, a pills row (priority / due / tags) with the assignee
 * pinned bottom-right, and subtasks as compact full-width rows with an inline
 * "create subtask" field. Tap opens the modal; long-press body = drag,
 * long-press title = inline rename.
 */
@Composable
fun TaskCard(
    task: Task,
    state: BoardUiState,
    vm: BoardViewModel,
    onOpen: (Task) -> Unit,
    modifier: Modifier = Modifier,
    nested: Boolean = false,
    /** Column of the card this one is nested under — a nested card whose own
     *  column differs shows a marker (it ran ahead of / behind its parent). */
    parentColumnId: String? = null,
    compact: Boolean = false,
    drag: BoardDragState? = null,
    onDropTask: ((Task) -> Unit)? = null,
    nestSlot: Pair<String?, String?>? = null,
    conflictTaskIds: Set<String> = emptySet(),
    onOpenConflict: ((Task) -> Unit)? = null,
    /** Marks this as the card the user can actually touch, so it carries an e2e
     *  anchor ([TestTags.taskCard]) — and, by staying `false` by default, keeps
     *  the drag ghosts and preview clones (which render the *same* task) out of
     *  the tag namespace. Two nodes under one tag would break every board spec
     *  the moment a drag started, so the opt-in is the point. Propagates to this
     *  card's subtasks, which are equally real. */
    anchored: Boolean = false,
) {
    val c = Tessera.colors
    // Keep ALL subtasks composed during a drag (removing the dragged one would
    // dispose its gesture node and cancel the drag); the dragged one just dims.
    val subtasks = if (nested || compact) emptyList() else state.visibleSubtasksOf(task.id)
    val accent = PriorityColors.getOrElse(task.priority) { PriorityColors[0] }
    val hasSubs = subtasks.isNotEmpty()
    // The composer filter hid part of this card's children (the card only stayed on the
    // board because one of them matched): show a hint and lock child drag-reorder —
    // reordering against a partial list would write meaningless positions.
    val subsNarrowed = !nested && !compact && state.isSubtasksNarrowed(task.id)
    val childDrag = if (subsNarrowed) null else drag
    val onDropChild = if (subsNarrowed) null else onDropTask
    val shape = RoundedCornerShape(RadiusLg)
    // Subtask cards are a touch lighter than the parent — surface mixed 70/30
    // with the page background (mirrors the web's color-mix).
    val subtaskSurface = lerp(c.cardSurface, c.bg, 0.30f)
    // Adding a subtask is triggered from the "⋯" menu (no persistent button under
    // the card — that reclaims the empty space); this reveals the inline field.
    var addingSub by remember(task.id) { mutableStateOf(false) }
    // This card is itself a subtask sitting in another column than its parent.
    val divergedCol = if (nested) divergedColumn(task.columnId, parentColumnId, state.columns) else null

    Column(modifier.fillMaxWidth()) {
        // Parent draws on top (zIndex above any subtasks) so the subtask cards
        // can tuck under it.
        Column(
            Modifier.fillMaxWidth()
                // On the card BODY, not on the outer column: the body is the node
                // that carries the tap-to-open click, and it excludes the subtask
                // cascade below — a tag on the outer node would centre a spec's
                // tap somewhere in the children.
                .then(if (anchored) Modifier.testTag(TestTags.taskCard(task.id)) else Modifier)
                .zIndex((subtasks.size + 1).toFloat())
                .softShadow(shape)
                .clip(shape)
                .leftAccentFrame(
                    accent = if (task.priority > 0) accent else c.border,
                    surface = if (nested) subtaskSurface else c.cardSurface,
                    // Whole-card border tinted a very muted priority hue (12%, mirrors
                    // web); the left edge keeps the 3px priority accent bar above.
                    border = if (task.priority > 0) lerp(c.border, accent, 0.12f) else c.border,
                    barWidth = if (task.priority > 0) 3.dp else 1.dp,
                    topRadius = RadiusLg,
                    bottomRadius = RadiusLg,
                    gradient = task.priority > 0,
                )
                // Parent-card drag lives on the BODY only (not the whole card),
                // so it never overlaps the subtasks' own draggables below — two
                // long-press detectors in one spot would fight. Nested (subtask)
                // cards are dragged via their outer `subtaskDrag` modifier instead.
                .then(
                    // No card drag in the read-only archive scope.
                    if (!nested && drag != null && onDropTask != null && !state.archivedMode) {
                        Modifier.draggableCard(drag, task) { onDropTask(task) }
                    } else {
                        Modifier
                    },
                )
                // Tap anywhere on the card body opens the modal; the interactive
                // children (checkbox, pills) consume their own taps first. Drag
                // is long-press, so this tap-only handler doesn't clash with it.
                .clickableNoRipple { onOpen(task) }
                .padding(
                    start = 12.dp,
                    end = 11.dp,
                    // For a nested (cascade) card the overlap region is hidden
                    // under the parent, so add the overlap PLUS a real visible gap.
                    top = if (nested) RadiusLg * 2 + 8.dp else 10.dp,
                    bottom = if (nested) 12.dp else 10.dp,
                ),
        ) {
            CardHeader(
                task, state, vm, onOpen,
                showMenu = !compact,
                showAddSub = !nested && !compact,
                onAddSubtask = { addingSub = true },
                divergedCol = divergedCol,
            )
            if (onOpenConflict != null && conflictTaskIds.contains(task.id)) {
                Spacer(Modifier.height(6.dp))
                ConflictPill { onOpenConflict(task) }
            }
            Spacer(Modifier.height(8.dp))
            if (state.archivedMode) {
                // Read-only: show the pills but swallow their edit taps — tapping
                // anywhere on them just opens the (read) modal.
                Box {
                    PillsRow(task, state, vm)
                    Spacer(Modifier.matchParentSize().clickableNoRipple { onOpen(task) })
                }
            } else {
                PillsRow(task, state, vm)
            }
        }

        // Render the subtask area when there are subtasks OR this card is the
        // current attach target (so a childless card still shows where the
        // dragged task will land).
        if (hasSubs || nestSlot != null) {
            if (state.subtasksExpanded) {
                // Cascade: each subtask is its own card peeking from under the
                // one above it (parent → sub1 → sub2 …), like the web stack.
                subtasks.forEachIndexed { i, sub ->
                    // A faded card at the slot previews where the dragged card
                    // joins the cascade; siblings animate to make room.
                    if (nestSlot != null && nestSlot.second == sub.id) ExpandedSubtaskPreview(drag?.dragging, state, vm)
                    TaskCard(
                        task = sub,
                        state = state,
                        vm = vm,
                        onOpen = onOpen,
                        nested = true,
                        parentColumnId = task.columnId,
                        drag = childDrag,
                        onDropTask = onDropChild,
                        conflictTaskIds = conflictTaskIds,
                        onOpenConflict = onOpenConflict,
                        anchored = anchored,
                        modifier = Modifier.animatePlacement().zIndex((subtasks.size - i).toFloat()).overlapTop(RadiusLg * 2)
                            .subtaskDrag(childDrag, onDropChild, sub),
                    )
                }
                if (nestSlot != null && nestSlot.second == null) ExpandedSubtaskPreview(drag?.dragging, state, vm)
            } else {
                // Compact: a single shaded card emerging from under the parent,
                // holding the subtasks as rows.
                Column(
                    Modifier.fillMaxWidth()
                        .overlapTop(RadiusLg * 2)
                        .softShadow(shape)
                        .clip(shape)
                        .leftAccentFrame(
                            accent = c.border,
                            surface = subtaskSurface,
                            border = c.border,
                            barWidth = 1.dp,
                            topRadius = RadiusLg,
                            bottomRadius = RadiusLg,
                            gradient = false,
                        )
                        .padding(top = RadiusLg * 2),
                ) {
                    subtasks.forEach { sub ->
                        // A faded copy of the dragged row marks the landing slot.
                        if (nestSlot != null && nestSlot.second == sub.id) SubtaskPreview(drag?.dragging, vm, onOpen)
                        SubtaskRow(
                            sub, vm, onOpen,
                            divergedCol = divergedColumn(sub.columnId, task.columnId, state.columns),
                            modifier = Modifier.animatePlacement().subtaskDrag(childDrag, onDropChild, sub),
                        )
                    }
                    // Append slot (drop past the last sibling / onto the body).
                    if (nestSlot != null && nestSlot.second == null) SubtaskPreview(drag?.dragging, vm, onOpen)
                }
            }
            if (subsNarrowed) SubtasksNarrowedHint(subtasks.size, state.subtaskCount(task.id))
        }
        if (!nested && !compact && addingSub) {
            SubtaskCreateField(task, vm, onDone = { addingSub = false })
        }
    }
}

@Composable
private fun CardHeader(
    task: Task,
    state: BoardUiState,
    vm: BoardViewModel,
    onOpen: (Task) -> Unit,
    showMenu: Boolean,
    showAddSub: Boolean,
    onAddSubtask: () -> Unit,
    divergedCol: BoardColumn? = null,
) {
    val c = Tessera.colors
    var editing by remember(task.id) { mutableStateOf(false) }
    val archived = state.archivedMode
    // Meta row (#number / GitLab !iid) is gated by card density + per-field toggles.
    val showNumber = state.cardShows("number") && task.number != null
    val showGitlab = state.cardShows("gitlab") && task.gitlabIid != null

    // Completion toggle: leading on subtask/compact cards; on regular top-level
    // cards it moves into the quick-action group (next to add-subtask), web parity.
    val completeToggle = @Composable { sz: Dp ->
        // Plain checkmark (web parity), not a circle: muted when open, accent when done.
        IonIcon(
            Ion.CHECK,
            size = sz,
            tint = if (task.isCompleted) c.primary else c.text3,
            gradient = task.isCompleted,
            modifier = Modifier.clickableNoRipple { vm.toggleDone(task) },
        )
    }
    Column {
        Row(verticalAlignment = Alignment.CenterVertically) {
            if (!showAddSub && !archived) {
                completeToggle(19.dp)
                Spacer(Modifier.width(8.dp))
            }
            if (editing) {
                InlineTitleEditor(
                    initial = task.title,
                    onCommit = {
                        editing = false
                        if (it != task.title) vm.renameTask(task, it)
                    },
                    onCancel = { editing = false },
                    modifier = Modifier.weight(1f),
                )
            } else {
                Text(
                    task.title,
                    color = c.text1,
                    fontSize = 14.sp,
                    // Clamp to two lines so a long title never wraps one word per
                    // line around the number/GitLab badge (now on the row below).
                    maxLines = 2,
                    overflow = TextOverflow.Ellipsis,
                    textDecoration = if (task.isCompleted) TextDecoration.LineThrough else null,
                    // Tap opens the modal. No long-press-to-edit here: it would steal
                    // the long-press from the card's drag gesture and pop the keyboard
                    // mid-drag. Renaming is via the "⋯" menu / the modal.
                    modifier = Modifier.weight(1f).clickableNoRipple { onOpen(task) },
                )
            }
            // Quick-action group (web hover-action-bar parity; touch has no hover so
            // it's persistent, top-level cards only): complete + add-subtask + menu,
            // all uniform 24dp/16dp icon buttons with even 2dp spacing. Suppressed in
            // the read-only archive scope, which shows a restore/delete menu instead.
            if (showAddSub && !archived) {
                Spacer(Modifier.width(2.dp))
                IonIconButton(
                    Ion.CHECK,
                    onClick = { vm.toggleDone(task) },
                    boxSize = 24.dp,
                    iconSize = 16.dp,
                    tint = if (task.isCompleted) c.primary else c.text3,
                )
                Spacer(Modifier.width(2.dp))
                IonIconButton(Ion.GIT_BRANCH, onClick = onAddSubtask, boxSize = 24.dp, iconSize = 16.dp, tint = c.text3)
            }
            if (showMenu) {
                Spacer(Modifier.width(2.dp))
                if (archived) {
                    ArchiveCardMenu(task, vm, onOpen)
                } else {
                    CardMenu(task, vm, onOpen, onEditTitle = { editing = true }, onAddSubtask = onAddSubtask)
                }
            }
        }
        // Meta row: task number + GitLab issue link, aligned under the title — 27dp
        // in when a leading checkbox is present (19dp + 8dp gap), else flush left.
        if (showNumber || showGitlab || divergedCol != null) {
            Row(
                Modifier.padding(start = if (showAddSub) 0.dp else 27.dp, top = 6.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                if (showNumber) {
                    task.number?.let { Text("#$it", color = c.text3, fontSize = 11.sp) }
                }
                // Expanded subtask card whose column differs from its parent's.
                if (divergedCol != null) {
                    if (showNumber) Spacer(Modifier.width(6.dp))
                    Row(
                        Modifier.clip(RoundedCornerShape(RadiusSm))
                            .border(1.dp, c.border, RoundedCornerShape(RadiusSm))
                            .padding(horizontal = 5.dp, vertical = 1.dp),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        Box(
                            Modifier.size(6.dp).clip(CircleShape)
                                .background(accentGradient(parseHexColor(divergedCol.color, c.text3))),
                        )
                        Spacer(Modifier.width(4.dp))
                        Text(divergedCol.name, color = c.text2, fontSize = 10.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
                    }
                }
                if (showGitlab) {
                    task.gitlabIid?.let { iid ->
                        if (showNumber || divergedCol != null) Spacer(Modifier.width(6.dp))
                        val ctx = LocalContext.current
                        Row(
                            Modifier.clip(RoundedCornerShape(RadiusSm))
                                .border(1.dp, c.border, RoundedCornerShape(RadiusSm))
                                .clickableNoRipple {
                                    task.gitlabUrl?.let { url ->
                                        runCatching { ctx.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(url))) }
                                    }
                                }
                                .padding(horizontal = 5.dp, vertical = 1.dp),
                            verticalAlignment = Alignment.CenterVertically,
                        ) {
                            IonIcon(Ion.GITLAB, size = 10.dp, tint = c.text2)
                            Spacer(Modifier.width(2.dp))
                            Text("!$iid", color = c.text2, fontSize = 10.sp, fontWeight = FontWeight.SemiBold)
                        }
                    }
                }
            }
        }
    }
}

/** The card's "⋯" quick-actions menu (open / toggle done / rename / archive /
 *  delete). Archive + delete confirm via a popover before acting. */
@Composable
private fun CardMenu(
    task: Task,
    vm: BoardViewModel,
    onOpen: (Task) -> Unit,
    onEditTitle: () -> Unit,
    onAddSubtask: () -> Unit,
) {
    val c = Tessera.colors
    var menu by remember { mutableStateOf(false) }
    var confirmArchive by remember { mutableStateOf(false) }
    var confirmDelete by remember { mutableStateOf(false) }
    Box {
        IonIconButton(Ion.ELLIPSIS_V, onClick = { menu = true }, boxSize = 24.dp, iconSize = 16.dp, tint = c.text3)
        TDropdown(expanded = menu, onDismiss = { menu = false }) {
            TMenuItem("Открыть", icon = Ion.DOCUMENT_TEXT, onClick = {
                menu = false
                onOpen(task)
            })
            TMenuItem(
                if (task.isCompleted) "Вернуть в работу" else "Выполнено",
                icon = if (task.isCompleted) Ion.ELLIPSE else Ion.CHECK_CIRCLE,
                onClick = {
                    menu = false
                    vm.toggleDone(task)
                },
            )
            TMenuItem("Переименовать", icon = Ion.PENCIL, onClick = {
                menu = false
                onEditTitle()
            })
            TMenuItem("Создать подзадачу", icon = Ion.GIT_BRANCH, onClick = {
                menu = false
                onAddSubtask()
            })
            TMenuDivider()
            TMenuItem("В архив", icon = Ion.ARCHIVE, onClick = {
                menu = false
                confirmArchive = true
            })
            TMenuItem("Удалить", icon = Ion.TRASH, danger = true, onClick = {
                menu = false
                confirmDelete = true
            })
        }
        TConfirmPopover(
            expanded = confirmArchive,
            message = "Архивировать задачу «${task.title}»?",
            confirmText = "В архив",
            danger = false,
            onConfirm = {
                confirmArchive = false
                vm.archive(task.id)
            },
            onDismiss = { confirmArchive = false },
        )
        TConfirmPopover(
            expanded = confirmDelete,
            message = "Удалить задачу «${task.title}»? Это действие необратимо.",
            confirmText = "Удалить",
            onConfirm = {
                confirmDelete = false
                vm.delete(task.id)
            },
            onDismiss = { confirmDelete = false },
        )
    }
}

/** Read-only archive scope card menu: open (view), restore, or delete forever. */
@Composable
private fun ArchiveCardMenu(task: Task, vm: BoardViewModel, onOpen: (Task) -> Unit) {
    val c = Tessera.colors
    var menu by remember { mutableStateOf(false) }
    var confirmDelete by remember { mutableStateOf(false) }
    Box {
        IonIconButton(Ion.ELLIPSIS_V, onClick = { menu = true }, boxSize = 24.dp, iconSize = 16.dp, tint = c.text3)
        TDropdown(expanded = menu, onDismiss = { menu = false }) {
            TMenuItem("Открыть", icon = Ion.DOCUMENT_TEXT, onClick = {
                menu = false
                onOpen(task)
            })
            TMenuItem("Вернуть из архива", icon = Ion.ELLIPSE, onClick = {
                menu = false
                vm.restoreFromArchive(task.id)
            })
            TMenuDivider()
            TMenuItem("Удалить навсегда", icon = Ion.TRASH, danger = true, onClick = {
                menu = false
                confirmDelete = true
            })
        }
        TConfirmPopover(
            expanded = confirmDelete,
            message = "Удалить задачу «${task.title}» навсегда? Это действие необратимо.",
            confirmText = "Удалить",
            onConfirm = {
                confirmDelete = false
                vm.deleteFromArchive(task.id)
            },
            onDismiss = { confirmDelete = false },
        )
    }
}

/**
 * A borderless single-line title editor reused by cards and column headers:
 * auto-focuses, commits on Enter or focus-loss (after first focus), cancels on
 * blank. [fontSize]/[fontWeight] let the caller match the static title it stands
 * in for.
 */
@Composable
fun InlineTitleEditor(
    initial: String,
    onCommit: (String) -> Unit,
    onCancel: () -> Unit,
    modifier: Modifier = Modifier,
    fontSize: androidx.compose.ui.unit.TextUnit = 14.sp,
    fontWeight: FontWeight = FontWeight.Normal,
) {
    val c = Tessera.colors
    val focus = remember { FocusRequester() }
    var text by remember { mutableStateOf(initial) }
    var done by remember { mutableStateOf(false) }
    var hadFocus by remember { mutableStateOf(false) }
    fun finish() {
        if (done) return
        done = true
        val t = text.trim()
        if (t.isEmpty()) onCancel() else onCommit(t)
    }
    LaunchedEffect(Unit) { focus.requestFocus() }
    BasicTextField(
        value = text,
        onValueChange = { text = it },
        singleLine = true,
        textStyle = TextStyle(color = c.text1, fontSize = fontSize, fontWeight = fontWeight),
        cursorBrush = SolidColor(c.primary),
        keyboardOptions = KeyboardOptions(imeAction = ImeAction.Done),
        keyboardActions = KeyboardActions(onDone = { finish() }),
        modifier = modifier.focusRequester(focus).onFocusChanged {
            if (it.isFocused) hadFocus = true else if (hadFocus) finish()
        },
    )
}

@Composable
private fun PillsRow(task: Task, state: BoardUiState, vm: BoardViewModel) {
    // Compact density = title only, no pills (web cardSize=compact → SIZE_FIELDS=[]).
    if (state.isCompactCard) return

    val showEmpty = state.showEmpty
    val hasDue = !task.dueDate.isNullOrBlank()
    val hasTags = task.tagIds.isNotEmpty()
    val hasAssignee = task.assigneeIds.isNotEmpty() || task.gitlabAssignees.isNotEmpty() ||
        task.createdBy != null || task.gitlabAuthor != null

    // web: show(k) = sizeAllows(k) && fieldVis(k); the "always-on" fields
    // (priority/due/tags/assignee) additionally hide when empty && !showEmpty.
    val showPriority = state.cardShows("priority") && (showEmpty || task.priority > 0)
    val showDue = state.cardShows("due") && (showEmpty || hasDue)
    val showTags = state.cardShows("tags") && (showEmpty || hasTags)
    val showEstimate = state.cardShows("estimate") // pill self-hides when blank
    val showMilestone = state.cardShows("milestone") // pill self-hides when unset
    val showDescription = state.cardShows("description") && !state.stackFields
    val showAssignee = state.cardShows("assignee") && (showEmpty || hasAssignee)

    // Stacked layout (web stackFields): each field is an aligned "icon → value" row
    // with its OWN inline picker (same as the pill), a dash for empty, generous row
    // spacing for touch. Description is omitted in stack mode (web parity).
    if (state.stackFields) {
        Column(Modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(3.dp)) {
            if (showPriority) PriorityPill(task, vm, stacked = true)
            if (showDue) DuePill(task, state, vm, stacked = true)
            if (showEstimate) EstimatePill(task, state, stacked = true)
            if (showTags) TagsPill(task, state, vm, stacked = true)
            if (showMilestone) MilestonePill(task, state, stacked = true)
            if (showAssignee) AssigneesPill(task, state, vm, stacked = true)
        }
        return
    }

    // Inline: the field pills wrap (FlowRow) with the assignee pinned to the right.
    Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
        FlowRow(
            Modifier.weight(1f),
            horizontalArrangement = Arrangement.spacedBy(6.dp),
            verticalArrangement = Arrangement.spacedBy(6.dp),
        ) {
            if (showPriority) PriorityPill(task, vm)
            if (showDue) DuePill(task, state, vm)
            if (showEstimate) EstimatePill(task, state)
            if (showDescription) DescriptionPill(task, state)
            if (showTags) TagsPill(task, state, vm)
            if (showMilestone) MilestonePill(task, state)
        }
        if (showAssignee) {
            Spacer(Modifier.width(6.dp))
            AssigneesPill(task, state, vm)
        }
    }
}

/** A stacked-mode field row: the icon in a fixed leading column + the value, so
 *  every field's value aligns to the same vertical line (web `.pills.stacked`). */
@Composable
private fun StackField(
    icon: String,
    iconTint: Color,
    gradient: Boolean = false,
    onClick: (() -> Unit)? = null,
    value: @Composable RowScope.() -> Unit,
) {
    Row(
        Modifier.fillMaxWidth()
            .clip(RoundedCornerShape(RadiusSm))
            .then(if (onClick != null) Modifier.clickableNoRipple(onClick = onClick) else Modifier)
            .padding(vertical = 6.dp, horizontal = 3.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Box(Modifier.width(22.dp), contentAlignment = Alignment.CenterStart) {
            IonIcon(icon, size = 14.dp, tint = iconTint, gradient = gradient)
        }
        value()
    }
}

/** A stacked-field value, or a muted dash when the field is empty. */
@Composable
private fun StackValue(text: String, tint: Color = Tessera.colors.text2) {
    val c = Tessera.colors
    Text(
        text.ifBlank { "—" },
        color = if (text.isBlank()) c.text3 else tint,
        fontSize = 12.sp,
        maxLines = 1,
        overflow = TextOverflow.Ellipsis,
    )
}

/** Amber «Конфликт» pill for a task with an unresolved GitLab write-back conflict;
 *  tapping opens the resolver focused on this task (web parity). */
@Composable
private fun ConflictPill(onClick: () -> Unit) {
    val shape = RoundedCornerShape(RadiusSm)
    Row(
        Modifier.clip(shape)
            .background(ConflictAmber.copy(alpha = 0.14f))
            .border(1.dp, ConflictAmber.copy(alpha = 0.55f), shape)
            .clickableNoRipple(onClick = onClick)
            .padding(horizontal = 8.dp, vertical = 3.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        IonIcon(Ion.GIT_NETWORK, size = 13.dp, tint = ConflictAmber)
        Spacer(Modifier.width(5.dp))
        Text("Конфликт", color = ConflictAmber, fontSize = 11.sp, fontWeight = FontWeight.Medium)
    }
}

/** «N из M подзадач» — footnote under a filter-narrowed child list, telling you the card
 *  is only here because a child matched and the rest are hidden (web parity). */
@Composable
private fun SubtasksNarrowedHint(shown: Int, total: Int) {
    val c = Tessera.colors
    Text(
        "$shown из $total подзадач — остальные скрыты фильтром",
        color = c.text3,
        fontSize = 10.sp,
        modifier = Modifier.padding(start = 12.dp, top = 4.dp, end = 8.dp),
    )
}

/** Display-only milestone («Этап») chip: a flag + the milestone title, dimmed when
 *  the milestone is closed. Only shown when the task is assigned one (web parity;
 *  assigning/clearing happens in the task modal). */
@Composable
private fun MilestonePill(task: Task, state: BoardUiState, stacked: Boolean = false) {
    val c = Tessera.colors
    val ms = task.milestoneId?.let { state.milestonesMap[it] }
    if (stacked) {
        StackField(Ion.ROCKET, c.text2) {
            StackValue(ms?.title ?: "", tint = if (ms?.isClosed == true) c.text3 else c.text2)
        }
        return
    }
    if (ms == null) return
    Box(Modifier.alpha(if (ms.isClosed) 0.6f else 1f)) {
        Pill(onClick = {}, set = true) {
            IonIcon(Ion.ROCKET, size = 13.dp, tint = c.text2)
            Spacer(Modifier.width(4.dp))
            Text(
                ms.title,
                color = c.text2,
                fontSize = 11.sp,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.widthIn(max = 96.dp),
            )
        }
    }
}

/** Display-only estimate chip: the task's own estimate, or — when unset — the
 *  rollup sum of its subtasks ("Σ …"). Editing happens in the task modal. */
@Composable
private fun EstimatePill(task: Task, state: BoardUiState, stacked: Boolean = false) {
    val c = Tessera.colors
    val own = task.estimate
    val rollup = website.msdnna.tessera.util.Estimation.sum(
        state.subtasks.filter { it.parentId == task.id }.map { it.estimate },
    )
    val value = own ?: rollup
    val text = website.msdnna.tessera.util.Estimation.format(value, state.estimation)
    val isRollup = own == null && rollup != null
    if (stacked) {
        StackField(Ion.TIME, c.text2) { StackValue(if (text.isBlank()) "" else (if (isRollup) "Σ " else "") + text) }
        return
    }
    if (text.isBlank()) return
    Pill(onClick = {}, set = true) {
        IonIcon(Ion.TIME, size = 13.dp, tint = c.text2)
        Spacer(Modifier.width(4.dp))
        Text((if (isRollup) "Σ " else "") + text, color = c.text2, fontSize = 11.sp)
    }
}

/** Display-only description indicator: shown only when the task has a description;
 *  tapping opens a popover with the rendered markdown (web hover-preview parity —
 *  touch has no hover, so it's a tap). */
@Composable
private fun DescriptionPill(task: Task, state: BoardUiState) {
    val c = Tessera.colors
    if (task.description.isBlank()) return
    var open by remember { mutableStateOf(false) }
    Box {
        Pill(onClick = { open = true }, set = true) {
            IonIcon(Ion.MENU, size = 13.dp, tint = c.text2)
        }
        TDropdown(expanded = open, onDismiss = { open = false }, scrollable = true) {
            Box(Modifier.width(280.dp).heightIn(max = 320.dp).padding(horizontal = 12.dp, vertical = 8.dp)) {
                RichContent(source = task.description, mentions = buildMentionItems(state.members, state.gitlabMembers))
            }
        }
    }
}

@Composable
private fun PriorityPill(task: Task, vm: BoardViewModel, stacked: Boolean = false) {
    val c = Tessera.colors
    var menu by remember { mutableStateOf(false) }
    val on = task.priority > 0
    val color = if (on) PriorityColors[task.priority] else c.text3
    Box {
        if (stacked) {
            StackField(Ion.FLAG, color, gradient = on, onClick = { menu = true }) {
                StackValue(if (on) PriorityLabels[task.priority] else "")
            }
        } else {
            // Icon-only flag, tinted by priority (no text) — like the web.
            Pill(onClick = { menu = true }, set = on) {
                IonIcon(Ion.FLAG, size = 13.dp, tint = color, gradient = on)
            }
        }
        TDropdown(expanded = menu, onDismiss = { menu = false }) {
            PriorityLabels.forEachIndexed { i, label ->
                Row(
                    Modifier.fillMaxWidth().clickableNoRipple {
                        menu = false
                        vm.setPriority(task, i)
                    }
                        .padding(horizontal = 14.dp, vertical = 10.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Box(Modifier.size(8.dp).clip(CircleShape).background(accentGradient(PriorityColors[i])))
                    Spacer(Modifier.width(10.dp))
                    Text(label, color = c.text1, fontSize = 14.sp)
                }
            }
        }
    }
}

@Composable
private fun TagsPill(task: Task, state: BoardUiState, vm: BoardViewModel, stacked: Boolean = false) {
    val c = Tessera.colors
    var menu by remember { mutableStateOf(false) }
    val taskTags = task.tagIds.mapNotNull { state.tags[it] }
    Box {
        if (stacked) {
            // Icon → coloured chips (as many as fit) + "+N" for the overflow (web parity).
            StackField(Ion.PRICETAG, c.text2, onClick = { menu = true }) {
                if (taskTags.isEmpty()) {
                    StackValue("")
                } else {
                    TagChipsFit(taskTags, Modifier.fillMaxWidth(), state.prefixNames)
                }
            }
        } else if (taskTags.isEmpty()) {
            Pill(onClick = { menu = true }) { IonIcon(Ion.PRICETAG, size = 13.dp, tint = c.text3) }
        } else {
            val first = taskTags.first()
            val base = parseHexColor(first.color, c.text3)
            // Clamp the tag colour into a legible band for the active theme (web
            // parity) so the gradient text stays readable on either background.
            val tagText = readableHue(base, c.isDark)
            // Behind layers are OPAQUE (surface blended with each tag's colour)
            // so neither the front pill nor the layers bleed through each other.
            val extra = taskTags.drop(1).take(2).map { lerp(c.cardSurface, parseHexColor(it.color, c.text3), 0.35f) }
            val pillShape = RoundedCornerShape(RadiusSm)
            val frontBg = lerp(c.cardSurface, base, 0.18f)
            val stack = if (extra.isNotEmpty()) Modifier.padding(end = (extra.size * 5).dp) else Modifier
            if (tagParts(first.name, state.prefixNames, Tessera.rawTagPrefix).hasScope) {
                // Scoped tag: the pill's box belongs to the two-segment [TagChip], so
                // this row keeps only the stack cascade (moved onto the chip) and the
                // "+N", which steps right past the peeking layers (web parity).
                Row(
                    Modifier.clickableNoRipple { menu = true },
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    TagChip(
                        first.name,
                        first.color,
                        modifier = stack.stackedTagShadow(extra, RadiusSm),
                        prefixNames = state.prefixNames,
                    )
                    if (taskTags.size > 1) {
                        Spacer(Modifier.width(4.dp))
                        Text("+${taskTags.size - 1}", fontSize = 10.sp, style = TextStyle(brush = accentGradient(tagText.copy(alpha = 0.85f))))
                    }
                }
            } else {
                Row(
                    Modifier
                        .height(TagPillHeight)
                        .then(stack)
                        .stackedTagShadow(extra, RadiusSm)
                        .clip(pillShape)
                        .background(frontBg)
                        .border(1.dp, base.copy(alpha = 0.45f), pillShape)
                        .clickableNoRipple { menu = true }
                        .padding(horizontal = 9.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Text(first.name, fontSize = 11.sp, fontWeight = FontWeight.Medium, style = TextStyle(brush = accentGradient(tagText)))
                    if (taskTags.size > 1) {
                        Spacer(Modifier.width(4.dp))
                        Text("+${taskTags.size - 1}", fontSize = 10.sp, style = TextStyle(brush = accentGradient(tagText.copy(alpha = 0.85f))))
                    }
                }
            }
        }
        TDropdown(expanded = menu, onDismiss = { menu = false }, scrollable = true) {
            // Group the chips by tag prefix; headers only with >1 group (web parity).
            val groups = buildTagGroups(state.tagList, state.prefixNames)
            val headers = groups.size > 1
            groups.forEach { g ->
                if (headers) {
                    Text(
                        g.label.uppercase(),
                        color = c.text3,
                        fontSize = 10.sp,
                        fontWeight = FontWeight.SemiBold,
                        letterSpacing = 0.4.sp,
                        modifier = Modifier.padding(start = 10.dp, end = 10.dp, top = 8.dp, bottom = 2.dp),
                    )
                }
                FlowRow(
                    Modifier.width(240.dp).padding(horizontal = 8.dp, vertical = 4.dp),
                    horizontalArrangement = Arrangement.spacedBy(6.dp),
                    verticalArrangement = Arrangement.spacedBy(6.dp),
                ) {
                    g.tags.forEach { tag ->
                        val on = tag.id in task.tagIds
                        val base = parseHexColor(tag.color, c.text3)
                        Box(
                            Modifier.clip(RoundedCornerShape(10.dp))
                                .background(accentGradient(if (on) base else base.copy(alpha = 0.14f)))
                                .clickableNoRipple { vm.toggleTag(task, tag.id) }
                                .padding(horizontal = 9.dp, vertical = 3.dp),
                        ) {
                            // The scope is already the section header when grouping is
                            // visible — repeating it in every chip is just noise.
                            TagLabel(
                                tag.name,
                                color = if (on) onColor(base) else readableHue(base, c.isDark),
                                prefixNames = state.prefixNames,
                                showScope = !headers,
                            )
                        }
                    }
                }
            }
            Box(Modifier.padding(horizontal = 8.dp, vertical = 4.dp).width(240.dp)) {
                // autoFocus=false so opening the tag picker doesn't pop the keyboard;
                // the user taps the field when they actually want to add a tag.
                InlineCreateField(
                    placeholder = "Новый тег, Enter",
                    autoFocus = false,
                    onCommit = {
                        vm.createTagAndAdd(task, it)
                        menu = false
                    },
                    onDismiss = { },
                )
            }
        }
    }
}

@Composable
private fun DuePill(task: Task, state: BoardUiState, vm: BoardViewModel, stacked: Boolean = false) {
    val c = Tessera.colors
    var picker by remember { mutableStateOf(false) }
    val due = dueShort(task.dueDate)
    // Overdue (past due, not done) → red tint, like the web.
    val overdue = !task.isCompleted && isOverdue(task.dueDate)
    val overdueColor = Color(0xFFE0533D)
    val tint = when {
        overdue -> overdueColor
        due.isNotBlank() -> c.text2
        else -> c.text3
    }
    if (stacked) {
        // Capitalised value (web stack), no pill; opens the same date picker.
        StackField(Ion.CALENDAR, tint, onClick = { picker = true }) {
            StackValue(due.replaceFirstChar { it.uppercaseChar() }, tint = tint)
            if (task.recurrence != null) {
                Spacer(Modifier.width(4.dp))
                IonIcon(Ion.REPEAT, size = 11.dp, tint = tint)
            }
        }
    } else {
        Pill(onClick = { picker = true }, set = due.isNotBlank()) {
            IonIcon(Ion.CALENDAR, size = 13.dp, tint = tint)
            if (due.isNotBlank()) {
                Spacer(Modifier.width(4.dp))
                Text(due, color = tint, fontSize = 11.sp)
            }
            if (task.recurrence != null) {
                Spacer(Modifier.width(4.dp))
                // Recur glyph inherits the pill's text colour (web 0.113.2) — the purple
                // accent clashed on the dark theme.
                IonIcon(Ion.REPEAT, size = 11.dp, tint = tint)
            }
        }
    }
    if (picker) {
        DueDateTimePicker(
            initialIso = task.dueDate,
            initialStartIso = task.startDate,
            initialRecurrence = task.recurrence,
            columns = state.sortedColumns,
            notifyEnabled = task.dueNotifyEnabled,
            notifyLead = task.dueLeadMinutes,
            notifyRepeat = task.dueRepeatMinutes,
            onApply = { iso, start, rec -> vm.setDueAndRecurrence(task, iso, start, rec) },
            onNotify = { lead, repeat, enabled -> vm.setDueNotify(task, lead, repeat, enabled) },
            onDismiss = { picker = false },
        )
    }
}

@Composable
private fun AssigneesPill(task: Task, state: BoardUiState, vm: BoardViewModel, stacked: Boolean = false) {
    val c = Tessera.colors
    var menu by remember { mutableStateOf(false) }
    val assignees = task.assigneeIds.mapNotNull { state.membersMap[it] }
    val external = task.gitlabAssignees
    // Author (read-only): the GitLab issue author for synced cards, else the
    // Tessera creator resolved from createdBy.
    val authorName: String? = when {
        task.gitlabAuthor != null -> task.gitlabAuthorName?.takeIf { it.isNotBlank() } ?: task.gitlabAuthor
        task.createdBy != null -> state.membersMap[task.createdBy]?.name
        else -> null
    }
    // When the author is also an assignee, don't render a separate (muted) author
    // avatar — the person already shows once as the accent assignee. Mirrors web.
    val authorIsAssignee = when {
        task.gitlabAuthor != null ->
            external.any { it.equals(authorName, true) || it.equals(task.gitlabAuthor, true) }

        task.createdBy != null -> task.createdBy in task.assigneeIds

        else -> false
    }
    val showAuthor = authorName != null && !authorIsAssignee
    val isEmpty = authorName == null && assignees.isEmpty() && external.isEmpty()
    // Author + assignees merged into one overlapping stack (card-coloured ring =
    // the "cutout"): the muted author leads, then accent assignees.
    val avatars: @Composable () -> Unit = {
        Row(horizontalArrangement = Arrangement.spacedBy((-8).dp)) {
            if (showAuthor) {
                CardAvatar(
                    authorName!!,
                    muted = true,
                    userId = if (task.gitlabAuthor == null) task.createdBy else null,
                    avatarUrl = if (task.gitlabAuthor != null) task.gitlabAuthorAvatarUrl else null,
                )
            }
            assignees.forEach { CardAvatar(it.name, muted = false, userId = it.userId) }
            external.forEach { CardAvatar(it, muted = true) }
        }
    }
    Box {
        if (stacked) {
            // Icon → avatars (or a dash when unassigned); opens the same picker.
            StackField(Ion.PEOPLE, c.text3, onClick = { menu = true }) {
                if (isEmpty) StackValue("") else avatars()
            }
        } else {
            Box(Modifier.clip(RoundedCornerShape(RadiusSm)).clickableNoRipple { menu = true }.padding(2.dp)) {
                if (isEmpty) IonIcon(Ion.PERSON_ADD, size = 14.dp, tint = c.text3) else avatars()
            }
        }
        var query by remember { mutableStateOf("") }
        LaunchedEffect(menu) { if (!menu) query = "" }
        val recent by AppContainer.prefs.recentAssignees.collectAsState(initial = emptyList())
        // No query: assigned → recently-picked (MRU) → alphabetical, deduped and
        // capped at 10 (never hiding a current assignee). With a query: name filter.
        // Mirrors web TaskCard pickerMembers.
        val pickerMembers = remember(query, recent, state.members, task.assigneeIds) {
            val q = query.trim().lowercase()
            if (q.isNotBlank()) {
                state.members.filter { it.name.lowercase().contains(q) }
            } else {
                val byId = state.members.associateBy { it.userId }
                val out = LinkedHashMap<String, website.msdnna.tessera.data.model.Member>()
                (task.assigneeIds + recent + state.members.sortedBy { it.name.lowercase() }.map { it.userId })
                    .forEach { id -> byId[id]?.let { out.putIfAbsent(id, it) } }
                out.values.toList().take(maxOf(10, task.assigneeIds.size))
            }
        }
        TDropdown(expanded = menu, onDismiss = { menu = false }, scrollable = true) {
            TTextField(
                query, { query = it }, placeholder = "Поиск…",
                modifier = Modifier.fillMaxWidth().padding(horizontal = 10.dp, vertical = 6.dp),
            )
            pickerMembers.forEach { m ->
                val on = m.userId in task.assigneeIds
                Row(
                    Modifier.fillMaxWidth().clickableNoRipple { vm.toggleAssignee(task, m.userId) }
                        .padding(horizontal = 12.dp, vertical = 8.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Box(Modifier.size(22.dp).clip(CircleShape).background(accentGradient(c.primary)), contentAlignment = Alignment.Center) {
                        Text(initials(m.name), color = c.onPrimary, fontSize = 10.sp)
                    }
                    Spacer(Modifier.width(8.dp))
                    Text(m.name, color = c.text1, fontSize = 14.sp, modifier = Modifier.weight(1f))
                    if (on) {
                        Spacer(Modifier.width(8.dp))
                        IonIcon(Ion.CHECK, size = 16.dp, tint = c.primary, gradient = true)
                    }
                }
            }
            if (state.gitlabMembers.isNotEmpty()) {
                Text(
                    "GitLab",
                    color = c.text3, fontSize = 10.sp,
                    modifier = Modifier.padding(start = 12.dp, top = 8.dp, bottom = 2.dp),
                )
                state.gitlabMembers.forEach { m ->
                    val on = m.glUsername in task.gitlabAssigneeLogins
                    val label = m.glName.ifBlank { m.glUsername }
                    Row(
                        Modifier.fillMaxWidth().clickableNoRipple { vm.toggleGitlabAssignee(task, m) }
                            .padding(horizontal = 12.dp, vertical = 8.dp),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        CardAvatar(label, muted = true, avatarUrl = m.glAvatarUrl)
                        Spacer(Modifier.width(8.dp))
                        Text(label, color = c.text1, fontSize = 14.sp, modifier = Modifier.weight(1f))
                        if (on) {
                            Spacer(Modifier.width(8.dp))
                            IonIcon(Ion.CHECK, size = 16.dp, tint = c.primary, gradient = true)
                        }
                    }
                }
            }
        }
    }
}

/** A faded copy of the dragged subtask, shown at the drop slot in a parent's list. */
@Composable
private fun SubtaskPreview(sub: Task?, vm: BoardViewModel, onOpen: (Task) -> Unit) {
    if (sub == null) return
    SubtaskRow(sub, vm, onOpen, modifier = Modifier.alpha(0.45f))
}

/** A faded cascade card at the drop slot (expanded subtasks). */
@Composable
private fun ExpandedSubtaskPreview(sub: Task?, state: BoardUiState, vm: BoardViewModel) {
    if (sub == null) return
    TaskCard(
        task = sub,
        state = state,
        vm = vm,
        onOpen = {},
        nested = true,
        modifier = Modifier.alpha(0.45f).overlapTop(RadiusLg * 2),
    )
}

@Composable
private fun SubtaskRow(
    sub: Task,
    vm: BoardViewModel,
    onOpen: (Task) -> Unit,
    divergedCol: BoardColumn? = null,
    modifier: Modifier = Modifier,
) {
    val c = Tessera.colors
    Row(
        modifier.fillMaxWidth().clickableNoRipple { onOpen(sub) }.padding(horizontal = 10.dp, vertical = 7.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        IonIcon(
            if (sub.isCompleted) Ion.CHECK_CIRCLE else Ion.ELLIPSE,
            gradient = sub.isCompleted,
            size = 15.dp,
            tint = if (sub.isCompleted) c.primary else c.text3,
            modifier = Modifier.clip(CircleShape).clickableNoRipple { vm.toggleDone(sub) },
        )
        Spacer(Modifier.width(7.dp))
        if (sub.priority > 0) {
            Box(Modifier.size(7.dp).clip(CircleShape).background(accentGradient(PriorityColors[sub.priority])))
            Spacer(Modifier.width(6.dp))
        }
        Text(
            sub.title,
            color = c.text2,
            fontSize = 13.sp,
            maxLines = 1,
            textDecoration = if (sub.isCompleted) TextDecoration.LineThrough else null,
            modifier = Modifier.weight(1f),
        )
        val due = shortDate(sub.dueDate)
        if (due.isNotBlank()) Text(due, color = c.text3, fontSize = 11.sp)
        // This child ran ahead of (or behind) its parent — mark it with the
        // column's own colour. Just the marker: a row this narrow has no space
        // for a name, and the modal spells it out.
        if (divergedCol != null) {
            Spacer(Modifier.width(6.dp))
            Box(
                Modifier.size(7.dp).clip(RoundedCornerShape(2.dp))
                    .background(accentGradient(parseHexColor(divergedCol.color, c.text3))),
            )
        }
    }
}

/** The inline "new subtask" title field, revealed by the card's "⋯" → «Создать
 *  подзадачу» (no persistent button under the card — web parity). */
@Composable
private fun SubtaskCreateField(task: Task, vm: BoardViewModel, onDone: () -> Unit) {
    Box(Modifier.fillMaxWidth().padding(top = 6.dp)) {
        InlineCreateField(
            placeholder = "Название подзадачи, Enter",
            onCommit = {
                vm.createTask(task.columnId, it, parentId = task.id)
                onDone()
            },
            onDismiss = onDone,
        )
    }
}

@Composable
private fun Pill(onClick: () -> Unit, set: Boolean = false, content: @Composable () -> Unit) {
    val c = Tessera.colors
    val shape = RoundedCornerShape(RadiusSm)
    Row(
        Modifier
            .height(24.dp)
            .clip(shape)
            // Unset quick-action pills get a dashed outline (like the web); a set
            // pill gets a solid 1px border.
            .then(if (set) Modifier.border(1.dp, c.border, shape) else Modifier.dashedBorder(c.border, RadiusSm))
            .clickableNoRipple(onClick = onClick)
            .padding(horizontal = 7.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) { content() }
}

/**
 * Avatar initials: two words → first letter of each ("Василий Соколов" → ВС);
 * a dot handle → each part ("a.fokin" → AF); a single word → its first two
 * letters ("msdnna" → MS). Mirrors the web `utils/initials`.
 */
private fun initials(name: String): String {
    val s = name.trim()
    if (s.isEmpty()) return "?"
    if (s.contains('.')) {
        val parts = s.split('.').map { it.trim() }.filter { it.isNotEmpty() }
        if (parts.size >= 2) return "${parts[0].first()}${parts[1].first()}".uppercase()
    }
    val words = s.split(Regex("\\s+")).filter { it.isNotEmpty() }
    if (words.size >= 2) return "${words[0].first()}${words[1].first()}".uppercase()
    return s.take(2).uppercase()
}

/** A card avatar circle (24dp ring + inner gradient/grey disc). Shows the user's
 *  uploaded avatar when [userId] (Tessera) or [avatarUrl] (GitLab) is given,
 *  falling back to initials. [muted] greys the fallback for the read-only author
 *  and external GitLab assignees. */
@Composable
private fun CardAvatar(name: String, muted: Boolean, userId: String? = null, avatarUrl: String? = null) {
    val c = Tessera.colors
    val url = avatarUrl?.takeIf { it.isNotBlank() }
        ?: userId?.takeIf { it.isNotBlank() }?.let { "${RetrofitClient.serverRoot}/api/users/$it/avatar" }
    Box(
        Modifier.size(24.dp).clip(CircleShape).background(c.cardSurface).padding(1.5.dp),
        contentAlignment = Alignment.Center,
    ) {
        Box(
            Modifier.size(21.dp).clip(CircleShape)
                .then(if (muted) Modifier.background(c.text3) else Modifier.background(accentGradient(c.primary))),
            contentAlignment = Alignment.Center,
        ) {
            // Initials are the base layer; the image overlays them and shows
            // through when it fails to load (e.g. a GitLab avatar the phone
            // can't reach) instead of leaving an empty circle.
            Text(
                initials(name),
                color = if (muted) Color.White else c.onPrimary,
                fontSize = 9.sp,
                fontWeight = FontWeight.SemiBold,
            )
            if (url != null) {
                AsyncImage(model = url, contentDescription = null, modifier = Modifier.size(21.dp).clip(CircleShape))
            }
        }
    }
}

/**
 * Makes a subtask independently draggable (so the gesture lifts the subtask, not
 * its parent card) + dims it while dragged. No-op when the board isn't in a
 * drag-capable view (list/calendar pass no [drag]).
 */
private fun Modifier.subtaskDrag(drag: BoardDragState?, onDropTask: ((Task) -> Unit)?, sub: Task): Modifier =
    if (drag != null && onDropTask != null) {
        this.dragCollapse(drag.dragging?.id == sub.id).draggableCard(drag, sub) { onDropTask(sub) }
    } else {
        this
    }

/**
 * Lifts a composable up by [overlap] and shrinks its reported height by the
 * same amount, so it visually tucks under the preceding sibling (which should
 * sit above it via `zIndex`) without leaving a gap below — the Compose
 * equivalent of a negative top margin.
 */
private fun Modifier.overlapTop(overlap: Dp): Modifier = layout { measurable, constraints ->
    val placeable = measurable.measure(constraints)
    val o = overlap.roundToPx()
    layout(placeable.width, (placeable.height - o).coerceAtLeast(0)) {
        placeable.place(0, -o)
    }
}
