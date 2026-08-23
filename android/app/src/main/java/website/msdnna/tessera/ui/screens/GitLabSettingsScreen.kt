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
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.CircularProgressIndicator
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
import androidx.compose.ui.platform.LocalResources
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import website.msdnna.tessera.R
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
import website.msdnna.tessera.ui.resolve
import website.msdnna.tessera.ui.theme.ConflictAmber
import website.msdnna.tessera.ui.theme.PriorityLabels
import website.msdnna.tessera.ui.theme.RadiusMd
import website.msdnna.tessera.ui.theme.RadiusSm
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.TesseraDanger
import website.msdnna.tessera.ui.theme.accentGradient
import website.msdnna.tessera.ui.viewmodels.GitlabViewModel
import website.msdnna.tessera.util.Ion
import website.msdnna.tessera.util.canonPrefix
import website.msdnna.tessera.util.localDateTimeLabel

private val MapActions = setOf("status", "priority", "board")

// ── подписи вариантов ────────────────────────────────────────────────────────
// Все списки собираются на вызов, а не в module-level `val`: значения там
// вычислились бы один раз при загрузке класса и застыли бы на языке первого
// рендера — переключение языка эти меню уже не тронуло бы.

@Composable
private fun intervalOptions(): List<Pair<Int, String>> = listOf(
    0 to stringResource(R.string.gitlab_interval_off),
    300 to stringResource(R.string.gitlab_interval_5m),
    900 to stringResource(R.string.gitlab_interval_15m),
    3600 to stringResource(R.string.gitlab_interval_1h),
)

// Periodic FULL sweep (catches deletes/drift an incremental pull can't see). 0 = off
// — a full sync then runs only on the very first sync or via the «Полная» tile.
@Composable
private fun fullIntervalOptions(): List<Pair<Int, String>> = listOf(
    0 to stringResource(R.string.gitlab_full_off),
    21600 to stringResource(R.string.gitlab_full_6h),
    43200 to stringResource(R.string.gitlab_full_12h),
    86400 to stringResource(R.string.gitlab_full_1d),
    172800 to stringResource(R.string.gitlab_full_2d),
    604800 to stringResource(R.string.gitlab_full_1w),
)

@Composable
private fun dueSourceOptions(): List<Pair<String, String>> = listOf(
    "issue_milestone" to stringResource(R.string.gitlab_due_issue_milestone),
    "issue" to stringResource(R.string.gitlab_due_issue),
    "milestone" to stringResource(R.string.gitlab_due_milestone),
    "off" to stringResource(R.string.gitlab_source_off),
)

@Composable
private fun startSourceOptions(): List<Pair<String, String>> = listOf(
    "created" to stringResource(R.string.gitlab_start_created),
    "milestone" to stringResource(R.string.gitlab_start_milestone),
    "off" to stringResource(R.string.gitlab_source_off),
)

@Composable
private fun actionOptions(): List<Pair<String, String>> = listOf(
    "status" to stringResource(R.string.gitlab_rule_action_status),
    "priority" to stringResource(R.string.gitlab_rule_action_priority),
    "board" to stringResource(R.string.gitlab_rule_action_board),
    "tag" to stringResource(R.string.gitlab_rule_action_tag),
    "group" to stringResource(R.string.gitlab_rule_action_group),
    "ignore" to stringResource(R.string.gitlab_rule_action_ignore),
)

@Composable
private fun matchTypeOptions(): List<Pair<String, String>> = listOf(
    "prefix" to stringResource(R.string.gitlab_match_prefix),
    "regex" to stringResource(R.string.gitlab_match_regex),
)

@Composable
private fun defaultActionOptions(): List<Pair<String, String>> = listOf(
    "tag" to stringResource(R.string.gitlab_default_action_tag),
    "ignore" to stringResource(R.string.gitlab_rule_action_ignore),
)

@Composable
private fun scopeOptions(): List<Pair<String, String>> = listOf(
    "assigned" to stringResource(R.string.gitlab_scope_assigned),
    "all" to stringResource(R.string.gitlab_scope_all),
)

@Composable
private fun closedPolicyOptions(): List<Pair<String, String>> = listOf(
    "all" to stringResource(R.string.gitlab_closed_all),
    "archive_closed_sprints" to stringResource(R.string.gitlab_closed_archive),
    "period" to stringResource(R.string.gitlab_closed_period),
)

