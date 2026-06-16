package website.msdnna.tessera.ui.screens

import android.content.Intent
import android.net.Uri
import androidx.compose.animation.AnimatedContent
import androidx.compose.animation.core.tween
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.slideInHorizontally
import androidx.compose.animation.slideOutHorizontally
import androidx.compose.animation.togetherWith
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Text
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
import androidx.compose.ui.draw.rotate
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.TransformOrigin
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import coil.compose.AsyncImage
import kotlinx.coroutines.launch
import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.api.RetrofitClient
import website.msdnna.tessera.data.model.GitlabAssignee
import website.msdnna.tessera.data.model.GitlabLink
import website.msdnna.tessera.data.model.Member
import website.msdnna.tessera.data.model.Tag
import website.msdnna.tessera.data.model.Task
import website.msdnna.tessera.ui.components.DueDatePicker
import website.msdnna.tessera.ui.components.IonIcon
import website.msdnna.tessera.ui.components.IonIconButton
import website.msdnna.tessera.ui.components.MarkdownEditor
import website.msdnna.tessera.ui.components.RichContent
import website.msdnna.tessera.ui.components.TButton
import website.msdnna.tessera.ui.components.TButtonKind
import website.msdnna.tessera.ui.components.TConfirmPopover
import website.msdnna.tessera.ui.components.TDropdown
import website.msdnna.tessera.ui.components.TMenuItem
import website.msdnna.tessera.ui.components.TSwitch
import website.msdnna.tessera.ui.components.TabItem
import website.msdnna.tessera.ui.components.TesseraLoader
import website.msdnna.tessera.ui.components.UnderlineTabs
import website.msdnna.tessera.ui.components.clickableNoRipple
import website.msdnna.tessera.ui.components.popupAppear
import website.msdnna.tessera.ui.theme.PriorityColors
import website.msdnna.tessera.ui.theme.PriorityLabels
import website.msdnna.tessera.ui.theme.RadiusLg
import website.msdnna.tessera.ui.theme.RadiusMd
import website.msdnna.tessera.ui.theme.RadiusSm
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.accentGradient
import website.msdnna.tessera.ui.theme.accentGradientTint
import website.msdnna.tessera.ui.viewmodels.TaskDetailViewModel
import website.msdnna.tessera.util.Ion
import website.msdnna.tessera.util.longDate
import website.msdnna.tessera.util.onColor
import website.msdnna.tessera.util.parseHexColor
import website.msdnna.tessera.util.readableHue
import website.msdnna.tessera.util.shortDate
import website.msdnna.tessera.util.whenLabel

private val RelKindLabels = mapOf(
    "relates" to "связана с",
    "blocks" to "блокирует",
    "blocked_by" to "заблокирована",
    "duplicates" to "дублирует",
)

/** Red used for destructive ghost actions (matches the web `--t-danger`). */
private val DangerRed = Color(0xFFE0533D)

/** Opens a downloaded attachment via the system, sharing it through our FileProvider. */
private fun openDownloadedFile(ctx: android.content.Context, file: java.io.File, mime: String?) {
    val uri = androidx.core.content.FileProvider.getUriForFile(ctx, "${ctx.packageName}.fileprovider", file)
    val type = mime?.takeIf { it.isNotBlank() } ?: "*/*"
    val view = android.content.Intent(android.content.Intent.ACTION_VIEW).apply {
        setDataAndType(uri, type)
        addFlags(android.content.Intent.FLAG_GRANT_READ_URI_PERMISSION or android.content.Intent.FLAG_ACTIVITY_NEW_TASK)
    }
    val chooser = android.content.Intent.createChooser(view, "Открыть «${file.name}»")
        .apply { addFlags(android.content.Intent.FLAG_ACTIVITY_NEW_TASK) }
    runCatching { ctx.startActivity(chooser) }
}

/**
 * The task detail modal (web `TaskModal.vue`), native: borderless title, a
 * property grid (priority / due / assignees / tags / completed / parent), a
 * Markdown description, and Комментарии / Подзадачи / Связи / Файлы / История
 * tabs. Card-level edits apply immediately; title + description save on
 * "Сохранить". Closing reports whether anything changed so the board refreshes.
 */
