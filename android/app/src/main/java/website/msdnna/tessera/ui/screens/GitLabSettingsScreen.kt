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
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import website.msdnna.tessera.data.model.GitlabRule
import website.msdnna.tessera.data.model.GitlabRules
import website.msdnna.tessera.data.model.GitlabSetIntegrationRequest
import website.msdnna.tessera.ui.components.IonIcon
import website.msdnna.tessera.ui.components.TButton
import website.msdnna.tessera.ui.components.TButtonKind
import website.msdnna.tessera.ui.components.TCard
import website.msdnna.tessera.ui.components.TDropdown
import website.msdnna.tessera.ui.components.TFormError
import website.msdnna.tessera.ui.components.TMenuItem
import website.msdnna.tessera.ui.components.TSwitch
import website.msdnna.tessera.ui.components.TTextField
import website.msdnna.tessera.ui.components.TesseraLoader
import website.msdnna.tessera.ui.components.clickableNoRipple
import website.msdnna.tessera.ui.theme.PriorityLabels
import website.msdnna.tessera.ui.theme.RadiusMd
import website.msdnna.tessera.ui.theme.RadiusSm
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.viewmodels.GitlabViewModel
import website.msdnna.tessera.util.Ion
import website.msdnna.tessera.util.shortDate

private val IntervalOptions = listOf(
    0 to "Вручную (выкл.)", 300 to "Каждые 5 минут", 900 to "Каждые 15 минут", 3600 to "Каждый час",
)
private val DueSourceOptions = listOf(
    "issue_milestone" to "Issue, иначе Milestone", "issue" to "Только Issue",
    "milestone" to "Только Milestone", "off" to "Не синхронизировать",
)
private val ActionOptions = listOf(
    "status" to "Статус → колонка", "priority" to "Приоритет", "board" to "Доска",
    "tag" to "Тег", "group" to "Группировка", "ignore" to "Игнорировать",
)
private val MatchTypeOptions = listOf("prefix" to "Префикс", "regex" to "Regex")
private val DefaultActionOptions = listOf("tag" to "Создавать тег", "ignore" to "Игнорировать")
private val MapActions = setOf("status", "priority", "board")

/** GitLab settings: connect a PAT account, configure the per-workspace
 *  integration (project → board, interval, due source) with a generic label
 *  rule editor, and run a manual sync. Mirrors the web `GitLabModal`. */
@Composable
fun GitLabSettingsScreen(workspaceId: String, vm: GitlabViewModel = viewModel(key = "gitlab")) {
    val c = Tessera.colors
    val state by vm.state.collectAsStateWithLifecycle()
    LaunchedEffect(workspaceId) { vm.loadAll(workspaceId) }

    if (state.loading) {
        Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) { TesseraLoader() }
        return
    }
    Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(16.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            IonIcon(Ion.GIT_BRANCH, size = 20.dp, tint = c.primary, gradient = true)
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

        if (state.connected && state.integration != null) {
            Spacer(Modifier.height(16.dp))
            IntegrationCard(state, vm, workspaceId)
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

@Composable
private fun IntegrationCard(state: website.msdnna.tessera.ui.viewmodels.GitlabUiState, vm: GitlabViewModel, workspaceId: String) {
    val c = Tessera.colors
    val integ = state.integration!!
    var projectPath by remember(integ) { mutableStateOf(integ.projectPath) }
    var boardId by remember(integ) { mutableStateOf(integ.boardId) }
    var enabled by remember(integ) { mutableStateOf(integ.enabled) }
    var interval by remember(integ) { mutableStateOf(integ.syncIntervalSec) }
    var dueSource by remember(integ) { mutableStateOf(integ.dueSource) }
    var defaultColumn by remember(integ) { mutableStateOf(integ.labelRules.defaultColumn) }
    var defaultAction by remember(integ) { mutableStateOf(integ.labelRules.defaultAction) }
    var tagKeepPrefix by remember(integ) { mutableStateOf(integ.labelRules.tagKeepPrefix) }
    val rules = remember(integ) { mutableStateListOf<EditRule>().apply { addAll(integ.labelRules.rules.map { EditRule(it) }) } }

    SectionLabel("Интеграция пространства")
    Field("Проект GitLab") { TTextField(projectPath, { projectPath = it }, placeholder = "group/project") }
    Field("Доска назначения") {
        TSelect(
            value = state.boards.find { it.id == boardId }?.label ?: "Выберите доску",
            options = state.boards.map { it.id to it.label },
            onSelect = {
                boardId = it
                vm.loadColumns(it)
            },
        )
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
    Field("Включена") { TSwitch(enabled, { enabled = it }) }

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
    TButton("+ Правило", onClick = { rules.add(EditRule(GitlabRule())) }, kind = TButtonKind.Secondary)

    Spacer(Modifier.height(16.dp))
    Text(
        "Последняя синхронизация: " + (integ.lastSyncedAt?.let { shortDate(it) }.takeUnless { it.isNullOrBlank() } ?: "—"),
        color = c.text3, fontSize = 12.sp,
    )
    Spacer(Modifier.height(10.dp))
    Row(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
        TButton("Синхронизировать", onClick = { vm.sync(workspaceId) }, kind = TButtonKind.Secondary, loading = state.syncing, modifier = Modifier.weight(1f))
        TButton(
            "Сохранить",
            onClick = {
                val bid = boardId
                if (projectPath.isNotBlank() && bid != null) {
                    vm.save(
                        workspaceId,
                        GitlabSetIntegrationRequest(
                            projectPath.trim(), bid, enabled, interval, dueSource,
                            GitlabRules(rules.map { it.toRule() }, defaultColumn, defaultAction, tagKeepPrefix),
                        ),
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
                TButton("+ значение", onClick = { rule.map.add(MapEntry("", "")) }, kind = TButtonKind.Secondary)
            }
        }
    }
}

// ── small helpers ────────────────────────────────────────────────────────────

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