// ── write-back binding option lists (mirror web GitLabModal) ─────────────────
@Composable
private fun triggerTypeOptions(): List<Pair<String, String>> = listOf(
    "column" to stringResource(R.string.gitlab_trigger_column),
    "completion" to stringResource(R.string.gitlab_trigger_completion),
    "priority" to stringResource(R.string.gitlab_trigger_priority),
    "due" to stringResource(R.string.gitlab_trigger_due),
    "assignees" to stringResource(R.string.gitlab_trigger_assignees),
    "estimate" to stringResource(R.string.gitlab_trigger_estimate),
    "milestone" to stringResource(R.string.gitlab_trigger_milestone),
    "title_desc" to stringResource(R.string.gitlab_trigger_title_desc),
    "labels" to stringResource(R.string.gitlab_trigger_labels),
    "comment" to stringResource(R.string.gitlab_trigger_comment),
)

@Composable
private fun actionTypeOptions(): List<Pair<String, String>> = listOf(
    "set_label" to stringResource(R.string.gitlab_action_set_label),
    "set_state" to stringResource(R.string.gitlab_action_set_state),
    "set_due" to stringResource(R.string.gitlab_action_set_due),
    "set_assignees" to stringResource(R.string.gitlab_action_set_assignees),
    "set_estimate" to stringResource(R.string.gitlab_action_set_estimate),
    "set_milestone" to stringResource(R.string.gitlab_action_set_milestone),
    "set_title_desc" to stringResource(R.string.gitlab_action_set_title_desc),
    "reconcile_labels" to stringResource(R.string.gitlab_action_reconcile_labels),
    "post_comment" to stringResource(R.string.gitlab_action_post_comment),
)

@Composable
private fun stateOptions(): List<Pair<String, String>> = listOf(
    "" to stringResource(R.string.gitlab_state_from_flag),
    "closed" to stringResource(R.string.gitlab_state_closed),
    "opened" to stringResource(R.string.gitlab_state_opened),
)

@Composable
private fun dateKindOptions(): List<Pair<String, String>> = listOf(
    "due" to stringResource(R.string.gitlab_date_due),
    "start" to stringResource(R.string.gitlab_date_start),
)

// completion qualifier: "" = any change, "true"/"false" = became/cleared done.
@Composable
private fun completionOptions(): List<Pair<String, String>> = listOf(
    "" to stringResource(R.string.gitlab_completion_any),
    "true" to stringResource(R.string.gitlab_completion_true),
    "false" to stringResource(R.string.gitlab_completion_false),
)