@Composable
fun TaskModal(
    initialTaskId: String,
    workspaceId: String,
    tags: List<Tag>,
    members: List<Member>,
    parentCandidates: List<Task>,
    breadcrumb: List<String>,
    onClose: (changed: Boolean) -> Unit,
) {
    val c = Tessera.colors
    val vm: TaskDetailViewModel = viewModel(key = "taskdetail")
    val state by vm.state.collectAsStateWithLifecycle()
    val me by AppContainer.prefs.user.collectAsStateWithLifecycle(initialValue = null)

    var currentId by remember { mutableStateOf(initialTaskId) }
    LaunchedEffect(currentId) { vm.load(currentId, workspaceId) }

    val detail = state.detail
    var title by remember(detail?.id) { mutableStateOf(detail?.title ?: "") }
    var description by remember(detail?.id) { mutableStateOf(detail?.description ?: "") }
    var tab by remember { mutableStateOf(0) }
    var confirmArchive by remember { mutableStateOf(false) }
    var confirmDelete by remember { mutableStateOf(false) }
    var showTransfer by remember { mutableStateOf(false) }

    // Read `changed` from the StateFlow's current value, not the composed
    // snapshot — a delete/archive flips it in the same coroutine that calls
    // close(), before the snapshot has caught the emission, so the board would
    // otherwise not refresh and the deleted card would linger.
    fun close() = onClose(vm.state.value.changed)

    Dialog(onDismissRequest = { close() }, properties = DialogProperties(usePlatformDefaultWidth = false)) {
        Column(
            Modifier
                .popupAppear(TransformOrigin.Center)
                .fillMaxWidth(0.96f)
                .fillMaxHeight(0.94f)
                .clip(RoundedCornerShape(RadiusLg))
                .background(c.surface),
        ) {
            // ── scrollable body ──
            Column(
                Modifier.weight(1f).verticalScroll(rememberScrollState()).padding(18.dp),
            ) {
                if (state.loading && detail == null) {
                    Box(Modifier.fillMaxWidth().height(200.dp), contentAlignment = Alignment.Center) {
                        TesseraLoader()
                    }
                } else if (detail != null) {
                    HeadRow(breadcrumb, detail.number, onTransfer = { showTransfer = true })
                    Spacer(Modifier.height(10.dp))
                    TitleField(title, onChange = { title = it })
                    Spacer(Modifier.height(14.dp))

                    PropertyGrid(
                        vm = vm,
                        priority = detail.priority,
                        dueIso = detail.dueDate,
                        completed = detail.isCompleted,
                        assignees = detail.assignees.map { it.id },
                        gitlabAssignees = detail.gitlabAssignees,
                        createdBy = detail.createdBy,
                        gitlab = detail.gitlab,
                        taskTagIds = detail.tags.map { it.id },
                        parentId = detail.parentId,
                        tags = tags,
                        members = members,
                        parentCandidates = parentCandidates,
                    )

                    Spacer(Modifier.height(16.dp))
                    Text("Описание", color = c.text3, fontSize = 12.sp, fontWeight = FontWeight.Medium)
                    Spacer(Modifier.height(4.dp))
                    MarkdownEditor(
                        value = description,
                        onValueChange = { description = it },
                        placeholder = "Добавьте описание…",
                        startInPreview = detail.description.isNotBlank(),
                        onBlur = { vm.saveDescription(description) },
                        uploadImage = { b, n, m -> vm.uploadMediaUrl(b, n, m) },
                        mentions = members.map { it.name },
                    )

                    Spacer(Modifier.height(18.dp))
                    UnderlineTabs(
                        tabs = listOf(
                            TabItem("Комментарии", state.comments.size),
                            TabItem("Подзадачи", detail.subtasks.size),
                            TabItem("Связи", state.relations.size),
                            TabItem("Файлы", state.attachments.size),
                            TabItem("История"),
                        ),
                        selected = tab,
                        onSelect = { tab = it },
                    )
                    Spacer(Modifier.height(12.dp))
                    AnimatedContent(
                        targetState = tab,
                        transitionSpec = {
                            // Slide toward the direction of travel (right when moving
                            // to a later tab), with a quick cross-fade.
                            val dir = if (targetState > initialState) 1 else -1
                            (slideInHorizontally(tween(220)) { w -> dir * w / 8 } + fadeIn(tween(200))) togetherWith
                                (slideOutHorizontally(tween(180)) { w -> -dir * w / 8 } + fadeOut(tween(160)))
                        },
                        label = "taskTab",
                    ) { t ->
                        when (t) {
                            0 -> CommentsTab(vm, state.comments, members, me?.id)

                            1 -> SubtasksTab(vm, detail.columnId, detail.subtasks) { currentId = it }

                            2 -> RelationsTab(
                                vm = vm,
                                relations = state.relations,
                                candidates = state.relationCandidates,
                                currentTaskId = detail.id,
                                onOpen = { currentId = it },
                            )

                            3 -> FilesTab(vm, state.attachments)

                            else -> HistoryTab(state.events)
                        }
                    }
                }
            }

            HorizontalDivider(color = c.border)
            // ── footer ──
            Row(
                Modifier.fillMaxWidth().padding(horizontal = 14.dp, vertical = 10.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Box {
                    GhostIconButton(Ion.ARCHIVE, c.primary) { confirmArchive = true }
                    TConfirmPopover(
                        expanded = confirmArchive,
                        message = "Архивировать задачу?",
                        confirmText = "В архив",
                        danger = false,
                        onConfirm = {
                            confirmArchive = false
                            vm.archive { close() }
                        },
                        onDismiss = { confirmArchive = false },
                    )
                }
                Spacer(Modifier.width(8.dp))
                Box {
                    GhostIconButton(Ion.TRASH, DangerRed) { confirmDelete = true }
                    TConfirmPopover(
                        expanded = confirmDelete,
                        message = "Удалить задачу? Это действие необратимо.",
                        confirmText = "Удалить",
                        onConfirm = {
                            confirmDelete = false
                            vm.delete { close() }
                        },
                        onDismiss = { confirmDelete = false },
                    )
                }
                Spacer(Modifier.weight(1f))
                TButton("Отмена", kind = TButtonKind.Secondary, onClick = { close() })
                Spacer(Modifier.width(8.dp))
                TButton("Сохранить", onClick = {
                    vm.saveCore(title, description)
                    onClose(true)
                })
            }
        }
    }

    if (showTransfer) {
        TransferBoardPicker(
            workspaceId = workspaceId,
            onPick = { boardId -> vm.transfer(boardId) { onClose(true) } },
            onDismiss = { showTransfer = false },
        )
    }
}

@Composable
private fun HeadRow(breadcrumb: List<String>, number: Long?, onTransfer: () -> Unit) {
    val c = Tessera.colors
    Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
        FlowRow(
            Modifier.weight(1f).clickableNoRipple(onClick = onTransfer),
            horizontalArrangement = Arrangement.spacedBy(6.dp),
        ) {
            breadcrumb.forEachIndexed { i, crumb ->
                Text(crumb, color = c.text3, fontSize = 12.sp)
                if (i < breadcrumb.size - 1) Text("/", color = c.text3.copy(alpha = 0.5f), fontSize = 12.sp)
            }
        }
        if (number != null) Text("#$number", color = c.text3, fontSize = 12.sp)
    }
}

