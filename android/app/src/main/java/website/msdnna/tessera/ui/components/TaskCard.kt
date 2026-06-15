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
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
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
import androidx.compose.ui.layout.layout
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.zIndex
import coil.compose.AsyncImage
import website.msdnna.tessera.data.api.RetrofitClient
import website.msdnna.tessera.data.model.Task
import website.msdnna.tessera.ui.theme.PriorityColors
import website.msdnna.tessera.ui.theme.PriorityLabels
import website.msdnna.tessera.ui.theme.RadiusLg
import website.msdnna.tessera.ui.theme.RadiusSm
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.accentGradient
import website.msdnna.tessera.ui.viewmodels.BoardUiState
import website.msdnna.tessera.ui.viewmodels.BoardViewModel
import website.msdnna.tessera.util.Ion
import website.msdnna.tessera.util.isOverdue
import website.msdnna.tessera.util.parseHexColor
import website.msdnna.tessera.util.shortDate

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
    compact: Boolean = false,
    drag: BoardDragState? = null,
    onDropTask: ((Task) -> Unit)? = null,
    nestSlot: Pair<String?, String?>? = null,
) {
    val c = Tessera.colors
    // Keep ALL subtasks composed during a drag (removing the dragged one would
    // dispose its gesture node and cancel the drag); the dragged one just dims.
    val subtasks = if (nested || compact) emptyList() else state.subtasksOf(task.id)
    val accent = PriorityColors.getOrElse(task.priority) { PriorityColors[0] }
    val hasSubs = subtasks.isNotEmpty()
    val shape = RoundedCornerShape(RadiusLg)
    // Subtask cards are a touch lighter than the parent — surface mixed 70/30
    // with the page background (mirrors the web's color-mix).
    val subtaskSurface = lerp(c.cardSurface, c.bg, 0.30f)

    Column(modifier.fillMaxWidth()) {
        // Parent draws on top (zIndex above any subtasks) so the subtask cards
        // can tuck under it.
        Column(
            Modifier.fillMaxWidth()
                .zIndex((subtasks.size + 1).toFloat())
                .softShadow(shape)
                .clip(shape)
                .leftAccentFrame(
                    accent = if (task.priority > 0) accent else c.border,
                    surface = if (nested) subtaskSurface else c.cardSurface,
                    border = c.border,
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
                    if (!nested && drag != null && onDropTask != null) {
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
            CardHeader(task, vm, onOpen, showMenu = !compact)
            Spacer(Modifier.height(8.dp))
            PillsRow(task, state, vm)
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
                        drag = drag,
                        onDropTask = onDropTask,
                        modifier = Modifier.animatePlacement().zIndex((subtasks.size - i).toFloat()).overlapTop(RadiusLg * 2)
                            .subtaskDrag(drag, onDropTask, sub),
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
                        SubtaskRow(sub, vm, onOpen, modifier = Modifier.animatePlacement().subtaskDrag(drag, onDropTask, sub))
                    }
                    // Append slot (drop past the last sibling / onto the body).
                    if (nestSlot != null && nestSlot.second == null) SubtaskPreview(drag?.dragging, vm, onOpen)
                }
            }
        }
        if (!nested && !compact) AddSubtaskRow(task, vm)
    }
}

@Composable
private fun CardHeader(task: Task, vm: BoardViewModel, onOpen: (Task) -> Unit, showMenu: Boolean) {
    val c = Tessera.colors
    var editing by remember(task.id) { mutableStateOf(false) }

    Row(verticalAlignment = Alignment.CenterVertically) {
        IonIcon(
            if (task.isCompleted) Ion.CHECK_CIRCLE else Ion.ELLIPSE,
            size = 19.dp,
            tint = if (task.isCompleted) c.primary else c.text3,
            gradient = task.isCompleted,
            modifier = Modifier.clip(CircleShape).clickableNoRipple { vm.toggleDone(task) },
        )
        Spacer(Modifier.width(8.dp))
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
                textDecoration = if (task.isCompleted) TextDecoration.LineThrough else null,
                // Tap opens the modal. No long-press-to-edit here: it would steal
                // the long-press from the card's drag gesture and pop the keyboard
                // mid-drag. Renaming is via the "⋯" menu / the modal.
                modifier = Modifier.weight(1f).clickableNoRipple { onOpen(task) },
            )
        }
        task.number?.let {
            Spacer(Modifier.width(6.dp))
            Text("#$it", color = c.text3, fontSize = 11.sp)
        }
        task.gitlabIid?.let { iid ->
            Spacer(Modifier.width(6.dp))
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
        if (showMenu) {
            Spacer(Modifier.width(2.dp))
            CardMenu(task, vm, onOpen, onEditTitle = { editing = true })
        }
    }
}