// priority qualifier: "" = any level, "0".."4" = a specific level.
@Composable
private fun priorityQualOptions(): List<Pair<String, String>> =
    listOf("" to stringResource(R.string.gitlab_priority_any)) +
        PriorityLabels.mapIndexed { i, l -> i.toString() to l }

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
            Text(it.resolve(), color = c.primary, fontSize = 13.sp)
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
                    stringResource(R.string.gitlab_conflicts, conflictCount),
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
    SectionLabel(stringResource(R.string.gitlab_section_account))
    TCard {
        if (state.connected) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Column(Modifier.weight(1f)) {
                    Text("@${state.glUsername}", color = c.text1, fontSize = 15.sp, fontWeight = FontWeight.SemiBold)
                    Text(state.baseUrl, color = c.text3, fontSize = 12.sp)
                }
                TButton(stringResource(R.string.gitlab_disconnect), onClick = { vm.disconnect() }, kind = TButtonKind.Secondary)
            }
        } else {
            var base by remember { mutableStateOf("") }
            var token by remember { mutableStateOf("") }
            Column {
                TTextField(base, { base = it }, label = stringResource(R.string.gitlab_url_label), placeholder = "https://gitlab.example.com")
                Spacer(Modifier.height(10.dp))
                TTextField(
                    token, { token = it },
                    label = stringResource(R.string.gitlab_token_label),
                    placeholder = "glpat-…", isPassword = true,
                )
                Spacer(Modifier.height(12.dp))
                TButton(
                    stringResource(R.string.gitlab_connect),
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

    SectionLabel(stringResource(R.string.gitlab_section_bindings))
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
        Text(stringResource(R.string.gitlab_bindings_empty), color = c.text3, fontSize = 13.sp)
        Spacer(Modifier.height(8.dp))
    } else {
        state.integrations.forEach { integ ->
            BindingRow(integ, state, vm, workspaceId, onEdit = { openEditor(integ) })
            Spacer(Modifier.height(8.dp))
        }
    }
    if (state.isAdmin) {
        DashedAddButton(stringResource(R.string.gitlab_add_binding), onClick = { openEditor(null) })
    } else if (!state.serviceConfigured && !state.connected) {
        Text(stringResource(R.string.gitlab_connect_hint), color = c.text3, fontSize = 12.sp)
    }
    Spacer(Modifier.height(12.dp))
    TButton(
        stringResource(R.string.gitlab_journal),
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
                if (!integ.enabled) Text(stringResource(R.string.gitlab_binding_off), color = c.text3, fontSize = 11.sp)
            }
            Spacer(Modifier.height(4.dp))
            Text(
                stringResource(
                    R.string.gitlab_last_sync,
                    integ.lastSyncedAt?.let { localDateTimeLabel(LocalResources.current, it) }
                        .takeUnless { it.isNullOrBlank() } ?: "—",
                ),
                color = c.text3, fontSize = 12.sp,
            )
            Spacer(Modifier.height(8.dp))
            // Icon-over-caption tiles, not text buttons: four labelled buttons don't
            // fit a phone card side by side — «Удалить» wrapped one letter per line.
            Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                integ.id?.let { id ->
                    ActionTile(
                        stringResource(R.string.gitlab_tile_sync), Ion.REFRESH, Modifier.weight(1f),
                        onClick = { vm.sync(workspaceId, id) },
                        loading = syncing && !state.syncingFull, enabled = !syncing,
                    )
                    // Full sweep: also re-checks issues an incremental pull skips, so
                    // deletes and drift in GitLab reach the board.
                    ActionTile(
                        stringResource(R.string.gitlab_tile_full), Ion.REPEAT, Modifier.weight(1f),
                        onClick = { vm.sync(workspaceId, id, full = true) },
                        loading = syncing && state.syncingFull, enabled = !syncing,
                    )
                }
                if (state.isAdmin) {
                    ActionTile(stringResource(R.string.common_edit), Ion.PENCIL, Modifier.weight(1f), onClick = onEdit)
                    ActionTile(
                        stringResource(R.string.common_delete), Ion.TRASH, Modifier.weight(1f),
                        onClick = { confirmDelete = true }, danger = true,
                    )
                }
            }
        }
    }
    if (confirmDelete) {
        TConfirmDialog(
            title = stringResource(R.string.gitlab_binding_delete_title),
            message = stringResource(R.string.gitlab_binding_delete_message, integ.name.ifBlank { integ.projectPath }),
            confirmText = stringResource(R.string.common_delete),
            onConfirm = {
                integ.id?.let { vm.deleteIntegration(workspaceId, it) }
                confirmDelete = false
            },
            onDismiss = { confirmDelete = false },
        )
    }
}

/**
 * A compact card action: a glyph over a small caption, sharing the row width with
 * its siblings (same shape as the board-layout selector tiles). Four of these fit
 * a phone card where four text buttons wrap; [danger] tints the destructive one,
 * [loading] swaps the glyph for a spinner in place.
 */