@Composable
private fun TitleField(title: String, onChange: (String) -> Unit) {
    val c = Tessera.colors
    BasicTextField(
        value = title,
        onValueChange = onChange,
        textStyle = TextStyle(color = c.text1, fontSize = 18.sp, fontWeight = FontWeight.SemiBold),
        cursorBrush = SolidColor(c.primary),
        modifier = Modifier.fillMaxWidth(),
        decorationBox = { inner ->
            if (title.isEmpty()) Text("Название задачи", color = c.placeholder, fontSize = 18.sp, fontWeight = FontWeight.SemiBold)
            inner()
        },
    )
}

// ── property grid ────────────────────────────────────────────────────────────

@Composable
private fun PropertyGrid(
    vm: TaskDetailViewModel,
    priority: Int,
    dueIso: String?,
    completed: Boolean,
    assignees: List<String>,
    gitlabAssignees: List<GitlabAssignee>,
    createdBy: String?,
    gitlab: GitlabLink?,
    taskTagIds: List<String>,
    parentId: String?,
    tags: List<Tag>,
    members: List<Member>,
    parentCandidates: List<Task>,
) {
    // Author (read-only): GitLab issue author for synced tasks, else the creator.
    val authorName: String? = when {
        gitlab?.author?.isNotBlank() == true -> gitlab.authorName.ifBlank { gitlab.author }
        createdBy != null -> members.find { it.userId == createdBy }?.name
        else -> null
    }
    Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
        PropRow(Ion.FLAG, "Приоритет") { PriorityValue(priority) { vm.setPriority(it) } }
        PropRow(Ion.CALENDAR, "Срок") { DueValue(dueIso) { vm.setDue(it) } }
        if (authorName != null) {
            PropRow(Ion.PERSON_ADD, "Автор") {
                AuthorValue(
                    authorName,
                    gl = gitlab?.author?.takeIf { it.isNotBlank() },
                    userId = if (gitlab?.author?.isNotBlank() == true) null else createdBy,
                    avatarUrl = if (gitlab?.author?.isNotBlank() == true) gitlab.authorAvatarUrl else null,
                )
            }
        }
        PropRow(Ion.PEOPLE, "Исполнители") { AssigneesValue(assignees, gitlabAssignees, members) { vm.toggleAssignee(it) } }
        if (gitlab != null) {
            PropRow(Ion.GITLAB, "GitLab") { GitlabLinkValue(gitlab) }
        }
        PropRow(Ion.PRICETAG, "Теги") { TagsValue(taskTagIds, tags, onToggle = { vm.toggleTag(it) }, onCreate = { vm.createTagAndAdd(it) {} }) }
        PropRow(Ion.CHECK, "Выполнено") { TSwitch(checked = completed, onCheckedChange = { vm.setCompleted(it) }) }
        PropRow(Ion.GIT_MERGE, "Родитель") {
            ParentValue(parentId, parentCandidates, onAttach = { vm.attachToParent(it) }, onDetach = { vm.detachFromParent() })
        }
    }
}

@Composable
private fun PropRow(icon: String, label: String, value: @Composable () -> Unit) {
    val c = Tessera.colors
    Row(Modifier.fillMaxWidth().padding(vertical = 7.dp), verticalAlignment = Alignment.CenterVertically) {
        Row(Modifier.width(140.dp), verticalAlignment = Alignment.CenterVertically) {
            IonIcon(icon, size = 15.dp, tint = c.text3)
            Spacer(Modifier.width(8.dp))
            Text(label, color = c.text2, fontSize = 14.sp)
        }
        Box(Modifier.weight(1f)) { value() }
    }
}

