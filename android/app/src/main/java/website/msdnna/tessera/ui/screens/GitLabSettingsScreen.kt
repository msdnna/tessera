package website.msdnna.tessera.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import website.msdnna.tessera.data.model.GitlabBindAction
import website.msdnna.tessera.data.model.GitlabBindTrigger
import website.msdnna.tessera.data.model.GitlabBinding
import website.msdnna.tessera.data.model.GitlabIntegration
import website.msdnna.tessera.data.model.GitlabIntegrationRequest
import website.msdnna.tessera.data.model.GitlabRule
import website.msdnna.tessera.data.model.GitlabRules
import website.msdnna.tessera.data.model.GitlabWriteback
import website.msdnna.tessera.ui.components.IonIcon
import website.msdnna.tessera.ui.components.TButton
import website.msdnna.tessera.ui.components.TButtonKind
import website.msdnna.tessera.ui.components.TCard
import website.msdnna.tessera.ui.components.TConfirmDialog
import website.msdnna.tessera.ui.components.TDropdown
import website.msdnna.tessera.ui.components.TFormError
import website.msdnna.tessera.ui.components.TMenuItem
import website.msdnna.tessera.ui.components.TSwitch
import website.msdnna.tessera.ui.components.TTextField
import website.msdnna.tessera.ui.components.TesseraLoader
import website.msdnna.tessera.ui.components.clickableNoRipple
import website.msdnna.tessera.ui.components.dashedBorder
import website.msdnna.tessera.ui.theme.ConflictAmber
import website.msdnna.tessera.ui.theme.PriorityLabels
import website.msdnna.tessera.ui.theme.RadiusMd
import website.msdnna.tessera.ui.theme.RadiusSm
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.accentGradient
import website.msdnna.tessera.ui.viewmodels.GitlabViewModel
import website.msdnna.tessera.util.Ion
import website.msdnna.tessera.util.canonPrefix
import website.msdnna.tessera.util.localDateTimeLabel

private val IntervalOptions = listOf(
    0 to "Вручную (выкл.)", 300 to "Каждые 5 минут", 900 to "Каждые 15 минут", 3600 to "Каждый час",
)
private val DueSourceOptions = listOf(
    "issue_milestone" to "Issue, иначе Milestone", "issue" to "Только Issue",
    "milestone" to "Только Milestone", "off" to "Не синхронизировать",
)
private val StartSourceOptions = listOf(
    "created" to "Дата создания", "milestone" to "Начало Milestone", "off" to "Не синхронизировать",
)
private val ActionOptions = listOf(
    "status" to "Статус → колонка", "priority" to "Приоритет", "board" to "Доска",
    "tag" to "Тег", "group" to "Группировка", "ignore" to "Игнорировать",
)
private val MatchTypeOptions = listOf("prefix" to "Префикс", "regex" to "Regex")
private val DefaultActionOptions = listOf("tag" to "Создавать тег", "ignore" to "Игнорировать")
private val MapActions = setOf("status", "priority", "board")
private val ScopeOptions = listOf(
    "assigned" to "Только назначенные мне", "all" to "Все задачи проекта",
)
private val ClosedPolicyOptions = listOf(
    "all" to "Импортировать все", "archive_closed_sprints" to "Архивировать закрытые этапы",
    "period" to "Только за период",
)

// ── write-back binding option lists (mirror web GitLabModal) ─────────────────
private val TriggerTypeOptions = listOf(
    "column" to "Перемещение в колонку", "completion" to "Флаг «Выполнено»",
    "priority" to "Изменение приоритета", "due" to "Изменение срока",
    "assignees" to "Изменение исполнителей", "estimate" to "Изменение оценки",
    "milestone" to "Изменение этапа", "title_desc" to "Заголовок / описание",
    "labels" to "Изменение тегов", "comment" to "Новый комментарий",
)
private val ActionTypeOptions = listOf(
    "set_label" to "Установить метку", "set_state" to "Закрыть / открыть issue",
    "set_due" to "Установить срок", "set_assignees" to "Установить исполнителей",
    "set_estimate" to "Установить оценку", "set_milestone" to "Установить этап",
    "set_title_desc" to "Обновить заголовок/описание", "reconcile_labels" to "Синхронизировать теги",
    "post_comment" to "Написать комментарий",
)
private val StateOptions = listOf(
    "" to "Из флага «Выполнено»", "closed" to "Закрыть issue", "opened" to "Открыть issue",
)
private val DateKindOptions = listOf(
    "due" to "Срок (due)", "start" to "Начало (start) — для issue игнорируется",
)

// completion qualifier: "" = any change, "true"/"false" = became/cleared done.
private val CompletionOptions = listOf(
    "" to "Любое изменение", "true" to "Стало «Выполнено»", "false" to "Снято «Выполнено»",
)

