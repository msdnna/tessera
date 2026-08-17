package website.msdnna.tessera.ui.screens

import android.content.Intent
import android.net.Uri
import androidx.activity.compose.BackHandler
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
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.systemBarsPadding
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
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.rotate
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.TransformOrigin
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import coil.compose.AsyncImage
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.api.RetrofitClient
import website.msdnna.tessera.data.model.BoardColumn
import website.msdnna.tessera.data.model.GitlabAssignee
import website.msdnna.tessera.data.model.GitlabLink
import website.msdnna.tessera.data.model.Member
import website.msdnna.tessera.data.model.Recurrence
import website.msdnna.tessera.data.model.Tag
import website.msdnna.tessera.data.model.Task
import website.msdnna.tessera.ui.TestTags
import website.msdnna.tessera.ui.components.DueDateTimePicker
import website.msdnna.tessera.ui.components.ErrorState
import website.msdnna.tessera.ui.components.IonIcon
import website.msdnna.tessera.ui.components.IonIconButton
import website.msdnna.tessera.ui.components.LoadingState
import website.msdnna.tessera.ui.components.MarkdownEditor
import website.msdnna.tessera.ui.components.MentionItem
import website.msdnna.tessera.ui.components.RichContent
import website.msdnna.tessera.ui.components.SourceBadge
import website.msdnna.tessera.ui.components.TButton
import website.msdnna.tessera.ui.components.TButtonKind
import website.msdnna.tessera.ui.components.TConfirmPopover
import website.msdnna.tessera.ui.components.TDropdown
import website.msdnna.tessera.ui.components.TInputDialog
import website.msdnna.tessera.ui.components.TMenuItem
import website.msdnna.tessera.ui.components.TabItem
import website.msdnna.tessera.ui.components.TagChipsFit
import website.msdnna.tessera.ui.components.TagLabel
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
import website.msdnna.tessera.ui.theme.TesseraDanger
import website.msdnna.tessera.ui.theme.accentGradient
import website.msdnna.tessera.ui.theme.accentGradientTint
import website.msdnna.tessera.ui.viewmodels.TaskDetailViewModel
import website.msdnna.tessera.util.CommandItem
import website.msdnna.tessera.util.Ion
import website.msdnna.tessera.util.buildTagGroups
import website.msdnna.tessera.util.columnById
import website.msdnna.tessera.util.doneTarget
import website.msdnna.tessera.util.dueLabel
import website.msdnna.tessera.util.isExternalSource
import website.msdnna.tessera.util.moveNeighbors
import website.msdnna.tessera.util.nextColumn
import website.msdnna.tessera.util.onColor
import website.msdnna.tessera.util.parseHexColor
import website.msdnna.tessera.util.readableHue
import website.msdnna.tessera.util.shortDate
import website.msdnna.tessera.util.siblingNeighbors
import website.msdnna.tessera.util.sortedColumns
import website.msdnna.tessera.util.sourceMeta
import website.msdnna.tessera.util.toggleTaskMarker
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
    projectId: String,
    tags: List<Tag>,
    prefixNames: Map<String, String>,
    metaTagPrefixes: Set<String> = emptySet(),
    members: List<Member>,
    gitlabMembers: List<website.msdnna.tessera.data.model.GitlabMember> = emptyList(),
    milestones: List<website.msdnna.tessera.data.model.Milestone> = emptyList(),
    parentCandidates: List<Task>,
    /** Every card of the host board — only used to work out a moved task's landing
     *  slot (sibling order / column tail); a task from another board simply
     *  matches nothing here and the backend picks the default slot. */
    boardTasks: List<Task> = emptyList(),
    breadcrumb: List<String>,
    estimation: website.msdnna.tessera.data.model.EstimationConfig =
        website.msdnna.tessera.util.Estimation.DEFAULT,
    /** Quick-action rows for the comment composer's `/`-popup; empty → no popup. */
    commands: List<CommandItem> = emptyList(),
    onClose: (changed: Boolean) -> Unit,
) {
    val c = Tessera.colors
    val vm: TaskDetailViewModel = viewModel(key = "taskdetail")
    val state by vm.state.collectAsStateWithLifecycle()
    val me by AppContainer.prefs.user.collectAsStateWithLifecycle(initialValue = null)

    var currentId by remember { mutableStateOf(initialTaskId) }
    LaunchedEffect(currentId) { vm.load(currentId, workspaceId, projectId) }

    // Report what the backend actually did with the comment's quick actions: intent
    // and result can differ (a recurring task bounces straight out of the done
    // column), so echo its own wording rather than the click.
    val toastCtx = LocalContext.current
    LaunchedEffect(state.commandNotice) {
        val summary = state.commandNotice ?: return@LaunchedEffect
        val applied = summary.applied.orEmpty().map { it.summary.ifBlank { "/${it.key}" } }
        val failed = summary.errors.orEmpty().map { "/${it.key}: ${it.error}" }
        val text = (
            listOfNotNull(applied.takeIf { it.isNotEmpty() }?.joinToString("; ")?.let { "Применено: $it" }) + failed
            ).joinToString("\n")
        if (text.isNotBlank()) android.widget.Toast.makeText(toastCtx, text, android.widget.Toast.LENGTH_LONG).show()
        vm.consumeCommandNotice()
    }

    // A failed mutation on an open task was silent until now: `state.error` only
    // renders in place of a modal that has no detail to show. A comment whose
    // commands all fail is a 400 with nothing stored — the user has to hear it.
    LaunchedEffect(state.error) {
        val err = state.error ?: return@LaunchedEffect
        if (state.detail == null) return@LaunchedEffect
        android.widget.Toast.makeText(toastCtx, err, android.widget.Toast.LENGTH_LONG).show()
        vm.clearError()
    }

    val detail = state.detail
    var title by remember(detail?.id) { mutableStateOf(detail?.title ?: "") }
    var description by remember(detail?.id) { mutableStateOf(detail?.description ?: "") }
    var tab by remember { mutableStateOf(0) }
    var confirmArchive by remember { mutableStateOf(false) }
    var confirmDelete by remember { mutableStateOf(false) }
    var showTransfer by remember { mutableStateOf(false) }
    // Milestones inline-created from the picker, merged with the board's list so a
    // freshly-made «Этап» shows immediately (the board reloads on close).
    var extraMilestones by remember(detail?.id) {
        mutableStateOf<List<website.msdnna.tessera.data.model.Milestone>>(emptyList())
    }
    val allMilestones = remember(milestones, extraMilestones) {
        (milestones + extraMilestones).distinctBy { it.id }
    }

    // Read `changed` from the StateFlow's current value, not the composed
    // snapshot — a delete/archive flips it in the same coroutine that calls
    // close(), before the snapshot has caught the emission, so the board would
    // otherwise not refresh and the deleted card would linger.
    fun close() = onClose(vm.state.value.changed)

    Dialog(
        onDismissRequest = { close() },
        // decorFitsSystemWindows=false lets the dialog window see the IME inset, so
        // `imePadding()` below can shrink the modal to the space above the keyboard —
        // otherwise the keyboard covers the bottom of the modal (the comment composer
        // and its `/`-suggestions) and only manual scrolling reveals it.
        properties = DialogProperties(usePlatformDefaultWidth = false, decorFitsSystemWindows = false),
    ) {
        // Back off a non-default tab returns to the first tab; a second Back (now
        // disabled) falls through to the Dialog's dismiss and closes the modal.
        BackHandler(enabled = tab != 0) { tab = 0 }
        Column(
            Modifier
                .testTag(TestTags.TASK_MODAL)
                .popupAppear(TransformOrigin.Center)
                // With decorFitsSystemWindows off the window spans the whole display,
                // so the modal has to keep clear of the bars itself.
                .systemBarsPadding()
                .imePadding()
                .fillMaxWidth(0.96f)
                .fillMaxHeight(0.94f)
                .clip(RoundedCornerShape(RadiusLg))
                .background(c.surface),
        ) {
            // ── scrollable body (loader/error centered in the body until loaded) ──
            if (state.loading && detail == null) {
                Box(Modifier.weight(1f).fillMaxWidth()) { LoadingState() }
            } else if (state.error != null && detail == null) {
                Box(Modifier.weight(1f).fillMaxWidth()) {
                    ErrorState(message = state.error ?: "Ошибка", onRetry = { vm.load(currentId, workspaceId, projectId) })
                }
            } else {
                Column(
                    Modifier.weight(1f).verticalScroll(rememberScrollState()).padding(18.dp),
                ) {
                    if (detail != null) {
                        HeadRow(breadcrumb, detail.number, onTransfer = { showTransfer = true })
                        Spacer(Modifier.height(10.dp))
                        TitleField(title, onChange = { title = it })
                        Spacer(Modifier.height(14.dp))

                        PropertyGrid(
                            vm = vm,
                            taskId = detail.id,
                            columnId = detail.columnId,
                            doneColumnId = state.doneColumnId,
                            moving = state.moving,
                            boardTasks = boardTasks,
                            priority = detail.priority,
                            dueIso = detail.dueDate,
                            startIso = detail.startDate,
                            recurrence = detail.recurrence,
                            estimate = detail.estimate,
                            estimation = estimation,
                            subtasks = detail.subtasks,
                            columns = state.columns,
                            notifyEnabled = detail.dueNotifyEnabled,
                            notifyLead = detail.dueLeadMinutes,
                            notifyRepeat = detail.dueRepeatMinutes,
                            completed = detail.isCompleted,
                            assignees = detail.assignees.map { it.id },
                            gitlabAssignees = detail.gitlabAssignees,
                            createdBy = detail.createdBy,
                            gitlab = detail.gitlab,
                            taskTagIds = detail.tags.map { it.id },
                            parentId = detail.parentId,
                            milestoneId = detail.milestoneId,
                            milestones = allMilestones,
                            onCreateMilestone = { t -> vm.createMilestoneAndAssign(t) { m -> extraMilestones = extraMilestones + m } },
                            tags = tags,
                            prefixNames = prefixNames,
                            metaTagPrefixes = metaTagPrefixes,
                            members = members,
                            gitlabMembers = gitlabMembers,
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
                            mentions = buildMentionItems(members, gitlabMembers),
                            fieldTag = TestTags.TASK_DESCRIPTION,
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
                                0 -> CommentsTab(
                                    vm = vm,
                                    comments = state.comments,
                                    members = members,
                                    gitlabMembers = gitlabMembers,
                                    meId = me?.id,
                                    commands = commands,
                                    preview = state.commandPreview,
                                    previewCustom = state.commandCustom,
                                )

                                1 -> SubtasksTab(vm, detail.columnId, detail.subtasks, state.columns) { currentId = it }

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
                TButton("Сохранить", modifier = Modifier.testTag(TestTags.TASK_SAVE), onClick = {
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
        modifier = Modifier.fillMaxWidth().testTag(TestTags.TASK_TITLE),
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
    taskId: String,
    columnId: String,
    doneColumnId: String?,
    moving: Boolean,
    boardTasks: List<Task>,
    priority: Int,
    dueIso: String?,
    startIso: String?,
    recurrence: Recurrence?,
    estimate: Double?,
    estimation: website.msdnna.tessera.data.model.EstimationConfig,
    subtasks: List<Task>,
    columns: List<BoardColumn>,
    notifyEnabled: Boolean?,
    notifyLead: Int?,
    notifyRepeat: Int?,
    completed: Boolean,
    assignees: List<String>,
    gitlabAssignees: List<GitlabAssignee>,
    createdBy: String?,
    gitlab: GitlabLink?,
    taskTagIds: List<String>,
    parentId: String?,
    milestoneId: String?,
    milestones: List<website.msdnna.tessera.data.model.Milestone>,
    onCreateMilestone: (String) -> Unit,
    tags: List<Tag>,
    prefixNames: Map<String, String>,
    metaTagPrefixes: Set<String>,
    members: List<Member>,
    gitlabMembers: List<website.msdnna.tessera.data.model.GitlabMember>,
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
        PropRow(Ion.CALENDAR, "Срок") {
            DueValue(
                dueIso, startIso, recurrence, columns, notifyEnabled, notifyLead, notifyRepeat,
                onApply = { iso, start, rec -> vm.setDueAndRecurrence(iso, start, rec) },
                onNotify = { lead, repeat, enabled -> vm.setDueNotify(lead, repeat, enabled) },
            )
        }
        PropRow(Ion.TIME, "Оценка") {
            val rollup = website.msdnna.tessera.util.Estimation.sum(subtasks.map { it.estimate })
            EstimateValue(
                estimate = estimate,
                cfg = estimation,
                rollupText = if (rollup != null) website.msdnna.tessera.util.Estimation.format(rollup, estimation) else "",
                onSet = { vm.setEstimate(it) },
            )
        }
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
        PropRow(Ion.PEOPLE, "Исполнители") {
            AssigneesValue(assignees, gitlabAssignees, members, gitlabMembers, { vm.toggleAssignee(it) }, { vm.toggleGitlabAssignee(it) })
        }
        if (gitlab != null) {
            PropRow(Ion.GITLAB, "GitLab") { GitlabLinkValue(gitlab) }
        }
        PropRow(Ion.PRICETAG, "Теги") {
            TagsValue(taskTagIds, tags, prefixNames, metaTagPrefixes, onToggle = { vm.toggleTag(it) }, onCreate = { vm.createTagAndAdd(it) {} })
        }
        if (milestones.isNotEmpty() || milestoneId != null) {
            PropRow(Ion.ROCKET, "Этап") {
                MilestoneValue(
                    milestoneId = milestoneId,
                    milestones = milestones,
                    onSet = { vm.setMilestone(it) },
                    onCreate = onCreateMilestone,
                )
            }
        }
        // Status: current column · shift right · close. Replaces the old
        // «Выполнено» switch — the column IS the status, and closing goes through
        // the board's done column so the backend stamps and logs it.
        val siblings = remember(boardTasks, parentId) {
            if (parentId == null) emptyList() else boardTasks.filter { it.parentId == parentId }.sortedBy { it.position }
        }
        fun neighboursFor(target: String) =
            moveNeighbors(taskId, parentId, target, siblings, parentCandidates)
        val doneCol = doneTarget(columns, doneColumnId)
        PropRow(Ion.CHECK, "Статус") {
            StatusValue(
                columns = columns,
                columnId = columnId,
                completed = completed,
                moving = moving,
                onMove = { target -> vm.moveToColumn(target, neighboursFor(target)) },
                onToggleDone = {
                    when {
                        completed -> vm.setCompleted(false)

                        doneCol != null && doneCol.id != columnId ->
                            vm.moveToColumn(doneCol.id, neighboursFor(doneCol.id))

                        else -> vm.setCompleted(true)
                    }
                },
            )
        }
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

/** A dot in the column's own colour — same visual weight as the priority dot. */
@Composable
private fun ColumnDot(column: BoardColumn?, size: Dp = 8.dp) {
    val c = Tessera.colors
    Box(Modifier.size(size).clip(CircleShape).background(accentGradient(parseHexColor(column?.color, c.text3))))
}

/**
 * A column chip that opens a picker of the board's columns. Shared by the status
 * row and the subtask rows ([mini]); picking a column moves the task there.
 */
@Composable
private fun ColumnChipPicker(
    columns: List<BoardColumn>,
    columnId: String,
    enabled: Boolean = true,
    mini: Boolean = false,
    /** e2e anchors, set only by the status row — see [TestTags.TASK_STATUS] for why
     *  the subtask chips deliberately stay untagged. */
    chipTag: String? = null,
    optionTag: ((String) -> String)? = null,
    onPick: (String) -> Unit,
) {
    val c = Tessera.colors
    val cols = remember(columns) { sortedColumns(columns) }
    val current = columnById(columns, columnId)
    var menu by remember { mutableStateOf(false) }
    Box {
        Row(
            Modifier.then(if (chipTag != null) Modifier.testTag(chipTag) else Modifier)
                .clip(RoundedCornerShape(RadiusSm))
                .background(c.surfaceAlt)
                .clickableNoRipple(enabled = enabled && cols.isNotEmpty()) { menu = true }
                .padding(horizontal = if (mini) 7.dp else 9.dp, vertical = if (mini) 3.dp else 5.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            ColumnDot(current, size = if (mini) 6.dp else 8.dp)
            Spacer(Modifier.width(6.dp))
            Text(
                current?.name ?: "—",
                color = c.text2,
                fontSize = if (mini) 11.sp else 13.sp,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
        }
        TDropdown(expanded = menu, onDismiss = { menu = false }, scrollable = true) {
            cols.forEach { col ->
                Row(
                    Modifier.fillMaxWidth()
                        .then(optionTag?.let { Modifier.testTag(it(col.id)) } ?: Modifier)
                        .clickableNoRipple {
                            menu = false
                            onPick(col.id)
                        }
                        .padding(horizontal = 14.dp, vertical = 10.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    ColumnDot(col)
                    Spacer(Modifier.width(10.dp))
                    Text(
                        col.name,
                        color = if (col.id == columnId) c.primary else c.text1,
                        fontSize = 14.sp,
                        fontWeight = if (col.id == columnId) FontWeight.Medium else FontWeight.Normal,
                    )
                }
            }
        }
    }
}

/**
 * The status row: [● column ▾] [› shift right] [✓ close]. The column is the
 * status, so «Выполнено» became a check that walks the task into the board's
 * done column (falling back to the plain flag when the board has none).
 */
@Composable
private fun StatusValue(
    columns: List<BoardColumn>,
    columnId: String,
    completed: Boolean,
    moving: Boolean,
    onMove: (String) -> Unit,
    onToggleDone: () -> Unit,
) {
    val c = Tessera.colors
    val next = nextColumn(columns, columnId)
    Row(verticalAlignment = Alignment.CenterVertically) {
        ColumnChipPicker(
            columns,
            columnId,
            enabled = !moving,
            chipTag = TestTags.TASK_STATUS,
            optionTag = TestTags::taskStatusOption,
            onPick = onMove,
        )
        Spacer(Modifier.width(8.dp))
        // One tap for the most common status change: shift one column right.
        IonIcon(
            Ion.CHEVRON_FORWARD,
            size = 16.dp,
            tint = if (next != null && !moving) c.text2 else c.text3.copy(alpha = 0.4f),
            modifier = Modifier.clip(CircleShape)
                .clickableNoRipple(enabled = next != null && !moving) { next?.let { onMove(it.id) } }
                .padding(4.dp),
        )
        Spacer(Modifier.width(4.dp))
        IonIcon(
            if (completed) Ion.CHECK_CIRCLE else Ion.ELLIPSE,
            gradient = completed,
            size = 17.dp,
            tint = if (completed) c.primary else c.text3,
            modifier = Modifier.clip(CircleShape)
                .clickableNoRipple(enabled = !moving) { onToggleDone() }
                .padding(4.dp),
        )
    }
}

@Composable
private fun PriorityValue(priority: Int, onPick: (Int) -> Unit) {
    val c = Tessera.colors
    var menu by remember { mutableStateOf(false) }
    Box {
        Row(
            Modifier.testTag(TestTags.TASK_PRIORITY).clickableNoRipple { menu = true },
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Box(Modifier.size(9.dp).clip(CircleShape).background(if (priority > 0) accentGradient(PriorityColors[priority]) else SolidColor(c.text3)))
            Spacer(Modifier.width(8.dp))
            Text(PriorityLabels[priority], color = c.text1, fontSize = 14.sp)
        }
        TDropdown(expanded = menu, onDismiss = { menu = false }) {
            PriorityLabels.forEachIndexed { i, label ->
                Row(
                    Modifier.fillMaxWidth()
                        .testTag(TestTags.taskPriorityOption(i))
                        .clickableNoRipple {
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
private fun DueValue(
    dueIso: String?,
    startIso: String?,
    recurrence: Recurrence?,
    columns: List<BoardColumn>,
    notifyEnabled: Boolean?,
    notifyLead: Int?,
    notifyRepeat: Int?,
    onApply: (String?, String?, Recurrence?) -> Unit,
    onNotify: (Int?, Int?, Boolean?) -> Unit,
) {
    val c = Tessera.colors
    var picker by remember { mutableStateOf(false) }
    val dueText = dueLabel(dueIso)
    val startText = dueLabel(startIso)
    // Show the bar as «начало → срок» when a start is set.
    val label = when {
        startText.isNotBlank() && dueText.isNotBlank() -> "$startText → $dueText"
        startText.isNotBlank() -> "$startText →"
        else -> dueText
    }
    Row(
        Modifier.clickableNoRipple { picker = true },
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(
            label.ifBlank { "Не задан" },
            color = if (label.isBlank()) c.text3 else c.text1,
            fontSize = 14.sp,
        )
        if (recurrence != null) {
            Spacer(Modifier.width(6.dp))
            // Recur glyph inherits the value text colour (web 0.113.2 parity).
            IonIcon(Ion.REPEAT, size = 13.dp, tint = if (label.isBlank()) c.text3 else c.text1)
        }
    }
    if (picker) {
        DueDateTimePicker(
            initialIso = dueIso,
            initialStartIso = startIso,
            initialRecurrence = recurrence,
            columns = columns,
            notifyEnabled = notifyEnabled,
            notifyLead = notifyLead,
            notifyRepeat = notifyRepeat,
            onApply = { iso, start, rec -> onApply(iso, start, rec) },
            onNotify = { lead, repeat, enabled -> onNotify(lead, repeat, enabled) },
            onDismiss = { picker = false },
        )
    }
}

@Composable
private fun EstimateValue(
    estimate: Double?,
    cfg: website.msdnna.tessera.data.model.EstimationConfig,
    rollupText: String,
    onSet: (Double?) -> Unit,
) {
    val c = Tessera.colors
    val scaleOptions = website.msdnna.tessera.util.Estimation.scaleOptions(cfg)
    val isPoints = scaleOptions.isNotEmpty()
    val label = website.msdnna.tessera.util.Estimation.format(estimate, cfg)
    var menu by remember { mutableStateOf(false) }
    var dialog by remember { mutableStateOf(false) }
    Box {
        Row(
            Modifier.clickableNoRipple { if (isPoints) menu = true else dialog = true },
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(
                label.ifBlank { "Не задана" },
                color = if (label.isBlank()) c.text3 else c.text1,
                fontSize = 14.sp,
            )
            if (rollupText.isNotBlank()) {
                Spacer(Modifier.width(8.dp))
                Text("Σ $rollupText", color = c.text3, fontSize = 12.sp)
            }
            if (estimate != null) {
                Spacer(Modifier.width(6.dp))
                IonIcon(
                    Ion.CLOSE,
                    size = 13.dp,
                    tint = c.text3,
                    modifier = Modifier.clickableNoRipple { onSet(null) },
                )
            }
        }
        if (isPoints) {
            TDropdown(expanded = menu, onDismiss = { menu = false }) {
                EstOptionRow("Не задана") {
                    menu = false
                    onSet(null)
                }
                scaleOptions.forEach { (lbl, v) ->
                    EstOptionRow(lbl) {
                        menu = false
                        onSet(v)
                    }
                }
            }
        }
    }
    if (dialog) {
        TInputDialog(
            title = "Оценка",
            initial = label,
            placeholder = website.msdnna.tessera.util.Estimation.placeholder(cfg),
            onConfirm = {
                dialog = false
                onSet(website.msdnna.tessera.util.Estimation.parse(it, cfg))
            },
            onDismiss = { dialog = false },
        )
    }
}

@Composable
private fun EstOptionRow(label: String, onClick: () -> Unit) {
    val c = Tessera.colors
    Row(
        Modifier.fillMaxWidth().clickableNoRipple(onClick = onClick).padding(horizontal = 14.dp, vertical = 10.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(label, color = c.text1, fontSize = 14.sp)
    }
}

@Composable
private fun AssigneesValue(
    assignees: List<String>,
    gitlabAssignees: List<GitlabAssignee>,
    members: List<Member>,
    gitlabMembers: List<website.msdnna.tessera.data.model.GitlabMember>,
    onToggle: (String) -> Unit,
    onToggleGl: (website.msdnna.tessera.data.model.GitlabMember) -> Unit,
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
        // GitLab members already linked to a Tessera account (tesseraUserId) are
        // shown once as the Tessera member (with a «GL» badge) and dropped from the
        // GitLab sublist — dedup parity with web.
        val linkedGlIds = gitlabMembers.mapNotNull { it.tesseraUserId }.toSet()
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
                    if (m.userId in linkedGlIds) {
                        GlLinkBadge()
                        Spacer(Modifier.width(6.dp))
                    }
                    if (on) {
                        IonIcon(Ion.CHECK, size = 16.dp, tint = c.primary, gradient = true)
                    }
                }
            }
            val unlinkedGl = gitlabMembers.filter { it.tesseraUserId == null }
            if (unlinkedGl.isNotEmpty()) {
                val assignedGl = gitlabAssignees.map { it.glUsername }.toSet()
                Text(
                    "GitLab",
                    color = c.text3, fontSize = 10.sp,
                    modifier = Modifier.padding(start = 12.dp, top = 8.dp, bottom = 2.dp),
                )
                unlinkedGl.forEach { m ->
                    val label = m.glName.ifBlank { m.glUsername }
                    Row(
                        Modifier.fillMaxWidth().clickableNoRipple { onToggleGl(m) }
                            .padding(horizontal = 12.dp, vertical = 8.dp),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        MemberAvatar(22.dp, label, avatarUrl = m.glAvatarUrl, muted = true)
                        Spacer(Modifier.width(8.dp))
                        Text(label, color = c.text1, fontSize = 14.sp, modifier = Modifier.weight(1f))
                        if (m.glUsername in assignedGl) {
                            Spacer(Modifier.width(8.dp))
                            IonIcon(Ion.CHECK, size = 16.dp, tint = c.primary, gradient = true)
                        }
                    }
                }
            }
        }
    }
}

/** Small «GL» pill marking a Tessera member also linked to a GitLab account. */
@Composable
private fun GlLinkBadge() {
    val c = Tessera.colors
    Box(
        Modifier.clip(RoundedCornerShape(6.dp)).background(c.primary.copy(alpha = 0.14f))
            .padding(horizontal = 6.dp, vertical = 1.dp),
    ) {
        Text("GL", color = c.primary, fontSize = 10.sp, fontWeight = FontWeight.SemiBold)
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
        // Initials underneath; the image overlays and shows through if it fails
        // to load (e.g. an unreachable GitLab avatar) rather than leaving a blank.
        Text(initials(name), color = if (muted) Color.White else c.onPrimary, fontSize = 10.sp, fontWeight = FontWeight.SemiBold)
        if (url != null) {
            AsyncImage(model = url, contentDescription = null, modifier = Modifier.size(size).clip(CircleShape))
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
private fun TagsValue(
    taskTagIds: List<String>,
    tags: List<Tag>,
    prefixNames: Map<String, String>,
    metaTagPrefixes: Set<String>,
    onToggle: (String) -> Unit,
    onCreate: (String) -> Unit,
) {
    val c = Tessera.colors
    var menu by remember { mutableStateOf(false) }
    val chosen = tags.filter { it.id in taskTagIds }
    Box {
        Box(Modifier.fillMaxWidth().clickableNoRipple { menu = true }) {
            if (chosen.isEmpty()) {
                Text("Нет", color = c.text3, fontSize = 14.sp)
            } else {
                // As many whole chips as fit on one line; the rest collapse to a "+N"
                // chip (web tag-fit) — no clipping a tag name.
                TagChipsFit(chosen, Modifier.fillMaxWidth(), prefixNames)
            }
        }
        TDropdown(expanded = menu, onDismiss = { menu = false }, scrollable = true) {
            // Group the chips by tag prefix; show headers only with >1 group (web parity).
            // GitLab meta-labels (status/priority/…) are hidden from the ADD picker.
            val groups = buildTagGroups(tags, prefixNames, metaTagPrefixes)
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
                    Modifier.width(250.dp).padding(horizontal = 8.dp, vertical = 4.dp),
                    horizontalArrangement = Arrangement.spacedBy(6.dp),
                    verticalArrangement = Arrangement.spacedBy(6.dp),
                ) {
                    g.tags.forEach { t ->
                        val on = t.id in taskTagIds
                        val base = parseHexColor(t.color, c.text3)
                        Box(
                            Modifier.clip(RoundedCornerShape(10.dp))
                                .background(accentGradient(if (on) base else base.copy(alpha = 0.14f)))
                                .clickableNoRipple { onToggle(t.id) }
                                .padding(horizontal = 9.dp, vertical = 3.dp),
                        ) {
                            // Scope already titles the section when headers show — the
                            // chip repeats only the value then (web `scopeMode="hide"`).
                            TagLabel(
                                t.name,
                                color = if (on) onColor(base) else readableHue(base, c.isDark),
                                prefixNames = prefixNames,
                                showScope = !headers,
                            )
                        }
                    }
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
        // Plain value-text style, matching «Сделать подзадачей…» (web 0.113.6).
        Text("Открепить", color = c.text3, fontSize = 14.sp, modifier = Modifier.clickableNoRipple { onDetach() })
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

/** Milestone («Этап») picker: shows the current milestone (title + date range), a
 *  dropdown to switch / clear («Без этапа») and an inline create field. */
@Composable
private fun MilestoneValue(
    milestoneId: String?,
    milestones: List<website.msdnna.tessera.data.model.Milestone>,
    onSet: (String?) -> Unit,
    onCreate: (String) -> Unit,
) {
    val c = Tessera.colors
    var menu by remember { mutableStateOf(false) }
    val chosen = milestones.firstOrNull { it.id == milestoneId }
    Box {
        Row(
            Modifier.clickableNoRipple { menu = true },
            verticalAlignment = Alignment.CenterVertically,
        ) {
            if (chosen == null) {
                Text("Без этапа", color = c.text3, fontSize = 14.sp)
            } else {
                val range = website.msdnna.tessera.util.Milestones.range(chosen.startDate, chosen.dueDate)
                Box(Modifier.alpha(if (chosen.isClosed) 0.6f else 1f)) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Text(chosen.title, color = c.text1, fontSize = 14.sp, maxLines = 1)
                        if (range.isNotEmpty()) {
                            Spacer(Modifier.width(8.dp))
                            Text(range, color = c.text3, fontSize = 12.sp, maxLines = 1)
                        }
                    }
                }
            }
        }
        TDropdown(expanded = menu, onDismiss = { menu = false }, scrollable = true) {
            TMenuItem("Без этапа", onClick = {
                menu = false
                onSet(null)
            }, trailing = {
                if (milestoneId == null) IonIcon(Ion.CHECK, size = 16.dp, tint = c.primary, gradient = true)
            })
            milestones.forEach { m ->
                val range = website.msdnna.tessera.util.Milestones.range(m.startDate, m.dueDate)
                val label = if (range.isEmpty()) m.title else "${m.title}  ·  $range"
                TMenuItem(label, onClick = {
                    menu = false
                    onSet(m.id)
                }, trailing = {
                    if (m.id == milestoneId) IonIcon(Ion.CHECK, size = 16.dp, tint = c.primary, gradient = true)
                })
            }
            Box(Modifier.padding(horizontal = 8.dp, vertical = 4.dp).width(250.dp)) {
                InlineEnterField("Новый этап, Enter") {
                    onCreate(it)
                    menu = false
                }
            }
        }
    }
}

/** @-mention candidates (web `mentionItems`): Tessera members insert their name;
 *  GitLab-only users (no Tessera account) insert their username, which GitLab
 *  resolves on write-back. Tessera-side notifications key off the name via
 *  `detectMentions`, so GitLab users never generate a Tessera notification. */
private fun buildMentionItems(
    members: List<Member>,
    gitlabMembers: List<website.msdnna.tessera.data.model.GitlabMember>,
): List<MentionItem> {
    val tessera = members.map { MentionItem(insert = it.name, display = it.name, avatarUserId = it.userId) }
    val gl = gitlabMembers.filter { it.tesseraUserId == null }.map {
        MentionItem(
            insert = it.glUsername,
            display = it.glName.ifBlank { it.glUsername },
            avatarSrc = it.glAvatarUrl,
            gitlab = true,
        )
    }
    return tessera + gl
}

/** How long the composer waits after a keystroke before dry-running the draft. */
private const val CommandPreviewDebounceMs = 400L

/**
 * «Будет применено» under the composer: one flat row per `/`-command in the draft
 * with the backend's own wording, errors in the danger colour. Custom dictionary
 * keys are listed apart — they are never executed and stay in the comment text,
 * so promising an action for them would be a lie.
 */
@Composable
private fun CommandPreviewStrip(
    preview: List<website.msdnna.tessera.data.model.CommandOutcome>,
    custom: List<String>,
) {
    val c = Tessera.colors
    Column(
        Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(RadiusMd))
            .background(c.surfaceAlt)
            .padding(horizontal = 10.dp, vertical = 8.dp),
    ) {
        preview.forEach { o ->
            Row(Modifier.fillMaxWidth().padding(vertical = 2.dp)) {
                Text("/${o.key}", color = c.text2, fontSize = 12.sp, fontFamily = FontFamily.Monospace)
                Spacer(Modifier.width(8.dp))
                Text(
                    o.error.ifBlank { o.summary },
                    color = if (o.error.isNotBlank()) TesseraDanger else c.text3,
                    fontSize = 12.sp,
                    modifier = Modifier.weight(1f),
                )
            }
        }
        if (custom.isNotEmpty()) {
            Text(
                custom.joinToString(", ") { "/$it" } + " — останется текстом",
                color = c.text3,
                fontSize = 11.sp,
                modifier = Modifier.padding(top = 2.dp),
            )
        }
    }
}

// ── tabs ─────────────────────────────────────────────────────────────────────

@Composable
private fun CommentsTab(
    vm: TaskDetailViewModel,
    comments: List<website.msdnna.tessera.data.model.Comment>,
    members: List<Member>,
    gitlabMembers: List<website.msdnna.tessera.data.model.GitlabMember>,
    meId: String?,
    commands: List<CommandItem>,
    preview: List<website.msdnna.tessera.data.model.CommandOutcome>,
    previewCustom: List<String>,
) {
    val c = Tessera.colors
    var draft by remember { mutableStateOf("") }
    var editingId by remember { mutableStateOf<String?>(null) }
    var editBody by remember { mutableStateOf("") }
    val mentionItems = buildMentionItems(members, gitlabMembers)
    // Highlight tokens for read-only comment rendering (names + GitLab usernames).
    val mentionNames = mentionItems.map { it.insert }

    Column(Modifier.fillMaxWidth()) {
        if (comments.isEmpty()) {
            Text("Комментариев пока нет", color = c.text3, fontSize = 13.sp, modifier = Modifier.padding(vertical = 6.dp))
        }
        comments.forEach { cm ->
            Row(Modifier.fillMaxWidth().padding(vertical = 8.dp)) {
                MemberAvatar(
                    26.dp,
                    cm.displayName ?: "?",
                    userId = cm.authorId,
                    avatarUrl = cm.glAuthorAvatarUrl,
                    muted = cm.isGitlab,
                )
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
                            mentions = mentionItems,
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
                        val own = cm.authorId != null && cm.authorId == meId
                        RichContent(
                            cm.body,
                            mentions = mentionNames,
                            interactive = own,
                            onToggleCheck = if (own) {
                                { i -> vm.editComment(cm.id, toggleTaskMarker(cm.body, i)) }
                            } else {
                                null
                            },
                        )
                    }
                }
            }
        }
        Spacer(Modifier.height(8.dp))
        MarkdownEditor(
            value = draft,
            onValueChange = { draft = it },
            placeholder = if (commands.isEmpty()) {
                "Написать комментарий… (@ — упоминание)"
            } else {
                "Написать комментарий… (@ — упоминание, / — команда)"
            },
            minHeight = 56.dp,
            uploadImage = { b, n, m -> vm.uploadMediaUrl(b, n, m) },
            mentions = mentionItems,
            commands = commands,
            fieldTag = TestTags.TASK_COMMENT_INPUT,
        )
        // Dry-run the draft against the backend's parser instead of re-implementing
        // it here: the hint can never disagree with what will actually happen.
        LaunchedEffect(draft) {
            delay(CommandPreviewDebounceMs)
            vm.previewCommands(draft)
        }
        if (preview.isNotEmpty() || previewCustom.isNotEmpty()) {
            Spacer(Modifier.height(6.dp))
            CommandPreviewStrip(preview, previewCustom)
        }
        Spacer(Modifier.height(8.dp))
        TButton("Отправить", modifier = Modifier.testTag(TestTags.TASK_COMMENT_SUBMIT), onClick = {
            if (draft.isNotBlank()) {
                vm.postComment(draft, members)
                draft = ""
            }
        })
    }
}

@Composable
private fun SubtasksTab(
    vm: TaskDetailViewModel,
    columnId: String,
    subtasks: List<Task>,
    columns: List<BoardColumn>,
    onOpen: (String) -> Unit,
) {
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
                if (due.isNotBlank()) {
                    Text(due, color = c.text3, fontSize = 11.sp)
                    Spacer(Modifier.width(8.dp))
                }
                // Status of the subtask, changeable without opening it.
                if (columns.isNotEmpty()) {
                    ColumnChipPicker(
                        columns = columns,
                        columnId = sub.columnId,
                        mini = true,
                        onPick = { target -> vm.moveSubtask(sub.id, target, siblingNeighbors(subtasks, sub.id)) },
                    )
                }
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

    // id of the relation whose delete-confirm popover is open (null = none)
    var confirmRemove by remember { mutableStateOf<String?>(null) }

    LaunchedEffect(Unit) { vm.ensureRelationCandidates() }

    Column(Modifier.fillMaxWidth()) {
        if (relations.isEmpty()) {
            Text("Связей пока нет", color = c.text3, fontSize = 13.sp, modifier = Modifier.padding(vertical = 6.dp))
        }
        relations.forEach { r ->
            Row(Modifier.fillMaxWidth().padding(vertical = 6.dp), verticalAlignment = Alignment.CenterVertically) {
                Text(RelKindLabels[r.kind] ?: r.kind, color = c.text3, fontSize = 12.sp, modifier = Modifier.width(90.dp))
                if (isExternalSource(r.source)) {
                    SourceBadge(sourceMeta(r.source))
                    Spacer(Modifier.width(6.dp))
                }
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
                Box {
                    IonIconButton(Ion.CLOSE, { confirmRemove = r.id }, boxSize = 26.dp, iconSize = 14.dp, tint = c.text3)
                    // Deleting an integration-owned relation only holds until the next
                    // sync re-projects it — say so instead of promising it stays gone.
                    TConfirmPopover(
                        expanded = confirmRemove == r.id,
                        message = if (isExternalSource(r.source)) {
                            "Эта связь вернётся при следующем синке ${sourceMeta(r.source).label}. Удалить?"
                        } else {
                            "Убрать связь?"
                        },
                        onConfirm = {
                            vm.removeRelation(r.id)
                            confirmRemove = null
                        },
                        onDismiss = { confirmRemove = null },
                    )
                }
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
    "recurred" -> "перенёс(ла) повтор задачи"
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