@Composable
private fun PriorityValue(priority: Int, onPick: (Int) -> Unit) {
    val c = Tessera.colors
    var menu by remember { mutableStateOf(false) }
    Box {
        Row(Modifier.clickableNoRipple { menu = true }, verticalAlignment = Alignment.CenterVertically) {
            Box(Modifier.size(9.dp).clip(CircleShape).background(if (priority > 0) accentGradient(PriorityColors[priority]) else SolidColor(c.text3)))
            Spacer(Modifier.width(8.dp))
            Text(PriorityLabels[priority], color = c.text1, fontSize = 14.sp)
        }
        TDropdown(expanded = menu, onDismiss = { menu = false }) {
            PriorityLabels.forEachIndexed { i, label ->
                Row(
                    Modifier.fillMaxWidth().clickableNoRipple {
                        menu = false
                        onPick(i)
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
private fun DueValue(dueIso: String?, onPick: (String?) -> Unit) {
    val c = Tessera.colors
    var picker by remember { mutableStateOf(false) }
    val label = longDate(dueIso)
    Text(
        label.ifBlank { "Не задан" },
        color = if (label.isBlank()) c.text3 else c.text1,
        fontSize = 14.sp,
        modifier = Modifier.clickableNoRipple { picker = true },
    )
    if (picker) {
        DueDatePicker(initialIso = dueIso, onPick = {
            onPick(it)
            picker = false
        }, onDismiss = { picker = false })
    }
}

@Composable
private fun AssigneesValue(
    assignees: List<String>,
    gitlabAssignees: List<GitlabAssignee>,
    members: List<Member>,
    onToggle: (String) -> Unit,
) {
    val c = Tessera.colors
    var menu by remember { mutableStateOf(false) }
    val chosen = members.filter { it.userId in assignees }
    Box {
        Row(Modifier.clickableNoRipple { menu = true }, verticalAlignment = Alignment.CenterVertically) {
            if (chosen.isEmpty() && gitlabAssignees.isEmpty()) {
                Text("Никто", color = c.text3, fontSize = 14.sp)
            } else {
                chosen.forEach { m ->
                    MemberAvatar(24.dp, m.name, userId = m.userId)
                    Spacer(Modifier.width(4.dp))
                }
                // External GitLab assignees (no Tessera account) — muted, read-only.
                gitlabAssignees.forEach { g ->
                    MemberAvatar(24.dp, g.glName, avatarUrl = g.glAvatarUrl, muted = true)
                    Spacer(Modifier.width(4.dp))
                }
            }
        }
        TDropdown(expanded = menu, onDismiss = { menu = false }) {
            members.forEach { m ->
                val on = m.userId in assignees
                Row(
                    Modifier.fillMaxWidth().clickableNoRipple { onToggle(m.userId) }
                        .padding(horizontal = 12.dp, vertical = 8.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    MemberAvatar(22.dp, m.name, userId = m.userId)
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

/** A circular member avatar: the uploaded image (Tessera [userId] or GitLab
 *  [avatarUrl]) over a gradient/grey disc, falling back to initials. */
@Composable
private fun MemberAvatar(size: Dp, name: String, userId: String? = null, avatarUrl: String? = null, muted: Boolean = false) {
    val c = Tessera.colors
    val url = avatarUrl?.takeIf { it.isNotBlank() }
        ?: userId?.takeIf { it.isNotBlank() }?.let { "${RetrofitClient.serverRoot}/api/users/$it/avatar" }
    Box(
        Modifier.size(size).clip(CircleShape)
            .background(if (muted) SolidColor(c.text3) else accentGradient(c.primary)),
        contentAlignment = Alignment.Center,
    ) {
        if (url != null) {
            AsyncImage(model = url, contentDescription = null, modifier = Modifier.size(size).clip(CircleShape))
        } else {
            Text(initials(name), color = if (muted) Color.White else c.onPrimary, fontSize = 10.sp, fontWeight = FontWeight.SemiBold)
        }
    }
}

/** Read-only author display (creator or GitLab issue author). */
@Composable
private fun AuthorValue(name: String, gl: String?, userId: String? = null, avatarUrl: String? = null) {
    val c = Tessera.colors
    Row(verticalAlignment = Alignment.CenterVertically) {
        MemberAvatar(24.dp, name, userId = userId, avatarUrl = avatarUrl, muted = userId == null)
        Spacer(Modifier.width(8.dp))
        Text(name, color = c.text1, fontSize = 14.sp)
        if (gl != null) {
            Spacer(Modifier.width(6.dp))
            Text("@$gl · GitLab", color = c.text3, fontSize = 12.sp)
        }
    }
}

/** A clickable GitLab issue link (opens the issue in the browser). */
@Composable
private fun GitlabLinkValue(gitlab: GitlabLink) {
    val c = Tessera.colors
    val ctx = LocalContext.current
    Row(
        Modifier.clickableNoRipple {
            if (gitlab.webUrl.isNotBlank()) {
                runCatching { ctx.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(gitlab.webUrl))) }
            }
        },
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text("!${gitlab.iid}", fontSize = 14.sp, style = TextStyle(brush = accentGradient(c.primary)))
        Spacer(Modifier.width(6.dp))
        IonIcon(Ion.LINK, size = 13.dp, tint = c.text3)
    }
}

@Composable
private fun TagsValue(taskTagIds: List<String>, tags: List<Tag>, onToggle: (String) -> Unit, onCreate: (String) -> Unit) {
    val c = Tessera.colors
    var menu by remember { mutableStateOf(false) }
    val chosen = tags.filter { it.id in taskTagIds }
    Box {
        Row(
            Modifier.clickableNoRipple { menu = true },
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(5.dp),
        ) {
            if (chosen.isEmpty()) {
                Text("Нет", color = c.text3, fontSize = 14.sp)
            } else {
                // Show as many whole pills as fit on one line; the rest collapse to
                // a "+N" chip (no wrapping a tag name onto a second line).
                val maxVisible = 3
                chosen.take(maxVisible).forEach { t ->
                    val base = parseHexColor(t.color, c.text3)
                    val text = readableHue(base, c.isDark)
                    Box(
                        Modifier.clip(RoundedCornerShape(RadiusSm)).background(accentGradient(base.copy(alpha = 0.18f)))
                            .padding(horizontal = 8.dp, vertical = 2.dp),
                    ) {
                        Text(
                            t.name,
                            fontSize = 12.sp,
                            fontWeight = FontWeight.Medium,
                            maxLines = 1,
                            softWrap = false,
                            style = TextStyle(brush = accentGradient(text)),
                        )
                    }
                }
                val extra = chosen.size - maxVisible
                if (extra > 0) {
                    Box(
                        Modifier.clip(RoundedCornerShape(RadiusSm)).background(c.surfaceAlt)
                            .padding(horizontal = 8.dp, vertical = 2.dp),
                    ) { Text("+$extra", color = c.text3, fontSize = 12.sp, fontWeight = FontWeight.Medium) }
                }
            }
        }
        TDropdown(expanded = menu, onDismiss = { menu = false }) {
            FlowRow(
                Modifier.width(250.dp).padding(8.dp),
                horizontalArrangement = Arrangement.spacedBy(6.dp),
                verticalArrangement = Arrangement.spacedBy(6.dp),
            ) {
                tags.forEach { t ->
                    val on = t.id in taskTagIds
                    val base = parseHexColor(t.color, c.text3)
                    Box(
                        Modifier.clip(RoundedCornerShape(10.dp))
                            .background(accentGradient(if (on) base else base.copy(alpha = 0.14f)))
                            .clickableNoRipple { onToggle(t.id) }
                            .padding(horizontal = 9.dp, vertical = 3.dp),
                    ) { Text(t.name, color = if (on) onColor(base) else readableHue(base, c.isDark), fontSize = 12.sp) }
                }
            }
            Box(Modifier.padding(horizontal = 8.dp, vertical = 4.dp).width(250.dp)) {
                InlineEnterField("Новый тег, Enter") {
                    onCreate(it)
                    menu = false
                }
            }
        }
    }
}

@Composable
private fun ParentValue(parentId: String?, candidates: List<Task>, onAttach: (String) -> Unit, onDetach: () -> Unit) {
    val c = Tessera.colors
    var menu by remember { mutableStateOf(false) }
    if (parentId != null) {
        Text("Открепить", fontSize = 14.sp, style = TextStyle(brush = accentGradient(c.primary)), modifier = Modifier.clickableNoRipple { onDetach() })
    } else {
        Box {
            Text(
                "Сделать подзадачей…",
                color = c.text3,
                fontSize = 14.sp,
                modifier = Modifier.clickableNoRipple { menu = true },
            )
            TDropdown(expanded = menu, onDismiss = { menu = false }) {
                if (candidates.isEmpty()) {
                    Text(
                        "Нет других задач",
                        color = c.text3,
                        fontSize = 14.sp,
                        modifier = Modifier.padding(horizontal = 14.dp, vertical = 10.dp),
                    )
                }
                candidates.forEach { cand ->
                    TMenuItem(cand.title, onClick = {
                        menu = false
                        onAttach(cand.id)
                    })
                }
            }
        }
    }
}

// ── tabs ─────────────────────────────────────────────────────────────────────

@Composable
private fun CommentsTab(vm: TaskDetailViewModel, comments: List<website.msdnna.tessera.data.model.Comment>, members: List<Member>, meId: String?) {
    val c = Tessera.colors
    var draft by remember { mutableStateOf("") }
    var editingId by remember { mutableStateOf<String?>(null) }
    var editBody by remember { mutableStateOf("") }
    val mentionNames = members.map { it.name }

    Column(Modifier.fillMaxWidth()) {
        if (comments.isEmpty()) {
            Text("Комментариев пока нет", color = c.text3, fontSize = 13.sp, modifier = Modifier.padding(vertical = 6.dp))
        }
        comments.forEach { cm ->
            Row(Modifier.fillMaxWidth().padding(vertical = 8.dp)) {
                Box(
                    Modifier.size(26.dp).clip(CircleShape)
                        .then(if (cm.isGitlab) Modifier.background(c.text3) else Modifier.background(accentGradient(c.primary))),
                    contentAlignment = Alignment.Center,
                ) {
                    Text(
                        initials(cm.displayName ?: "?"),
                        color = if (cm.isGitlab) Color.White else c.onPrimary,
                        fontSize = 10.sp,
                        fontWeight = FontWeight.SemiBold,
                    )
                }
                Spacer(Modifier.width(8.dp))
                Column(Modifier.weight(1f)) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Text(cm.displayName ?: "Кто-то", color = c.text1, fontSize = 13.sp, fontWeight = FontWeight.SemiBold)
                        if (cm.isGitlab) {
                            Spacer(Modifier.width(5.dp))
                            Text("· GitLab", color = c.text3, fontSize = 11.sp)
                        }
                        Spacer(Modifier.width(6.dp))
                        Text(whenLabel(cm.createdAt), color = c.text3, fontSize = 11.sp)
                        if (cm.authorId != null && cm.authorId == meId) {
                            Spacer(Modifier.weight(1f))
                            IonIconButton(Ion.PENCIL, {
                                editingId = cm.id
                                editBody = cm.body
                            }, boxSize = 26.dp, iconSize = 14.dp, tint = c.text3)
                            IonIconButton(Ion.CLOSE, { vm.deleteComment(cm.id) }, boxSize = 26.dp, iconSize = 14.dp, tint = c.text3)
                        }
                    }
                    Spacer(Modifier.height(2.dp))
                    if (editingId == cm.id) {
                        MarkdownEditor(
                            value = editBody,
                            onValueChange = { editBody = it },
                            placeholder = "Комментарий…",
                            minHeight = 56.dp,
                            mentions = mentionNames,
                        )
                        Spacer(Modifier.height(6.dp))
                        Row {
                            TButton("Сохранить", onClick = {
                                vm.editComment(cm.id, editBody)
                                editingId = null
                            }, modifier = Modifier.height(34.dp))
                            Spacer(Modifier.width(6.dp))
                            TButton("Отмена", kind = TButtonKind.Secondary, onClick = { editingId = null }, modifier = Modifier.height(34.dp))
                        }
                    } else {
                        RichContent(cm.body, mentions = mentionNames)
                    }
                }
            }
        }
        Spacer(Modifier.height(8.dp))
        MarkdownEditor(
            value = draft,
            onValueChange = { draft = it },
            placeholder = "Написать комментарий… (@ — упоминание)",
            minHeight = 56.dp,
            uploadImage = { b, n, m -> vm.uploadMediaUrl(b, n, m) },
            mentions = mentionNames,
        )
        Spacer(Modifier.height(8.dp))
        TButton("Отправить", onClick = {
            if (draft.isNotBlank()) {
                vm.postComment(draft, members)
                draft = ""
            }
        })
    }
}

@Composable
private fun SubtasksTab(vm: TaskDetailViewModel, columnId: String, subtasks: List<Task>, onOpen: (String) -> Unit) {
    val c = Tessera.colors
    Column(Modifier.fillMaxWidth()) {
        if (subtasks.isEmpty()) {
            Text("Подзадач пока нет", color = c.text3, fontSize = 13.sp, modifier = Modifier.padding(vertical = 6.dp))
        }
        subtasks.forEach { sub ->
            Row(
                Modifier.fillMaxWidth().clip(RoundedCornerShape(RadiusSm)).background(c.surfaceAlt)
                    .clickableNoRipple { onOpen(sub.id) }.padding(horizontal = 10.dp, vertical = 9.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                IonIcon(
                    if (sub.isCompleted) Ion.CHECK_CIRCLE else Ion.ELLIPSE,
                    size = 17.dp,
                    tint = if (sub.isCompleted) c.primary else c.text3,
                    modifier = Modifier.clip(CircleShape).clickableNoRipple { vm.toggleSubtaskDone(sub.id, !sub.isCompleted) },
                )
                Spacer(Modifier.width(8.dp))
                if (sub.priority > 0) {
                    Box(Modifier.size(7.dp).clip(CircleShape).background(PriorityColors[sub.priority]))
                    Spacer(Modifier.width(6.dp))
                }
                Text(
                    sub.title,
                    color = c.text1,
                    fontSize = 13.sp,
                    textDecoration = if (sub.isCompleted) TextDecoration.LineThrough else null,
                    modifier = Modifier.weight(1f),
                )
                val due = shortDate(sub.dueDate)
                if (due.isNotBlank()) Text(due, color = c.text3, fontSize = 11.sp)
            }
            Spacer(Modifier.height(6.dp))
        }
        InlineEnterField("+ подзадача (Enter)") { vm.addSubtask(columnId, it) }
    }
}

private val RelKindOrder = listOf("relates", "blocks", "blocked_by", "duplicates")

@Composable
private fun RelationsTab(
    vm: TaskDetailViewModel,
    relations: List<website.msdnna.tessera.data.model.Relation>,
    candidates: List<website.msdnna.tessera.data.model.WorkspaceTask>,
    currentTaskId: String,
    onOpen: (String) -> Unit,
) {
    val c = Tessera.colors
    var kind by remember { mutableStateOf("relates") }
    var kindMenu by remember { mutableStateOf(false) }
    var query by remember { mutableStateOf("") }

    LaunchedEffect(Unit) { vm.ensureRelationCandidates() }

    Column(Modifier.fillMaxWidth()) {
        if (relations.isEmpty()) {
            Text("Связей пока нет", color = c.text3, fontSize = 13.sp, modifier = Modifier.padding(vertical = 6.dp))
        }
        relations.forEach { r ->
            Row(Modifier.fillMaxWidth().padding(vertical = 6.dp), verticalAlignment = Alignment.CenterVertically) {
                Text(RelKindLabels[r.kind] ?: r.kind, color = c.text3, fontSize = 12.sp, modifier = Modifier.width(90.dp))
                Row(
                    Modifier.weight(1f).clickableNoRipple { onOpen(r.relatedTaskId) },
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Text("#${r.relatedNumber ?: "?"}", fontSize = 13.sp, fontWeight = FontWeight.SemiBold, style = TextStyle(brush = accentGradient(c.primary)))
                    Spacer(Modifier.width(6.dp))
                    Text(
                        r.relatedTitle,
                        color = c.text1,
                        fontSize = 13.sp,
                        textDecoration = if (r.relatedCompletedAt != null) TextDecoration.LineThrough else null,
                    )
                }
                IonIconButton(Ion.CLOSE, { vm.removeRelation(r.id) }, boxSize = 26.dp, iconSize = 14.dp, tint = c.text3)
            }
        }

        Spacer(Modifier.height(10.dp))
        // ── add a relation: kind picker + cross-board task autocomplete ──
        Row(verticalAlignment = Alignment.CenterVertically) {
            Box {
                Row(
                    Modifier.clip(RoundedCornerShape(RadiusMd)).border(1.dp, c.border, RoundedCornerShape(RadiusMd))
                        .clickableNoRipple { kindMenu = true }.padding(horizontal = 10.dp, vertical = 9.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Text(RelKindLabels[kind] ?: kind, color = c.text1, fontSize = 13.sp)
                    Spacer(Modifier.width(4.dp))
                    IonIcon(Ion.CHEVRON_DOWN, size = 14.dp, tint = c.text3)
                }
                TDropdown(expanded = kindMenu, onDismiss = { kindMenu = false }) {
                    RelKindOrder.forEach { k ->
                        TMenuItem(RelKindLabels[k] ?: k, onClick = {
                            kind = k
                            kindMenu = false
                        })
                    }
                }
            }
            Spacer(Modifier.width(8.dp))
            Box(
                Modifier.weight(1f).clip(RoundedCornerShape(RadiusMd)).background(c.surface)
                    .border(1.dp, c.border, RoundedCornerShape(RadiusMd)).padding(horizontal = 12.dp, vertical = 10.dp),
            ) {
                BasicTextField(
                    value = query,
                    onValueChange = { query = it },
                    singleLine = true,
                    textStyle = TextStyle(color = c.text1, fontSize = 14.sp),
                    cursorBrush = SolidColor(c.primary),
                    keyboardOptions = KeyboardOptions(imeAction = ImeAction.Search),
                    modifier = Modifier.fillMaxWidth(),
                    decorationBox = { inner ->
                        if (query.isEmpty()) Text("Найти задачу: #№ или название", color = c.placeholder, fontSize = 14.sp)
                        inner()
                    },
                )
            }
        }

        val q = query.trim().lowercase()
        if (q.isNotEmpty()) {
            val matches = candidates.asSequence()
                .filter { it.id != currentTaskId && it.number != null }
                .filter { "#${it.number}".contains(q) || it.title.lowercase().contains(q) }
                .take(30)
                .toList()
            Spacer(Modifier.height(6.dp))
            if (matches.isEmpty()) {
                Text("Ничего не найдено", color = c.text3, fontSize = 12.sp, modifier = Modifier.padding(vertical = 4.dp))
            } else {
                matches.forEach { t ->
                    Row(
                        Modifier.fillMaxWidth().clickableNoRipple {
                            t.number?.let { vm.addRelation(it, kind) }
                            query = ""
                        }.padding(vertical = 8.dp),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        Text("#${t.number}", fontSize = 13.sp, fontWeight = FontWeight.SemiBold, style = TextStyle(brush = accentGradient(c.primary)))
                        Spacer(Modifier.width(8.dp))
                        Column(Modifier.weight(1f)) {
                            Text(t.title, color = c.text1, fontSize = 13.sp, maxLines = 1)
                            Text("${t.projectName} / ${t.boardName}", color = c.text3, fontSize = 11.sp, maxLines = 1)
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun FilesTab(vm: TaskDetailViewModel, attachments: List<website.msdnna.tessera.data.model.Attachment>) {
    val c = Tessera.colors
    val scope = rememberCoroutineScope()
    val ctx = androidx.compose.ui.platform.LocalContext.current
    val picker = androidx.activity.compose.rememberLauncherForActivityResult(
        androidx.activity.result.contract.ActivityResultContracts.GetContent(),
    ) { uri ->
        if (uri != null) {
            scope.launch {
                val bytes = kotlinx.coroutines.withContext(kotlinx.coroutines.Dispatchers.IO) {
                    ctx.contentResolver.openInputStream(uri)?.use { it.readBytes() }
                }
                val name = fileName(ctx, uri) ?: "file"
                val mime = ctx.contentResolver.getType(uri)
                if (bytes != null && bytes.isNotEmpty()) vm.uploadAttachment(bytes, name, mime)
            }
        }
    }
    Column(Modifier.fillMaxWidth()) {
        if (attachments.isEmpty()) {
            Text("Файлов пока нет", color = c.text3, fontSize = 13.sp, modifier = Modifier.padding(vertical = 6.dp))
        }
        attachments.forEach { a ->
            AttachmentRow(
                attachment = a,
                onDownload = {
                    vm.downloadAttachment(ctx.cacheDir, a.id, a.filename) { file -> openDownloadedFile(ctx, file, a.contentType) }
                },
                onDelete = { vm.removeAttachment(a.id) },
            )
        }
        Spacer(Modifier.height(8.dp))
        TButton("Прикрепить файл", kind = TButtonKind.Secondary, onClick = { picker.launch("*/*") })
    }
}

@Composable
private fun AttachmentRow(
    attachment: website.msdnna.tessera.data.model.Attachment,
    onDownload: () -> Unit,
    onDelete: () -> Unit,
) {
    val c = Tessera.colors
    var confirmDelete by remember { mutableStateOf(false) }
    Row(Modifier.fillMaxWidth().padding(vertical = 4.dp), verticalAlignment = Alignment.CenterVertically) {
        IonIcon(Ion.ATTACH, size = 16.dp, tint = c.text3)
        Spacer(Modifier.width(8.dp))
        Column(Modifier.weight(1f)) {
            Text(attachment.filename, color = c.text1, fontSize = 13.sp, maxLines = 2)
            Text(fmtSize(attachment.size), color = c.text3, fontSize = 11.sp)
        }
        Spacer(Modifier.width(6.dp))
        IonIconButton(Ion.DOWNLOAD, onDownload, boxSize = 44.dp, iconSize = 20.dp, tint = c.text2)
        Box {
            IonIconButton(Ion.TRASH, { confirmDelete = true }, boxSize = 44.dp, iconSize = 20.dp, tint = c.text2)
            TConfirmPopover(
                expanded = confirmDelete,
                message = "Удалить вложение?",
                onConfirm = {
                    confirmDelete = false
                    onDelete()
                },
                onDismiss = { confirmDelete = false },
            )
        }
    }
}

@Composable
private fun HistoryTab(events: List<website.msdnna.tessera.data.model.TaskEvent>) {
    val c = Tessera.colors
    Column(Modifier.fillMaxWidth()) {
        if (events.isEmpty()) {
            Text("История пуста", color = c.text3, fontSize = 13.sp, modifier = Modifier.padding(vertical = 6.dp))
        }
        events.forEach { e ->
            Row(Modifier.fillMaxWidth().padding(vertical = 6.dp), verticalAlignment = Alignment.CenterVertically) {
                Box(Modifier.size(22.dp).clip(CircleShape).background(c.surfaceAlt), contentAlignment = Alignment.Center) {
                    Text(initials(e.actorName ?: "?"), color = c.text2, fontSize = 9.sp, fontWeight = FontWeight.SemiBold)
                }
                Spacer(Modifier.width(8.dp))
                Text(
                    buildString {
                        append(e.actorName ?: "Кто-то")
                        append(' ')
                        append(eventText(e.kind))
                    },
                    color = c.text2,
                    fontSize = 13.sp,
                    modifier = Modifier.weight(1f),
                )
                Text(whenLabel(e.createdAt), color = c.text3, fontSize = 11.sp)
            }
        }
    }
}

// ── small helpers ──────────────────────────────────────────────────────────

@Composable
private fun GhostIconButton(icon: String, tint: Color, onClick: () -> Unit) {
    Box(
        Modifier.size(40.dp).clip(RoundedCornerShape(RadiusMd))
            .accentGradientTint(tint)
            .border(1.dp, tint.copy(alpha = 0.5f), RoundedCornerShape(RadiusMd))
            .clickableNoRipple(onClick = onClick),
        contentAlignment = Alignment.Center,
    ) { IonIcon(icon, size = 18.dp, tint = tint) }
}

@Composable
private fun InlineEnterField(placeholder: String, prefix: String? = null, onCommit: (String) -> Unit) {
    val c = Tessera.colors
    var text by remember { mutableStateOf("") }
    Row(
        Modifier.fillMaxWidth().clip(RoundedCornerShape(RadiusMd)).background(c.surface)
            .border(1.dp, c.border, RoundedCornerShape(RadiusMd)).padding(horizontal = 12.dp, vertical = 10.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        if (prefix != null) {
            Text(prefix, color = c.text3, fontSize = 14.sp)
            Spacer(Modifier.width(4.dp))
        }
        BasicTextField(
            value = text,
            onValueChange = { text = it },
            singleLine = true,
            textStyle = TextStyle(color = c.text1, fontSize = 14.sp),
            cursorBrush = SolidColor(c.primary),
            keyboardOptions = KeyboardOptions(imeAction = ImeAction.Done),
            keyboardActions = KeyboardActions(onDone = {
                val t = text.trim()
                if (t.isNotEmpty()) {
                    onCommit(t)
                    text = ""
                }
            }),
            modifier = Modifier.weight(1f),
            decorationBox = { inner ->
                if (text.isEmpty()) Text(placeholder, color = c.placeholder, fontSize = 14.sp)
                inner()
            },
        )
    }
}

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

/**
 * Board picker for transferring a task (web's breadcrumb "Перенести в доску").
 * Lists the workspace's projects; expanding one lazy-loads its boards. Picking a
 * board transfers the task there.
 */
@Composable
private fun TransferBoardPicker(workspaceId: String, onPick: (String) -> Unit, onDismiss: () -> Unit) {
    val c = Tessera.colors
    val scope = rememberCoroutineScope()
    val repo = remember { website.msdnna.tessera.data.repository.WorkspaceRepository() }
    var projects by remember { mutableStateOf<List<website.msdnna.tessera.data.model.Project>>(emptyList()) }
    var expanded by remember { mutableStateOf<String?>(null) }
    var boards by remember { mutableStateOf<Map<String, List<website.msdnna.tessera.data.model.Board>>>(emptyMap()) }
    var loading by remember { mutableStateOf(true) }

    LaunchedEffect(workspaceId) {
        loading = true
        projects = runCatching { repo.projects(workspaceId) }.getOrDefault(emptyList())
        loading = false
    }

    Dialog(onDismissRequest = onDismiss) {
        Column(
            Modifier.popupAppear(TransformOrigin.Center).fillMaxWidth().clip(RoundedCornerShape(RadiusLg)).background(c.surface).padding(18.dp),
        ) {
            Text("Перенести в доску", color = c.text1, fontSize = 16.sp, fontWeight = FontWeight.SemiBold)
            Spacer(Modifier.height(12.dp))
            when {
                loading -> Box(Modifier.fillMaxWidth().height(120.dp), contentAlignment = Alignment.Center) {
                    TesseraLoader()
                }

                projects.isEmpty() -> Text("Нет проектов", color = c.text3, fontSize = 13.sp)

                else -> Column(Modifier.heightIn(max = 360.dp).verticalScroll(rememberScrollState())) {
                    projects.forEach { p ->
                        Row(
                            Modifier.fillMaxWidth().clickableNoRipple {
                                if (expanded == p.id) {
                                    expanded = null
                                } else {
                                    expanded = p.id
                                    if (boards[p.id] == null) {
                                        scope.launch {
                                            val list = runCatching { repo.boards(p.id) }.getOrDefault(emptyList())
                                            boards = boards + (p.id to list)
                                        }
                                    }
                                }
                            }.padding(vertical = 10.dp),
                            verticalAlignment = Alignment.CenterVertically,
                        ) {
                            IonIcon(
                                Ion.CHEVRON_FORWARD,
                                size = 14.dp,
                                tint = c.text3,
                                modifier = Modifier.rotate(if (expanded == p.id) 90f else 0f),
                            )
                            Spacer(Modifier.width(8.dp))
                            Text(p.name, color = c.text1, fontSize = 14.sp, fontWeight = FontWeight.Medium)
                        }
                        if (expanded == p.id) {
                            val list = boards[p.id]
                            if (list == null) {
                                Text("Загрузка…", color = c.text3, fontSize = 12.sp, modifier = Modifier.padding(start = 30.dp, bottom = 6.dp))
                            } else if (list.isEmpty()) {
                                Text("Нет досок", color = c.text3, fontSize = 12.sp, modifier = Modifier.padding(start = 30.dp, bottom = 6.dp))
                            } else {
                                list.forEach { b ->
                                    Row(
                                        Modifier.fillMaxWidth().clickableNoRipple { onPick(b.id) }
                                            .padding(start = 30.dp, top = 7.dp, bottom = 7.dp),
                                        verticalAlignment = Alignment.CenterVertically,
                                    ) {
                                        IonIcon(Ion.GRID, size = 15.dp, tint = c.text3)
                                        Spacer(Modifier.width(8.dp))
                                        Text(b.name, color = c.text2, fontSize = 13.sp)
                                    }
                                }
                            }
                        }
                    }
                }
            }
            Spacer(Modifier.height(14.dp))
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
                TButton("Отмена", kind = TButtonKind.Ghost, onClick = onDismiss)
            }
        }
    }
}

private fun fmtSize(bytes: Long): String = when {
    bytes >= 1 shl 20 -> String.format(java.util.Locale.US, "%.1f МБ", bytes / (1 shl 20).toDouble())
    bytes >= 1 shl 10 -> String.format(java.util.Locale.US, "%.0f КБ", bytes / (1 shl 10).toDouble())
    else -> "$bytes Б"
}

private fun fileName(ctx: android.content.Context, uri: android.net.Uri): String? = runCatching {
    ctx.contentResolver.query(uri, null, null, null, null)?.use { cursor ->
        val idx = cursor.getColumnIndex(android.provider.OpenableColumns.DISPLAY_NAME)
        if (idx >= 0 && cursor.moveToFirst()) cursor.getString(idx) else null
    }
}.getOrNull()

private fun eventText(kind: String): String = when (kind) {
    "created" -> "создал(а) задачу"
    "renamed" -> "переименовал(а) задачу"
    "description" -> "изменил(а) описание"
    "priority" -> "сменил(а) приоритет"
    "due" -> "изменил(а) срок"
    "completed" -> "завершил(а) задачу"
    "reopened" -> "вернул(а) в работу"
    "moved" -> "переместил(а) задачу"
    "assigned" -> "назначил(а) исполнителя"
    "unassigned" -> "снял(а) исполнителя"
    "comment" -> "оставил(а) комментарий"
    "relation" -> "добавил(а) связь"
    "attachment" -> "прикрепил(а) файл"
    "archived" -> "архивировал(а) задачу"
    "restored" -> "восстановил(а) задачу"
    else -> kind
}