// priority qualifier: "" = any level, "0".."4" = a specific level.
private val PriorityQualOptions =
    listOf("" to "Любой приоритет") + PriorityLabels.mapIndexed { i, l -> i.toString() to l }

// The sensible default GitLab action for a freshly-picked trigger.
private val DefaultActionForTrigger = mapOf(
    "column" to "set_label", "completion" to "set_state", "priority" to "set_label",
    "due" to "set_due", "assignees" to "set_assignees", "estimate" to "set_estimate",
    "milestone" to "set_milestone", "title_desc" to "set_title_desc",
    "labels" to "reconcile_labels", "comment" to "post_comment",
)

/** GitLab settings: connect a PAT account, configure the per-workspace
 *  integration (project → board, interval, due source) with a generic label
 *  rule editor, and run a manual sync. Mirrors the web `GitLabModal`. */
@Composable
fun GitLabSettingsScreen(
    workspaceId: String,
    onOpenJournal: () -> Unit = {},
    conflictCount: Int = 0,
    onOpenConflicts: () -> Unit = {},
    vm: GitlabViewModel = viewModel(key = "gitlab"),
) {
    val c = Tessera.colors
    val state by vm.state.collectAsStateWithLifecycle()
    LaunchedEffect(workspaceId) { vm.loadAll(workspaceId) }

    if (state.loading) {
        Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) { TesseraLoader() }
        return
    }
    Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(16.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            IonIcon(Ion.GITLAB, size = 20.dp, tint = c.primary, gradient = true)
            Spacer(Modifier.width(8.dp))
            Text("GitLab", color = c.text1, fontSize = 20.sp, fontWeight = FontWeight.Bold)
        }
        state.error?.let {
            Spacer(Modifier.height(10.dp))
            TFormError(it)
        }
        state.message?.let {
            Spacer(Modifier.height(10.dp))
            Text(it, color = c.primary, fontSize = 13.sp)
        }
        Spacer(Modifier.height(14.dp))

        AccountCard(state, vm, workspaceId)

        if (state.serviceConfigured || state.connected || state.integrations.isNotEmpty()) {
            Spacer(Modifier.height(16.dp))
            BindingsSection(state, vm, workspaceId, onOpenJournal)
        }
        if (conflictCount > 0) {
            Spacer(Modifier.height(14.dp))
            Row(
                Modifier.fillMaxWidth().clip(RoundedCornerShape(RadiusMd))
                    .border(1.dp, ConflictAmber.copy(alpha = 0.55f), RoundedCornerShape(RadiusMd))
                    .background(ConflictAmber.copy(alpha = 0.12f))
                    .clickableNoRipple(onClick = onOpenConflicts)
                    .padding(horizontal = 14.dp, vertical = 12.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                IonIcon(Ion.GIT_NETWORK, size = 17.dp, tint = ConflictAmber)
                Spacer(Modifier.width(10.dp))
                Text(
                    "Конфликты обратной записи: $conflictCount",
                    color = c.text1,
                    fontSize = 14.sp,
                    fontWeight = FontWeight.Medium,
                    modifier = Modifier.weight(1f),
                )
                IonIcon(Ion.CHEVRON_FORWARD, size = 14.dp, tint = c.text3)
            }
        }
        Spacer(Modifier.height(40.dp))
    }
}

@Composable
private fun AccountCard(state: website.msdnna.tessera.ui.viewmodels.GitlabUiState, vm: GitlabViewModel, workspaceId: String) {
    val c = Tessera.colors
    SectionLabel("Аккаунт")
    TCard {
        if (state.connected) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Column(Modifier.weight(1f)) {
                    Text("@${state.glUsername}", color = c.text1, fontSize = 15.sp, fontWeight = FontWeight.SemiBold)
                    Text(state.baseUrl, color = c.text3, fontSize = 12.sp)
                }
                TButton("Отключить", onClick = { vm.disconnect() }, kind = TButtonKind.Secondary)
            }
        } else {
            var base by remember { mutableStateOf("") }
            var token by remember { mutableStateOf("") }
            Column {
                TTextField(base, { base = it }, label = "URL GitLab", placeholder = "https://gitlab.example.com")
                Spacer(Modifier.height(10.dp))
                TTextField(token, { token = it }, label = "Токен (PAT, scope read_api)", placeholder = "glpat-…", isPassword = true)
                Spacer(Modifier.height(12.dp))
                TButton(
                    "Подключить",
                    onClick = { if (base.isNotBlank() && token.isNotBlank()) vm.connect(workspaceId, base, token) },
                    loading = state.connecting,
                    modifier = Modifier.fillMaxWidth(),
                )
            }
        }
    }
}

/** Lists a workspace's GitLab bindings (or the create/edit editor). Admins can
 *  add/edit/delete bindings; members can trigger a manual sync. */