@Composable
private fun ActionTile(
    label: String,
    icon: String,
    modifier: Modifier = Modifier,
    onClick: () -> Unit,
    enabled: Boolean = true,
    loading: Boolean = false,
    danger: Boolean = false,
) {
    val c = Tessera.colors
    val active = enabled && !loading
    val fg = (if (danger) TesseraDanger else c.text1).copy(alpha = if (active) 1f else 0.45f)
    Column(
        modifier
            .clip(RoundedCornerShape(RadiusSm))
            .background(c.surfaceAlt)
            .clickableNoRipple(enabled = active, onClick = onClick)
            .padding(top = 9.dp, bottom = 6.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        if (loading) {
            CircularProgressIndicator(Modifier.size(18.dp), strokeWidth = 2.dp, color = fg)
        } else {
            IonIcon(icon, size = 18.dp, tint = fg)
        }
        Spacer(Modifier.height(5.dp))
        Text(label, color = fg, fontSize = 10.sp, fontWeight = FontWeight.Medium, maxLines = 1, softWrap = false)
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
    var fullInterval by remember(integ) { mutableStateOf(integ.fullSyncIntervalSec) }
    // relations_sync is off|pull on the wire, so the switch serialises to those two.
    var relationsSync by remember(integ) { mutableStateOf(integ.relationsSync != "off") }
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

    Field(stringResource(R.string.gitlab_field_name)) {
        TTextField(name, { name = it }, placeholder = stringResource(R.string.gitlab_name_placeholder))
    }
    Field(stringResource(R.string.gitlab_field_project)) {
        TTextField(projectPath, { projectPath = it }, placeholder = "group/project")
    }
    Field(stringResource(R.string.gitlab_field_board)) {
        TSelect(
            value = state.boards.find { it.id == boardId }?.label ?: stringResource(R.string.gitlab_board_placeholder),
            options = state.boards.map { it.id to it.label },
            onSelect = {
                boardId = it
                vm.loadColumns(it)
                vm.loadPrefixNamesForBoard(it)
            },
        )
    }
    Field(stringResource(R.string.gitlab_field_scope)) {
        val opts = scopeOptions()
        TSelect(opts.find { it.first == scope }?.second ?: "—", opts) { scope = it }
    }
    Field(stringResource(R.string.gitlab_field_closed)) {
        val opts = closedPolicyOptions()
        TSelect(opts.find { it.first == closedPolicy }?.second ?: "—", opts) { closedPolicy = it }
    }
    Field(stringResource(R.string.gitlab_field_interval)) {
        val opts = intervalOptions()
        TSelect(
            value = opts.find { it.first == interval }?.second ?: "—",
            options = opts.map { it.first.toString() to it.second },
        ) { interval = it.toInt() }
    }
    Field(stringResource(R.string.gitlab_field_full_interval)) {
        val opts = fullIntervalOptions()
        TSelect(
            value = opts.find { it.first == fullInterval }?.second ?: "—",
            options = opts.map { it.first.toString() to it.second },
        ) { fullInterval = it.toInt() }
    }
    Field(stringResource(R.string.gitlab_field_due_source)) {
        val opts = dueSourceOptions()
        TSelect(opts.find { it.first == dueSource }?.second ?: "—", opts) { dueSource = it }
    }
    Field(stringResource(R.string.gitlab_field_start_source)) {
        val opts = startSourceOptions()
        TSelect(opts.find { it.first == startSource }?.second ?: "—", opts) { startSource = it }
    }
    Field(stringResource(R.string.gitlab_field_relations)) { TSwitch(relationsSync, { relationsSync = it }) }
    Text(
        stringResource(R.string.gitlab_relations_hint),
        color = c.text3, fontSize = 12.sp,
    )
    Spacer(Modifier.height(8.dp))
    Field(stringResource(R.string.gitlab_field_enabled)) { TSwitch(enabled, { enabled = it }) }

    // Write-back (Tessera → GitLab): customizable trigger→action bindings (web GitLabModal).
    Spacer(Modifier.height(14.dp))
    SectionLabel(stringResource(R.string.gitlab_section_writeback))
    Field(stringResource(R.string.gitlab_field_writeback_enabled)) { TSwitch(wbEnabled, { wbEnabled = it }) }
    if (wbEnabled) {
        Spacer(Modifier.height(4.dp))
        Text(
            stringResource(R.string.gitlab_writeback_hint),
            color = c.text3, fontSize = 12.sp,
        )
        Spacer(Modifier.height(8.dp))
        bindings.forEachIndexed { i, b ->
            BindingCard(b, state.columnOptions, onRemove = { bindings.removeAt(i) })
            Spacer(Modifier.height(8.dp))
        }
        DashedAddButton(stringResource(R.string.gitlab_add_action), onClick = {
            bindings.add(EditBinding(GitlabBinding(action = GitlabBindAction(type = "set_label", clearPrefix = true))))
        })
    }

    Spacer(Modifier.height(14.dp))
    SectionLabel(stringResource(R.string.gitlab_section_rules))
    Field(stringResource(R.string.gitlab_field_default_column)) {
        TSelect(defaultColumn.ifBlank { "—" }, state.columns.map { it to it }) { defaultColumn = it }
    }
    Field(stringResource(R.string.gitlab_field_default_action)) {
        val opts = defaultActionOptions()
        TSelect(opts.find { it.first == defaultAction }?.second ?: "—", opts) { defaultAction = it }
    }
    Field(stringResource(R.string.gitlab_field_tag_keep_prefix)) { TSwitch(tagKeepPrefix, { tagKeepPrefix = it }) }

    Spacer(Modifier.height(8.dp))
    rules.forEachIndexed { i, rule ->
        RuleCard(rule, state.columns, state.boards.map { it.id to it.label }, onRemove = { rules.removeAt(i) })
        Spacer(Modifier.height(8.dp))
    }
    DashedAddButton(stringResource(R.string.gitlab_add_rule), onClick = { rules.add(EditRule(GitlabRule())) })

    Spacer(Modifier.height(16.dp))
    Row(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
        TButton(stringResource(R.string.common_cancel), onClick = onClose, kind = TButtonKind.Ghost, modifier = Modifier.weight(1f))
        TButton(
            stringResource(if (binding == null) R.string.common_create else R.string.common_save),
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
                            enabled = enabled, syncIntervalSec = interval,
                            fullSyncIntervalSec = fullInterval,
                            relationsSync = if (relationsSync) "pull" else "off",
                            dueSource = dueSource,
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
                TTextField(
                    rule.match, { rule.match = it },
                    placeholder = stringResource(R.string.gitlab_rule_match_placeholder),
                    modifier = Modifier.weight(1f),
                )
                Spacer(Modifier.width(6.dp))
                IonIcon(Ion.TRASH, size = 16.dp, tint = c.text3, modifier = Modifier.clickableNoRipple(onClick = onRemove))
            }
            Spacer(Modifier.height(8.dp))
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Box(Modifier.weight(1f)) {
                    val matchTypes = matchTypeOptions()
                    val mt = matchTypes.find { it.first == rule.matchType }?.second ?: "—"
                    TSelect(mt, matchTypes) { rule.matchType = it }
                }
                Box(Modifier.weight(1f)) {
                    val actions = actionOptions()
                    val act = actions.find { it.first == rule.action }?.second ?: "—"
                    TSelect(act, actions) { rule.action = it }
                }
            }
            if (rule.matchType == "prefix") {
                Spacer(Modifier.height(8.dp))
                TTextField(
                    rule.label, { rule.label = it },
                    label = stringResource(R.string.gitlab_rule_label),
                    placeholder = stringResource(R.string.gitlab_rule_label_placeholder),
                )
            }
            if (rule.action == "tag") {
                Spacer(Modifier.height(8.dp))
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text(stringResource(R.string.gitlab_rule_keep_prefix), color = c.text2, fontSize = 13.sp, modifier = Modifier.weight(1f))
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
                        TTextField(
                            m.k, { m.k = it },
                            placeholder = stringResource(R.string.gitlab_rule_map_value),
                            modifier = Modifier.weight(1f),
                        )
                        Spacer(Modifier.width(6.dp))
                        Box(Modifier.weight(1f)) { TSelect(targets.find { it.first == m.v }?.second ?: "→", targets) { m.v = it } }
                        Spacer(Modifier.width(6.dp))
                        IonIcon(Ion.TRASH, size = 15.dp, tint = c.text3, modifier = Modifier.clickableNoRipple { rule.map.removeAt(mi) })
                    }
                    Spacer(Modifier.height(6.dp))
                }
                DashedAddButton(stringResource(R.string.gitlab_rule_add_value), onClick = { rule.map.add(MapEntry("", "")) })
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
            Field(stringResource(R.string.gitlab_bind_trigger)) {
                val triggers = triggerTypeOptions()
                TSelect(triggers.find { it.first == b.triggerType }?.second ?: "—", triggers) { b.onTrigger(it) }
            }
            when (b.triggerType) {
                "column" -> Field(stringResource(R.string.gitlab_bind_column)) {
                    TSelect(columnName.ifBlank { stringResource(R.string.gitlab_bind_column_placeholder) }, columnOptions) {
                        b.columnId = it
                    }
                }

                "priority" -> Field(stringResource(R.string.gitlab_bind_priority)) {
                    val quals = priorityQualOptions()
                    TSelect(
                        quals.find { it.first == (b.priority?.toString() ?: "") }?.second ?: "—",
                        quals,
                    ) { b.priority = it.toIntOrNull() }
                }

                "completion" -> Field(stringResource(R.string.gitlab_bind_condition)) {
                    val conditions = completionOptions()
                    TSelect(conditions.find { it.first == completedKey(b.completed) }?.second ?: "—", conditions) {
                        b.completed = when (it) {
                            "true" -> true
                            "false" -> false
                            else -> null
                        }
                    }
                }

                "due" -> Field(stringResource(R.string.gitlab_bind_date_kind)) {
                    val kinds = dateKindOptions()
                    TSelect(kinds.find { it.first == b.triggerDateKind }?.second ?: "—", kinds) { b.triggerDateKind = it }
                }
            }
            Field(stringResource(R.string.gitlab_bind_action)) {
                val actions = actionTypeOptions()
                TSelect(actions.find { it.first == b.actionType }?.second ?: "—", actions) { b.onAction(it) }
            }
            when (b.actionType) {
                "set_label" -> {
                    Field(stringResource(R.string.gitlab_bind_label)) {
                        TTextField(b.label, { b.label = it }, placeholder = stringResource(R.string.gitlab_bind_label_placeholder))
                    }
                    Field(stringResource(R.string.gitlab_bind_clear_prefix)) { TSwitch(b.clearPrefix, { b.clearPrefix = it }) }
                }

                "set_state" -> Field(stringResource(R.string.gitlab_bind_state)) {
                    val states = stateOptions()
                    TSelect(states.find { it.first == b.state }?.second ?: "—", states) { b.state = it }
                }

                "set_due" -> Field(stringResource(R.string.gitlab_bind_date_kind)) {
                    val kinds = dateKindOptions()
                    TSelect(kinds.find { it.first == b.actionDateKind }?.second ?: "—", kinds) { b.actionDateKind = it }
                }

                "post_comment" -> Field(stringResource(R.string.gitlab_bind_add_marker)) { TSwitch(b.addMarker, { b.addMarker = it }) }
            }
        }
    }
}