/** The card's "⋯" quick-actions menu (open / toggle done / rename / archive /
 *  delete). Archive + delete confirm via a popover before acting. */
@Composable
private fun CardMenu(task: Task, vm: BoardViewModel, onOpen: (Task) -> Unit, onEditTitle: () -> Unit) {
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
    Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
        PriorityPill(task, vm)
        Spacer(Modifier.width(6.dp))
        DuePill(task, vm)
        Spacer(Modifier.width(6.dp))
        TagsPill(task, state, vm)
        Spacer(Modifier.weight(1f))
        AssigneesPill(task, state, vm)
    }
}

@Composable
private fun PriorityPill(task: Task, vm: BoardViewModel) {
    val c = Tessera.colors
    var menu by remember { mutableStateOf(false) }
    val color = if (task.priority > 0) PriorityColors[task.priority] else c.text3
    Box {
        // Icon-only flag, tinted by priority (no text) — like the web.
        Pill(onClick = { menu = true }, set = task.priority > 0) {
            IonIcon(Ion.FLAG, size = 13.dp, tint = color, gradient = task.priority > 0)
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
private fun TagsPill(task: Task, state: BoardUiState, vm: BoardViewModel) {
    val c = Tessera.colors
    var menu by remember { mutableStateOf(false) }
    val taskTags = task.tagIds.mapNotNull { state.tags[it] }
    Box {
        if (taskTags.isEmpty()) {
            Pill(onClick = { menu = true }) { IonIcon(Ion.PRICETAG, size = 13.dp, tint = c.text3) }
        } else {
            val first = taskTags.first()
            val base = parseHexColor(first.color, c.text3)
            // Behind layers are OPAQUE (surface blended with each tag's colour)
            // so neither the front pill nor the layers bleed through each other.
            val extra = taskTags.drop(1).take(2).map { lerp(c.cardSurface, parseHexColor(it.color, c.text3), 0.35f) }
            val pillShape = RoundedCornerShape(RadiusSm)
            val frontBg = lerp(c.cardSurface, base, 0.18f)
            Row(
                Modifier
                    .then(if (extra.isNotEmpty()) Modifier.padding(end = (extra.size * 5).dp) else Modifier)
                    .stackedTagShadow(extra, RadiusSm)
                    .clip(pillShape)
                    .background(frontBg)
                    .border(1.dp, base.copy(alpha = 0.45f), pillShape)
                    .clickableNoRipple { menu = true }
                    .padding(horizontal = 9.dp, vertical = 5.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(first.name, fontSize = 11.sp, fontWeight = FontWeight.Medium, style = TextStyle(brush = accentGradient(base)))
                if (taskTags.size > 1) {
                    Spacer(Modifier.width(4.dp))
                    Text("+${taskTags.size - 1}", fontSize = 10.sp, style = TextStyle(brush = accentGradient(base.copy(alpha = 0.85f))))
                }
            }
        }
        TDropdown(expanded = menu, onDismiss = { menu = false }) {
            FlowRow(
                Modifier.width(240.dp).padding(8.dp),
                horizontalArrangement = Arrangement.spacedBy(6.dp),
                verticalArrangement = Arrangement.spacedBy(6.dp),
            ) {
                state.tagList.forEach { tag ->
                    val on = tag.id in task.tagIds
                    val base = parseHexColor(tag.color, c.text3)
                    Box(
                        Modifier.clip(RoundedCornerShape(10.dp))
                            .background(accentGradient(if (on) base else base.copy(alpha = 0.14f)))
                            .clickableNoRipple { vm.toggleTag(task, tag.id) }
                            .padding(horizontal = 9.dp, vertical = 3.dp),
                    ) {
                        Text(tag.name, color = if (on) Tessera.colors.onPrimary else base, fontSize = 12.sp)
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
private fun DuePill(task: Task, vm: BoardViewModel) {
    val c = Tessera.colors
    var picker by remember { mutableStateOf(false) }
    val due = shortDate(task.dueDate)
    // Overdue (past due, not done) → red tint, like the web.
    val overdue = !task.isCompleted && isOverdue(task.dueDate)
    val overdueColor = Color(0xFFE0533D)
    val tint = when {
        overdue -> overdueColor
        due.isNotBlank() -> c.text2
        else -> c.text3
    }
    Pill(onClick = { picker = true }, set = due.isNotBlank()) {
        IonIcon(Ion.CALENDAR, size = 13.dp, tint = tint)
        if (due.isNotBlank()) {
            Spacer(Modifier.width(4.dp))
            Text(due, color = tint, fontSize = 11.sp)
        }
    }
    if (picker) {
        DueDatePicker(
            initialIso = task.dueDate,
            onPick = { iso ->
                vm.setDue(task, iso)
                picker = false
            },
            onDismiss = { picker = false },
        )
    }
}

@Composable
private fun AssigneesPill(task: Task, state: BoardUiState, vm: BoardViewModel) {
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
    Box {
        Box(Modifier.clip(RoundedCornerShape(RadiusSm)).clickableNoRipple { menu = true }.padding(2.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                // Author → assignee cascade (author muted, non-actionable).
                if (authorName != null) {
                    CardAvatar(authorName, muted = true, userId = if (task.gitlabAuthor == null) task.createdBy else null)
                    Text("→", color = c.text3, fontSize = 12.sp, modifier = Modifier.padding(horizontal = 3.dp))
                }
                if (assignees.isEmpty() && external.isEmpty()) {
                    IonIcon(Ion.PERSON_ADD, size = 14.dp, tint = c.text3)
                } else {
                    // Overlapping stack with a card-coloured ring (the "cutout").
                    Row(horizontalArrangement = Arrangement.spacedBy((-8).dp)) {
                        assignees.forEach { CardAvatar(it.name, muted = false, userId = it.userId) }
                        external.forEach { CardAvatar(it, muted = true) }
                    }
                }
            }
        }
        TDropdown(expanded = menu, onDismiss = { menu = false }) {
            state.members.forEach { m ->
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
private fun SubtaskRow(sub: Task, vm: BoardViewModel, onOpen: (Task) -> Unit, modifier: Modifier = Modifier) {
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
    }
}

@Composable
private fun AddSubtaskRow(task: Task, vm: BoardViewModel) {
    val c = Tessera.colors
    var adding by remember { mutableStateOf(false) }
    if (adding) {
        Box(Modifier.fillMaxWidth().padding(top = 4.dp)) {
            InlineCreateField(
                placeholder = "Название подзадачи, Enter",
                onCommit = {
                    vm.createTask(task.columnId, it, parentId = task.id)
                    adding = false
                },
                onDismiss = { adding = false },
            )
        }
    } else {
        Text(
            "+ СОЗДАТЬ ПОДЗАДАЧУ",
            color = c.text3.copy(alpha = 0.7f),
            fontSize = 10.sp,
            fontWeight = FontWeight.Medium,
            textAlign = TextAlign.Center,
            modifier = Modifier.fillMaxWidth().clickableNoRipple { adding = true }.padding(top = 10.dp, bottom = 4.dp),
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
            if (url != null) {
                AsyncImage(model = url, contentDescription = null, modifier = Modifier.size(21.dp).clip(CircleShape))
            } else {
                Text(
                    initials(name),
                    color = if (muted) Color.White else c.onPrimary,
                    fontSize = 9.sp,
                    fontWeight = FontWeight.SemiBold,
                )
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