@Composable
private fun BindingsSection(
    state: website.msdnna.tessera.ui.viewmodels.GitlabUiState,
    vm: GitlabViewModel,
    workspaceId: String,
    onOpenJournal: () -> Unit,
) {
    val c = Tessera.colors
    var editing by remember { mutableStateOf(false) }
    var editTarget by remember { mutableStateOf<GitlabIntegration?>(null) }

    fun openEditor(binding: GitlabIntegration?) {
        editTarget = binding
        editing = true
        vm.prepareEditor(binding)
    }

    SectionLabel("Привязки GitLab")
    if (editing) {
        IntegrationEditor(
            binding = editTarget,
            state = state,
            vm = vm,
            workspaceId = workspaceId,
            onClose = {
                editing = false
                editTarget = null
            },
        )
        return
    }

    if (state.integrations.isEmpty()) {
        Text("Привязок пока нет.", color = c.text3, fontSize = 13.sp)
        Spacer(Modifier.height(8.dp))
    } else {
        state.integrations.forEach { integ ->
            BindingRow(integ, state, vm, workspaceId, onEdit = { openEditor(integ) })
            Spacer(Modifier.height(8.dp))
        }
    }
    if (state.isAdmin) {
        DashedAddButton("Привязка", onClick = { openEditor(null) })
    } else if (!state.serviceConfigured && !state.connected) {
        Text("Подключите аккаунт GitLab, чтобы синхронизировать.", color = c.text3, fontSize = 12.sp)
    }
    Spacer(Modifier.height(12.dp))
    TButton(
        "Журнал синхронизации",
        onClick = onOpenJournal,
        kind = TButtonKind.Ghost,
        icon = Ion.TIME,
        modifier = Modifier.fillMaxWidth(),
    )
}

/** A single binding card: project → board, enabled/last-synced, sync + edit/delete. */
@Composable
private fun BindingRow(
    integ: GitlabIntegration,
    state: website.msdnna.tessera.ui.viewmodels.GitlabUiState,
    vm: GitlabViewModel,
    workspaceId: String,
    onEdit: () -> Unit,
) {
    val c = Tessera.colors
    val boardLabel = state.boards.find { it.id == integ.boardId }?.label ?: "—"
    var confirmDelete by remember { mutableStateOf(false) }
    val syncing = state.syncingId == integ.id
    TCard {
        Column {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Column(Modifier.weight(1f)) {
                    Text(integ.name.ifBlank { integ.projectPath }, color = c.text1, fontSize = 15.sp, fontWeight = FontWeight.SemiBold)
                    Text("${integ.projectPath} → $boardLabel", color = c.text3, fontSize = 12.sp)
                }
                if (!integ.enabled) Text("выкл.", color = c.text3, fontSize = 11.sp)
            }
            Spacer(Modifier.height(4.dp))
            Text(
                "Синхронизация: " +
                    (integ.lastSyncedAt?.let { localDateTimeLabel(it) }.takeUnless { it.isNullOrBlank() } ?: "—"),
                color = c.text3, fontSize = 12.sp,
            )
            Spacer(Modifier.height(8.dp))
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                integ.id?.let { id ->
                    TButton("Синхр.", onClick = { vm.sync(workspaceId, id) }, kind = TButtonKind.Secondary, loading = syncing, icon = Ion.REFRESH)
                }
                if (state.isAdmin) {
                    TButton("Изменить", onClick = onEdit, kind = TButtonKind.Secondary)
                    TButton("Удалить", onClick = { confirmDelete = true }, kind = TButtonKind.Ghost)
                }
            }
        }
    }
    if (confirmDelete) {
        TConfirmDialog(
            title = "Удалить привязку",
            message = "Удалить привязку «${integ.name.ifBlank { integ.projectPath }}»? Связи задач с GitLab будут разорваны.",
            confirmText = "Удалить",
            onConfirm = {
                integ.id?.let { vm.deleteIntegration(workspaceId, it) }
                confirmDelete = false
            },
            onDismiss = { confirmDelete = false },
        )
    }
}

/** Create ([binding] == null) or edit one binding: project→board, scope, closed
 *  policy, interval, sources, write-back toggles and the label-rule editor. */