private fun completedKey(v: Boolean?): String = when (v) {
    true -> "true"
    false -> "false"
    null -> ""
}

/** Сводка триггера в шапке карточки. Композабл, а не обычная функция: подписи
 *  приходят из ресурсов и обязаны пересчитаться на смене языка. */
@Composable
private fun triggerSummary(b: EditBinding, columnName: String): String = when (b.triggerType) {
    "column" -> stringResource(R.string.gitlab_sum_column, columnName.ifBlank { "?" })

    "priority" -> {
        val level = b.priority
        if (level == null) {
            stringResource(R.string.gitlab_sum_priority_any)
        } else {
            stringResource(R.string.gitlab_sum_priority, PriorityLabels.getOrElse(level) { "?" })
        }
    }

    "completion" -> when (b.completed) {
        null -> stringResource(R.string.gitlab_trigger_completion)
        true -> stringResource(R.string.gitlab_completion_true)
        false -> stringResource(R.string.gitlab_completion_false)
    }

    "due" -> if (b.triggerDateKind == "start") {
        stringResource(R.string.gitlab_sum_start_change)
    } else {
        stringResource(R.string.gitlab_trigger_due)
    }

    else -> triggerTypeOptions().find { it.first == b.triggerType }?.second ?: b.triggerType
}

@Composable
private fun actionSummary(b: EditBinding): String = when (b.actionType) {
    "set_label" -> stringResource(R.string.gitlab_sum_label, b.label.ifBlank { "?" })

    "set_state" -> when (b.state) {
        "closed" -> stringResource(R.string.gitlab_sum_state_closed)
        "opened" -> stringResource(R.string.gitlab_sum_state_opened)
        else -> stringResource(R.string.gitlab_sum_state_any)
    }

    "post_comment" -> if (b.addMarker) {
        stringResource(R.string.gitlab_sum_comment_marker)
    } else {
        stringResource(R.string.gitlab_sum_comment)
    }

    else -> (actionTypeOptions().find { it.first == b.actionType }?.second ?: b.actionType).lowercase()
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