@Composable
private fun IntegrationEditor(
    binding: GitlabIntegration?,
    state: website.msdnna.tessera.ui.viewmodels.GitlabUiState,
    vm: GitlabViewModel,
    workspaceId: String,
    onClose: () -> Unit,
) {
    val c = Tessera.colors
    val integ = binding ?: GitlabIntegration()
    var name by remember(integ) { mutableStateOf(integ.name) }
    var projectPath by remember(integ) { mutableStateOf(integ.projectPath) }
    var boardId by remember(integ) { mutableStateOf(integ.boardId) }
    var enabled by remember(integ) { mutableStateOf(integ.enabled) }
    var interval by remember(integ) { mutableStateOf(integ.syncIntervalSec) }
    var dueSource by remember(integ) { mutableStateOf(integ.dueSource) }
    var startSource by remember(integ) { mutableStateOf(integ.startSource) }
    var scope by remember(integ) { mutableStateOf(integ.scope.ifBlank { "assigned" }) }
    var closedPolicy by remember(integ) { mutableStateOf(integ.closedPolicy.ifBlank { "all" }) }
    var defaultColumn by remember(integ) { mutableStateOf(integ.labelRules.defaultColumn) }
    var defaultAction by remember(integ) { mutableStateOf(integ.labelRules.defaultAction) }
    var tagKeepPrefix by remember(integ) { mutableStateOf(integ.labelRules.tagKeepPrefix) }
    var wbEnabled by remember(integ) { mutableStateOf(integ.writeback.enabled) }
    val rules = remember(integ) { mutableStateListOf<EditRule>().apply { addAll(integ.labelRules.rules.map { EditRule(it) }) } }
    // Write-back bindings: an explicit set wins; otherwise synthesize from the legacy
    // toggles (using the just-built rules for priority inversion) so a pre-bindings
    // integration opens with an equivalent, editable default set (web synthesizeBindings).
    val bindings = remember(integ) {
        mutableStateListOf<EditBinding>().apply {
            val stored = integ.writeback.bindings
            val initial = if (!stored.isNullOrEmpty()) stored else synthesizeBindings(integ.writeback, rules)
            addAll(initial.map { EditBinding(it) })
        }
    }
    // Prefill each prefix rule's friendly name from the loaded store (web GitLabModal
    // loadPrefixNames). Re-runs when the target project's names load / change.
    LaunchedEffect(state.prefixNames, rules) {
        rules.forEach { r -> if (r.matchType == "prefix") r.label = state.prefixNames[canonPrefix(r.match)] ?: "" }
    }

    Field("Название") { TTextField(name, { name = it }, placeholder = "напр. Основной") }
    Field("Проект GitLab") { TTextField(projectPath, { projectPath = it }, placeholder = "group/project") }
    Field("Доска назначения") {
        TSelect(
            value = state.boards.find { it.id == boardId }?.label ?: "Выберите доску",
            options = state.boards.map { it.id to it.label },
            onSelect = {
                boardId = it
                vm.loadColumns(it)
                vm.loadPrefixNamesForBoard(it)
            },
        )
    }
    Field("Область импорта") {
        TSelect(ScopeOptions.find { it.first == scope }?.second ?: "—", ScopeOptions) { scope = it }
    }
    Field("Закрытые задачи") {
        TSelect(ClosedPolicyOptions.find { it.first == closedPolicy }?.second ?: "—", ClosedPolicyOptions) { closedPolicy = it }
    }
    Field("Автосинхронизация") {
        TSelect(
            value = IntervalOptions.find { it.first == interval }?.second ?: "—",
            options = IntervalOptions.map { it.first.toString() to it.second },
        ) { interval = it.toInt() }
    }
    Field("Источник срока") {
        TSelect(DueSourceOptions.find { it.first == dueSource }?.second ?: "—", DueSourceOptions) { dueSource = it }
    }
    Field("Источник начала") {
        TSelect(StartSourceOptions.find { it.first == startSource }?.second ?: "—", StartSourceOptions) { startSource = it }
    }
    Field("Включена") { TSwitch(enabled, { enabled = it }) }

    // Write-back (Tessera → GitLab): customizable trigger→action bindings (web GitLabModal).
    Spacer(Modifier.height(14.dp))
    SectionLabel("Обратная запись в GitLab")
    Field("Включить запись") { TSwitch(wbEnabled, { wbEnabled = it }) }
    if (wbEnabled) {
        Spacer(Modifier.height(4.dp))
        Text(
            "Каждое действие связывает событие в задаче Tessera с действием на issue " +
                "GitLab (под сервис-токеном инстанса или токеном владельца, scope «api»). " +
                "По умолчанию набор повторяет прежнее поведение записи.",
            color = c.text3, fontSize = 12.sp,
        )
        Spacer(Modifier.height(8.dp))
        bindings.forEachIndexed { i, b ->
            BindingCard(b, state.columnOptions, onRemove = { bindings.removeAt(i) })
            Spacer(Modifier.height(8.dp))
        }
        DashedAddButton("Действие", onClick = {
            bindings.add(EditBinding(GitlabBinding(action = GitlabBindAction(type = "set_label", clearPrefix = true))))
        })
    }

    Spacer(Modifier.height(14.dp))
    SectionLabel("Правила меток")
    Field("Колонка по умолчанию") {
        TSelect(defaultColumn.ifBlank { "—" }, state.columns.map { it to it }) { defaultColumn = it }
    }
    Field("Прочие метки") {
        TSelect(DefaultActionOptions.find { it.first == defaultAction }?.second ?: "—", DefaultActionOptions) { defaultAction = it }
    }
    Field("Сохранять префикс тега") { TSwitch(tagKeepPrefix, { tagKeepPrefix = it }) }

    Spacer(Modifier.height(8.dp))
    rules.forEachIndexed { i, rule ->
        RuleCard(rule, state.columns, state.boards.map { it.id to it.label }, onRemove = { rules.removeAt(i) })
        Spacer(Modifier.height(8.dp))
    }
    DashedAddButton("Правило", onClick = { rules.add(EditRule(GitlabRule())) })

    Spacer(Modifier.height(16.dp))
    Row(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
        TButton("Отмена", onClick = onClose, kind = TButtonKind.Ghost, modifier = Modifier.weight(1f))
        TButton(
            if (binding == null) "Создать" else "Сохранить",
            onClick = {
                val bid = boardId
                if (projectPath.isNotBlank() && bid != null) {
                    val pid = state.boards.find { it.id == bid }?.projectId
                    // Prefix rules carry friendly names → merged into the project's
                    // tag-prefix store on save (canonical key, blank = remove).
                    val ruleLabels = rules.filter { it.matchType == "prefix" }
                        .associate { canonPrefix(it.match) to it.label }
                    vm.saveIntegration(
                        workspaceId, binding?.id, pid, ruleLabels,
                        GitlabIntegrationRequest(
                            name = name.trim(), projectPath = projectPath.trim(), boardId = bid,
                            enabled = enabled, syncIntervalSec = interval, dueSource = dueSource,
                            startSource = startSource, scope = scope, closedPolicy = closedPolicy,
                            closedAfter = integ.closedAfter,
                            labelRules = GitlabRules(rules.map { it.toRule() }, defaultColumn, defaultAction, tagKeepPrefix),
                            // A non-empty bindings set fully replaces the legacy flags on the
                            // backend; push_create/fetch_templates are round-tripped so a
                            // web-configured create-issue setup isn't clobbered.
                            writeback = GitlabWriteback(
                                enabled = wbEnabled,
                                pushCreate = integ.writeback.pushCreate,
                                fetchTemplates = integ.writeback.fetchTemplates,
                                bindings = if (wbEnabled) {
                                    bindings.map { b -> b.toBinding { id -> state.columnOptions.find { it.first == id }?.second ?: "" } }
                                } else {
                                    emptyList()
                                },
                            ),
                        ),
                        onDone = onClose,
                    )
                }
            },
            loading = state.saving,
            modifier = Modifier.weight(1f),
        )
    }
}

@Composable
private fun RuleCard(rule: EditRule, columns: List<String>, boards: List<Pair<String, String>>, onRemove: () -> Unit) {
    val c = Tessera.colors
    TCard {
        Column {
            Row(verticalAlignment = Alignment.CenterVertically) {
                TTextField(rule.match, { rule.match = it }, placeholder = "S: либо ^(T|C): ", modifier = Modifier.weight(1f))
                Spacer(Modifier.width(6.dp))
                IonIcon(Ion.TRASH, size = 16.dp, tint = c.text3, modifier = Modifier.clickableNoRipple(onClick = onRemove))
            }
            Spacer(Modifier.height(8.dp))
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Box(Modifier.weight(1f)) {
                    val mt = MatchTypeOptions.find { it.first == rule.matchType }?.second ?: "—"
                    TSelect(mt, MatchTypeOptions) { rule.matchType = it }
                }
                Box(Modifier.weight(1f)) {
                    val act = ActionOptions.find { it.first == rule.action }?.second ?: "—"
                    TSelect(act, ActionOptions) { rule.action = it }
                }
            }
            if (rule.matchType == "prefix") {
                Spacer(Modifier.height(8.dp))
                TTextField(rule.label, { rule.label = it }, label = "Понятное имя", placeholder = "напр. Статус")
            }
            if (rule.action == "tag") {
                Spacer(Modifier.height(8.dp))
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text("Сохранять префикс", color = c.text2, fontSize = 13.sp, modifier = Modifier.weight(1f))
                    TSwitch(rule.keepPrefix, { rule.keepPrefix = it })
                }
            }
            if (rule.action in MapActions) {
                val targets: List<Pair<String, String>> = when (rule.action) {
                    "status" -> columns.map { it to it }
                    "priority" -> PriorityLabels.mapIndexed { i, l -> i.toString() to l }
                    else -> boards
                }
                Spacer(Modifier.height(8.dp))
                rule.map.forEachIndexed { mi, m ->
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        TTextField(m.k, { m.k = it }, placeholder = "значение GitLab", modifier = Modifier.weight(1f))
                        Spacer(Modifier.width(6.dp))
                        Box(Modifier.weight(1f)) { TSelect(targets.find { it.first == m.v }?.second ?: "→", targets) { m.v = it } }
                        Spacer(Modifier.width(6.dp))
                        IonIcon(Ion.TRASH, size = 15.dp, tint = c.text3, modifier = Modifier.clickableNoRipple { rule.map.removeAt(mi) })
                    }
                    Spacer(Modifier.height(6.dp))
                }
                DashedAddButton("значение", onClick = { rule.map.add(MapEntry("", "")) })
            }
        }
    }
}

// ── small helpers ────────────────────────────────────────────────────────────

/** A dashed accent-bordered "+ add" button (web `n-button dashed type=primary`):
 *  the icon + label carry the accent gradient on a dashed accent outline. */
@Composable
private fun DashedAddButton(label: String, onClick: () -> Unit) {
    val c = Tessera.colors
    Row(
        Modifier.clip(RoundedCornerShape(RadiusSm)).dashedBorder(c.primary, RadiusSm)
            .clickableNoRipple(onClick = onClick)
            .padding(horizontal = 12.dp, vertical = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        IonIcon(Ion.ADD, size = 14.dp, tint = c.primary, gradient = true)
        Text(label, style = TextStyle(brush = accentGradient(c.primary)), fontSize = 13.sp, fontWeight = FontWeight.Medium)
    }
}

@Composable
private fun SectionLabel(text: String) {
    Text(text.uppercase(), color = Tessera.colors.text3, fontSize = 11.sp, fontWeight = FontWeight.SemiBold, letterSpacing = 0.4.sp)
    Spacer(Modifier.height(8.dp))
}

@Composable
private fun Field(label: String, content: @Composable () -> Unit) {
    Row(Modifier.fillMaxWidth().padding(vertical = 6.dp), verticalAlignment = Alignment.CenterVertically) {
        Text(label, color = Tessera.colors.text2, fontSize = 13.sp, modifier = Modifier.width(150.dp))
        Box(Modifier.weight(1f)) { content() }
    }
}

/** A dropdown "select": a bordered trigger showing [value] that opens a menu. */
@Composable
private fun TSelect(value: String, options: List<Pair<String, String>>, onSelect: (String) -> Unit) {
    val c = Tessera.colors
    var open by remember { mutableStateOf(false) }
    Box {
        Row(
            Modifier.fillMaxWidth().clip(RoundedCornerShape(RadiusSm)).background(c.surface)
                .border(1.dp, c.border, RoundedCornerShape(RadiusSm)).clickableNoRipple { open = true }
                .padding(horizontal = 10.dp, vertical = 9.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(value, color = c.text1, fontSize = 13.sp, modifier = Modifier.weight(1f))
            IonIcon(Ion.CHEVRON_DOWN, size = 13.dp, tint = c.text3)
        }
        TDropdown(expanded = open, onDismiss = { open = false }) {
            options.forEach { (k, label) ->
                TMenuItem(label, onClick = {
                    open = false
                    onSelect(k)
                })
            }
        }
    }
}

// ── editable rule model (Compose-observable) ─────────────────────────────────

private class MapEntry(k: String, v: String) {
    var k by mutableStateOf(k)
    var v by mutableStateOf(v)
}

private class EditRule(rule: GitlabRule) {
    var match by mutableStateOf(rule.match)
    var matchType by mutableStateOf(rule.matchType.ifBlank { "prefix" })
    var action by mutableStateOf(rule.action.ifBlank { "tag" })
    var keepPrefix by mutableStateOf(rule.keepPrefix)

    /** Friendly display name for a prefix rule. Not part of [GitlabRule] — it lives
     *  in the project's tag-prefix store; prefilled from / merged into it on save. */
    var label by mutableStateOf("")
    val map = mutableStateListOf<MapEntry>().apply {
        addAll((rule.valueMap ?: emptyMap()).map { MapEntry(it.key, it.value) })
    }

    fun toRule(): GitlabRule {
        val vm = if (action in MapActions) {
            map.filter { it.k.isNotBlank() && it.v.isNotBlank() }.associate { it.k to it.v }
        } else {
            null
        }
        return GitlabRule(match, matchType, action, vm, keepPrefix)
    }
}

// ── write-back binding card + editable model ─────────────────────────────────

/** One write-back binding card: an enable toggle + summary, a trigger selector with
 *  a type-specific qualifier, and a GitLab action selector with its parameters.
 *  Mirrors the web GitLabModal binding cards; edited inline like [RuleCard]. */
@Composable
private fun BindingCard(b: EditBinding, columnOptions: List<Pair<String, String>>, onRemove: () -> Unit) {
    val c = Tessera.colors
    val columnName = columnOptions.find { it.first == b.columnId }?.second ?: b.columnName
    TCard {
        Column {
            Row(verticalAlignment = Alignment.CenterVertically) {
                TSwitch(b.enabled, { b.enabled = it })
                Spacer(Modifier.width(8.dp))
                Row(Modifier.weight(1f), verticalAlignment = Alignment.CenterVertically) {
                    Text("${triggerSummary(b, columnName)} → ", color = c.text2, fontSize = 12.sp)
                    Text(
                        actionSummary(b),
                        style = TextStyle(brush = accentGradient(c.primary)),
                        fontSize = 12.sp, fontWeight = FontWeight.Medium,
                    )
                }
                IonIcon(Ion.TRASH, size = 16.dp, tint = c.text3, modifier = Modifier.clickableNoRipple(onClick = onRemove))
            }
            Spacer(Modifier.height(6.dp))
            Field("Действие в Tessera") {
                TSelect(TriggerTypeOptions.find { it.first == b.triggerType }?.second ?: "—", TriggerTypeOptions) { b.onTrigger(it) }
            }
            when (b.triggerType) {
                "column" -> Field("Целевая колонка") {
                    TSelect(columnName.ifBlank { "Выберите колонку" }, columnOptions) { b.columnId = it }
                }

                "priority" -> Field("Приоритет") {
                    TSelect(
                        PriorityQualOptions.find { it.first == (b.priority?.toString() ?: "") }?.second ?: "—",
                        PriorityQualOptions,
                    ) { b.priority = it.toIntOrNull() }
                }

                "completion" -> Field("Условие") {
                    TSelect(CompletionOptions.find { it.first == completedKey(b.completed) }?.second ?: "—", CompletionOptions) {
                        b.completed = when (it) {
                            "true" -> true
                            "false" -> false
                            else -> null
                        }
                    }
                }

                "due" -> Field("Тип срока") {
                    TSelect(DateKindOptions.find { it.first == b.triggerDateKind }?.second ?: "—", DateKindOptions) { b.triggerDateKind = it }
                }
            }
            Field("Действие в GitLab") {
                TSelect(ActionTypeOptions.find { it.first == b.actionType }?.second ?: "—", ActionTypeOptions) { b.onAction(it) }
            }
            when (b.actionType) {
                "set_label" -> {
                    Field("Метка") { TTextField(b.label, { b.label = it }, placeholder = "напр. S: In Progress") }
                    Field("Снимать метки того же префикса") { TSwitch(b.clearPrefix, { b.clearPrefix = it }) }
                }

                "set_state" -> Field("Состояние issue") {
                    TSelect(StateOptions.find { it.first == b.state }?.second ?: "—", StateOptions) { b.state = it }
                }

                "set_due" -> Field("Тип срока") {
                    TSelect(DateKindOptions.find { it.first == b.actionDateKind }?.second ?: "—", DateKindOptions) { b.actionDateKind = it }
                }

                "post_comment" -> Field("Добавлять маркер Tessera") { TSwitch(b.addMarker, { b.addMarker = it }) }
            }
        }
    }
}

private fun completedKey(v: Boolean?): String = when (v) {
    true -> "true"
    false -> "false"
    null -> ""
}

private fun triggerSummary(b: EditBinding, columnName: String): String = when (b.triggerType) {
    "column" -> "Перенос → «${columnName.ifBlank { "?" }}»"

    "priority" -> b.priority?.let { "Приоритет: ${PriorityLabels.getOrElse(it) { "?" }}" } ?: "Приоритет (любой)"

    "completion" -> when (b.completed) {
        null -> "Флаг «Выполнено»"
        true -> "Стало «Выполнено»"
        false -> "Снято «Выполнено»"
    }

    "due" -> if (b.triggerDateKind == "start") "Изменение начала" else "Изменение срока"

    else -> TriggerTypeOptions.find { it.first == b.triggerType }?.second ?: b.triggerType
}

private fun actionSummary(b: EditBinding): String = when (b.actionType) {
    "set_label" -> "метка «${b.label.ifBlank { "?" }}»"

    "set_state" -> when (b.state) {
        "closed" -> "закрыть issue"
        "opened" -> "открыть issue"
        else -> "закрыть/открыть issue"
    }

    "post_comment" -> if (b.addMarker) "комментарий (+маркер)" else "комментарий"

    else -> (ActionTypeOptions.find { it.first == b.actionType }?.second ?: b.actionType).lowercase()
}

/** Compose-observable editable binding (like [EditRule]). [toBinding] strips it to
 *  the wire shape (only the fields relevant to the picked trigger/action). */
private class EditBinding(b: GitlabBinding) {
    var enabled by mutableStateOf(b.enabled)
    var triggerType by mutableStateOf(b.trigger.type.ifBlank { "column" })
    var columnId by mutableStateOf(b.trigger.columnId ?: "")
    var columnName by mutableStateOf(b.trigger.columnName ?: "")
    var priority by mutableStateOf(b.trigger.priority)
    var completed by mutableStateOf(b.trigger.completed)
    var triggerDateKind by mutableStateOf(b.trigger.dateKind ?: if (b.trigger.type == "due") "due" else "")
    var actionType by mutableStateOf(b.action.type.ifBlank { "set_label" })
    var label by mutableStateOf(b.action.label ?: "")
    var clearPrefix by mutableStateOf(b.action.clearPrefix)
    var state by mutableStateOf(b.action.state ?: "")
    var actionDateKind by mutableStateOf(b.action.dateKind ?: if (b.action.type == "set_due") "due" else "")
    var addMarker by mutableStateOf(b.action.addMarker)

    /** Switch the trigger type: reset now-irrelevant qualifiers, pick the default action. */
    fun onTrigger(type: String) {
        triggerType = type
        columnId = ""
        columnName = ""
        priority = null
        completed = null
        triggerDateKind = if (type == "due") "due" else ""
        onAction(DefaultActionForTrigger[type] ?: "set_label")
    }

    fun onAction(type: String) {
        actionType = type
        if (type == "set_label" && label.isBlank()) clearPrefix = true
        if (type == "set_due" && actionDateKind.isBlank()) actionDateKind = "due"
    }

    fun toBinding(columnNameById: (String) -> String): GitlabBinding {
        val t = when (triggerType) {
            "column" -> GitlabBindTrigger(
                type = "column", columnId = columnId,
                columnName = columnNameById(columnId).ifBlank { columnName },
            )

            "priority" -> GitlabBindTrigger(type = "priority", priority = priority)

            "completion" -> GitlabBindTrigger(type = "completion", completed = completed)

            "due" -> GitlabBindTrigger(type = "due", dateKind = triggerDateKind.ifBlank { "due" })

            else -> GitlabBindTrigger(type = triggerType)
        }
        val a = when (actionType) {
            "set_label" -> GitlabBindAction(type = "set_label", label = label.trim(), clearPrefix = clearPrefix)
            "set_state" -> GitlabBindAction(type = "set_state", state = state)
            "set_due" -> GitlabBindAction(type = "set_due", dateKind = actionDateKind.ifBlank { "due" })
            "post_comment" -> GitlabBindAction(type = "post_comment", addMarker = addMarker)
            else -> GitlabBindAction(type = actionType)
        }
        return GitlabBinding(enabled, t, a)
    }
}

/** Synthesize bindings from the legacy write-back flags so a pre-bindings integration
 *  opens with an equivalent, editable set. Mirrors web GitLabModal synthesizeBindings
 *  (and the backend `effectiveBindings`): priority fans out to one per-level set_label
 *  from the priority rule's inverted value map. */
private fun synthesizeBindings(wb: GitlabWriteback, rules: List<EditRule>): List<GitlabBinding> {
    if (!wb.enabled) return emptyList()
    val out = mutableListOf<GitlabBinding>()
    fun add(t: GitlabBindTrigger, a: GitlabBindAction) = out.add(GitlabBinding(true, t, a))

    if (wb.pushState) add(GitlabBindTrigger(type = "completion"), GitlabBindAction(type = "set_state", state = ""))
    if (wb.pushPriority) {
        val pr = rules.firstOrNull { it.action == "priority" && it.matchType == "prefix" }
        if (pr != null) {
            val byLevel = LinkedHashMap<String, String>() // level → gitlab value
            var ambiguous = false
            pr.map.forEach { m ->
                if (m.v.isBlank()) return@forEach
                if (byLevel.containsKey(m.v)) ambiguous = true
                byLevel[m.v] = m.k
            }
            if (!ambiguous) {
                byLevel.entries.sortedBy { it.key.toIntOrNull() ?: 0 }.forEach { (lvl, value) ->
                    add(
                        GitlabBindTrigger(type = "priority", priority = lvl.toIntOrNull()),
                        GitlabBindAction(type = "set_label", label = pr.match + value, clearPrefix = true),
                    )
                }
            }
        }
    }
    if (wb.pushComments) add(GitlabBindTrigger(type = "comment"), GitlabBindAction(type = "post_comment"))
    if (wb.pushLabels) add(GitlabBindTrigger(type = "labels"), GitlabBindAction(type = "reconcile_labels"))
    if (wb.pushDue) add(GitlabBindTrigger(type = "due", dateKind = "due"), GitlabBindAction(type = "set_due", dateKind = "due"))
    if (wb.pushAssignees) add(GitlabBindTrigger(type = "assignees"), GitlabBindAction(type = "set_assignees"))
    if (wb.pushEstimate) add(GitlabBindTrigger(type = "estimate"), GitlabBindAction(type = "set_estimate"))
    return out
}
