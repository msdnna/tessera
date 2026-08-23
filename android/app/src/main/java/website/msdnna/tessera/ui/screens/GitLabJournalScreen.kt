package website.msdnna.tessera.ui.screens

import android.content.res.Resources
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalResources
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Dialog
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import com.google.gson.JsonObject
import website.msdnna.tessera.R
import website.msdnna.tessera.data.model.GitlabSyncAction
import website.msdnna.tessera.data.model.GitlabSyncRun
import website.msdnna.tessera.ui.components.IonIcon
import website.msdnna.tessera.ui.components.TButton
import website.msdnna.tessera.ui.components.TButtonKind
import website.msdnna.tessera.ui.components.TCard
import website.msdnna.tessera.ui.components.TesseraLoader
import website.msdnna.tessera.ui.components.clickableNoRipple
import website.msdnna.tessera.ui.resolve
import website.msdnna.tessera.ui.theme.PriorityLabels
import website.msdnna.tessera.ui.theme.RadiusLg
import website.msdnna.tessera.ui.theme.RadiusSm
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.viewmodels.GitlabJournalViewModel
import website.msdnna.tessera.util.Ion
import website.msdnna.tessera.util.dueLabel
import website.msdnna.tessera.util.localDateTimeLabel

private val OK = Color(0xFF18A058)
private val WARN = Color(0xFFF0A020)
private val ERR = Color(0xFFD03050)
private val PULL = Color(0xFF2B6CB0)
private val PUSH = Color(0xFF805AD5)

private val TriggerLabels = mapOf(
    "manual" to R.string.gljournal_trigger_manual,
    "auto" to R.string.gljournal_trigger_auto,
)

/** Подписи полей диффа — те же свойства задачи, что показывает её модалка, поэтому
 *  ключи общие (`task_prop_*`), а не свои на те же слова. */
private val FieldLabels = mapOf(
    "title" to R.string.task_prop_title,
    "description" to R.string.task_tab_description,
    "priority" to R.string.task_prop_priority,
    "column" to R.string.task_prop_column,
    "completed" to R.string.task_prop_status,
    "due" to R.string.task_prop_due,
    "start" to R.string.task_prop_start,
)
private val FieldOrder = listOf("title", "description", "priority", "column", "completed", "due", "start")

/** GitLab sync journal: a list of pull/push runs (tap to expand its actions) and a
 *  per-action before/after diff dialog with retry for failed pushes. Mirrors the
 *  web `GitLabJournalModal` master-detail, stacked for mobile. */
@Composable
fun GitLabJournalScreen(workspaceId: String, vm: GitlabJournalViewModel = viewModel(key = "gitlabJournal")) {
    val c = Tessera.colors
    val state by vm.state.collectAsStateWithLifecycle()
    LaunchedEffect(workspaceId) { vm.load(workspaceId) }

    if (state.loading) {
        Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) { TesseraLoader() }
        return
    }

    Column(Modifier.fillMaxSize().padding(horizontal = 16.dp)) {
        state.error?.let {
            Spacer(Modifier.height(10.dp))
            Text(it.resolve(), color = ERR, fontSize = 13.sp)
        }
        state.message?.let {
            Spacer(Modifier.height(10.dp))
            Text(it.resolve(), color = c.primary, fontSize = 13.sp)
        }
        Spacer(Modifier.height(12.dp))

        if (state.runs.isEmpty()) {
            Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                Text(stringResource(R.string.gljournal_empty), color = c.text3, fontSize = 14.sp)
            }
            return
        }

        LazyColumn(verticalArrangement = Arrangement.spacedBy(4.dp)) {
            items(state.runs, key = { it.id }) { run ->
                RunRow(
                    run = run,
                    expanded = state.expandedRunId == run.id,
                    actions = state.actionsByRun[run.id],
                    loadingActions = state.loadingActions && state.expandedRunId == run.id,
                    onToggle = { vm.toggleRun(workspaceId, run) },
                    onAction = { vm.select(run, it) },
                )
            }
        }
    }

    state.selected?.let { (run, action) ->
        ActionDetailDialog(run, action, retrying = state.retrying, onRetry = { vm.retry(workspaceId) }, onDismiss = { vm.closeDetail() })
    }
}

@Composable
private fun RunRow(
    run: GitlabSyncRun,
    expanded: Boolean,
    actions: List<GitlabSyncAction>?,
    loadingActions: Boolean,
    onToggle: () -> Unit,
    onAction: (GitlabSyncAction) -> Unit,
) {
    val c = Tessera.colors
    // Дату, подпись триггера и счётчики собирают обычные функции — ресурсы берём из
    // композиции, где их уже подменил AppLocale на язык профиля.
    val res = LocalResources.current
    Column {
        Row(
            Modifier.fillMaxWidth().clip(RoundedCornerShape(RadiusSm)).clickableNoRipple(onClick = onToggle)
                .padding(vertical = 8.dp, horizontal = 6.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            KindChip(run.kind)
            Spacer(Modifier.width(8.dp))
            Column(Modifier.weight(1f)) {
                Text(localDateTimeLabel(res, run.startedAt), color = c.text1, fontSize = 13.sp)
                Text(
                    res.getString(R.string.gljournal_run_meta, triggerLabel(res, run.trigger), runCounts(res, run)),
                    color = c.text3, fontSize = 11.sp,
                )
            }
            Box(Modifier.size(8.dp).clip(CircleShape).background(statusColor(run.status)))
        }
        if (expanded) {
            Column(Modifier.padding(start = 14.dp, bottom = 6.dp)) {
                when {
                    loadingActions -> Text(stringResource(R.string.common_loading), color = c.text3, fontSize = 12.sp, modifier = Modifier.padding(6.dp))

                    actions.isNullOrEmpty() -> Text(
                        stringResource(R.string.gljournal_no_actions),
                        color = c.text3, fontSize = 12.sp, modifier = Modifier.padding(6.dp),
                    )

                    else -> actions.forEach { a ->
                        Row(
                            Modifier.fillMaxWidth().clip(RoundedCornerShape(RadiusSm)).clickableNoRipple { onAction(a) }
                                .padding(vertical = 5.dp, horizontal = 8.dp),
                            verticalAlignment = Alignment.CenterVertically,
                        ) {
                            Text(a.op.uppercase(), color = opColor(a.op), fontSize = 10.sp, fontWeight = FontWeight.SemiBold, modifier = Modifier.width(52.dp))
                            Text(
                                a.summary,
                                color = if (a.status == "fail") ERR else c.text2,
                                fontSize = 12.5.sp,
                                maxLines = 1,
                                modifier = Modifier.weight(1f),
                            )
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun ActionDetailDialog(
    run: GitlabSyncRun,
    action: GitlabSyncAction,
    retrying: Boolean,
    onRetry: () -> Unit,
    onDismiss: () -> Unit,
) {
    val c = Tessera.colors
    // Даты в журнале рисует обычная функция — ресурсы берём из композиции, где их
    // уже подменил AppLocale на язык профиля.
    val res = LocalResources.current
    val isPush = action.direction == "push"
    val canRetry = isPush && action.status == "fail"
    val detail = action.detail ?: JsonObject()
    Dialog(onDismissRequest = onDismiss) {
        Column(
            Modifier.fillMaxWidth().clip(RoundedCornerShape(RadiusLg)).background(c.surface)
                .padding(20.dp).heightIn(max = 560.dp).verticalScroll(rememberScrollState()),
        ) {
            // direction + summary
            Text(
                if (isPush) "Tessera → GitLab" else "GitLab → Tessera",
                color = if (isPush) PUSH else PULL, fontSize = 11.sp, fontWeight = FontWeight.Bold,
            )
            Spacer(Modifier.height(2.dp))
            Text(action.summary, color = c.text1, fontSize = 15.sp, fontWeight = FontWeight.SemiBold)

            // pull: changed fields
            detail.objOrNull("fields")?.let { fields ->
                DetailSection {
                    orderedKeys(fields).forEach { k ->
                        val f = fields.objOrNull(k) ?: return@forEach
                        Row(Modifier.padding(vertical = 3.dp)) {
                            Text(fieldLabel(res, k), color = c.text3, fontSize = 12.sp, modifier = Modifier.width(96.dp))
                            Text(fmtVal(res, k, f.get("before")), color = ERR, fontSize = 12.5.sp)
                            Text(" → ", color = c.text3, fontSize = 12.5.sp)
                            Text(fmtVal(res, k, f.get("after")), color = c.text1, fontSize = 12.5.sp)
                        }
                    }
                }
            }
            // pull: created snapshot
            detail.objOrNull("after")?.let { after ->
                DetailSection {
                    orderedKeys(after).forEach { k ->
                        Row(Modifier.padding(vertical = 3.dp)) {
                            Text(fieldLabel(res, k), color = c.text3, fontSize = 12.sp, modifier = Modifier.width(96.dp))
                            Text(fmtVal(res, k, after.get(k)), color = c.text1, fontSize = 12.5.sp)
                        }
                    }
                }
            }
            // tags
            detail.objOrNull("tags")?.let { tags ->
                DetailSection {
                    Text(stringResource(R.string.task_prop_tags), color = c.text3, fontSize = 12.sp)
                    Spacer(Modifier.height(4.dp))
                    tags.strList("added").forEach { Text("+ $it", color = OK, fontSize = 12.5.sp) }
                    tags.strList("removed").forEach { Text("− $it", color = ERR, fontSize = 12.5.sp) }
                }
            }
            // comments
            detail.objOrNull("comments")?.let { com ->
                val added = com.get("added")?.takeUnless { it.isJsonNull }?.asInt ?: 0
                if (added > 0) {
                    DetailSection {
                        Text(stringResource(R.string.gljournal_comments_new, added), color = c.text3, fontSize = 12.sp)
                        com.strList("new").forEach {
                            Spacer(Modifier.height(4.dp))
                            Text(
                                it, color = c.text2, fontSize = 12.5.sp,
                                modifier = Modifier.fillMaxWidth()
                                    .background(c.surfaceAlt, RoundedCornerShape(RadiusSm)).padding(7.dp),
                            )
                        }
                    }
                }
            }
            // relations — one aggregated row per run (entity_type='relation'), so the
            // counts never disturb the per-task created/updated counters
            detail.objOrNull("relations")?.let { rel ->
                val added = rel.int("added")
                val removed = rel.int("removed")
                val deferred = rel.int("deferred")
                if (added > 0 || removed > 0 || deferred > 0) {
                    DetailSection {
                        Text(stringResource(R.string.task_tab_relations), color = c.text3, fontSize = 12.sp)
                        Spacer(Modifier.height(4.dp))
                        if (added > 0) {
                            Text(
                                pluralStringResource(R.plurals.gljournal_relations_added, added, added),
                                color = OK, fontSize = 12.5.sp,
                            )
                        }
                        if (removed > 0) Text(stringResource(R.string.gljournal_relations_removed, removed), color = ERR, fontSize = 12.5.sp)
                        if (deferred > 0) Text(stringResource(R.string.gljournal_relations_deferred, deferred), color = c.text2, fontSize = 12.5.sp)
                    }
                }
            }
            // push payload + result/error
            if (isPush) {
                DetailSection {
                    Row(Modifier.padding(vertical = 3.dp)) {
                        Text(stringResource(R.string.gitlab_add_action), color = c.text3, fontSize = 12.sp, modifier = Modifier.width(96.dp))
                        Text(pushPayloadText(res, detail), color = c.text1, fontSize = 12.5.sp)
                    }
                    if (action.status == "fail") {
                        Text(
                            action.error.ifBlank { detail.str("error").ifBlank { stringResource(R.string.gljournal_delivery_failed) } },
                            color = ERR, fontSize = 12.5.sp,
                            modifier = Modifier.background(ERR.copy(alpha = 0.1f), RoundedCornerShape(RadiusSm)).padding(7.dp).fillMaxWidth(),
                        )
                    } else {
                        detail.str("result").takeIf { it.isNotBlank() }?.let { Text(it, color = OK, fontSize = 12.5.sp) }
                    }
                }
            }

            Spacer(Modifier.height(16.dp))
            Row(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                TButton(stringResource(R.string.common_close), onClick = onDismiss, kind = TButtonKind.Secondary, modifier = Modifier.weight(1f))
                if (canRetry) {
                    TButton(
                        stringResource(R.string.common_retry),
                        onClick = onRetry, loading = retrying, icon = Ion.REFRESH, modifier = Modifier.weight(1f),
                    )
                }
            }
        }
    }
}

@Composable
private fun DetailSection(content: @Composable () -> Unit) {
    val c = Tessera.colors
    Column(Modifier.fillMaxWidth().padding(top = 10.dp)) {
        androidx.compose.material3.HorizontalDivider(color = c.border)
        Spacer(Modifier.height(8.dp))
        content()
    }
}

@Composable
private fun KindChip(kind: String) {
    val color = if (kind == "push") PUSH else PULL
    Text(
        if (kind == "push") "Push" else "Pull",
        color = color, fontSize = 11.sp, fontWeight = FontWeight.Bold,
        modifier = Modifier.clip(RoundedCornerShape(RadiusSm)).background(color.copy(alpha = 0.14f)).padding(horizontal = 7.dp, vertical = 2.dp),
    )
}

// ── helpers ──────────────────────────────────────────────────────────────────

private fun statusColor(s: String) = when (s) {
    "partial" -> WARN
    "error", "fail" -> ERR
    else -> OK
}

private fun opColor(op: String) = when (op) {
    "create" -> OK
    "update" -> Color(0xFF2080F0)
    "delete" -> ERR
    else -> Color(0xFF8A8A8A)
}

/** Подпись триггера прогона; незнакомое значение показываем как есть — оно с сервера. */
internal fun triggerLabel(res: Resources, trigger: String): String =
    TriggerLabels[trigger]?.let { res.getString(it) } ?: trigger

/** Подпись поля в диффе; незнакомый ключ показываем как есть (поле добавили на бэке). */
internal fun fieldLabel(res: Resources, key: String): String =
    FieldLabels[key]?.let { res.getString(it) } ?: key

internal fun runCounts(res: Resources, run: GitlabSyncRun): String {
    if (run.kind == "push") return res.getString(R.string.gljournal_push_count, run.actionCount)
    val parts = buildList {
        if (run.createdCount > 0) add("+${run.createdCount}")
        if (run.updatedCount > 0) add("~${run.updatedCount}")
    }
    return parts.joinToString(" ").ifBlank { res.getString(R.string.gljournal_no_changes) }
}

private fun orderedKeys(obj: JsonObject): List<String> = FieldOrder.filter { obj.has(it) }

internal fun fmtVal(res: Resources, key: String, el: com.google.gson.JsonElement?): String {
    if (el == null || el.isJsonNull) return "—"
    val raw = if (el.isJsonPrimitive) el.asString else el.toString()
    if (raw.isBlank()) return "—"
    return when (key) {
        "priority" -> PriorityLabels.getOrNull(raw.toDoubleOrNull()?.toInt() ?: -1) ?: raw
        "completed" -> res.getString(if (raw == "true") R.string.task_status_completed else R.string.task_status_active)
        "due", "start" -> dueLabel(res, raw)
        else -> raw
    }
}

internal fun pushPayloadText(res: Resources, detail: JsonObject): String {
    val p = detail.objOrNull("payload") ?: JsonObject()
    return when (detail.str("change_kind")) {
        "state" -> res.getString(
            if (p.str("state") == "closed") R.string.gljournal_push_issue_close else R.string.gljournal_push_issue_open,
        )

        "priority" -> res.getString(
            R.string.gljournal_push_priority,
            PriorityLabels.getOrNull(p.str("priority").toDoubleOrNull()?.toInt() ?: -1) ?: p.str("priority"),
        )

        "comment" -> p.str("body")

        else -> p.toString()
    }
}

private fun JsonObject.objOrNull(key: String): JsonObject? =
    get(key)?.takeIf { it.isJsonObject }?.asJsonObject

private fun JsonObject.int(key: String): Int =
    get(key)?.takeUnless { it.isJsonNull }?.takeIf { it.isJsonPrimitive }?.asInt ?: 0

private fun JsonObject.str(key: String): String =
    get(key)?.takeUnless { it.isJsonNull }?.takeIf { it.isJsonPrimitive }?.asString ?: ""

private fun JsonObject.strList(key: String): List<String> =
    get(key)?.takeIf { it.isJsonArray }?.asJsonArray
        ?.mapNotNull { e -> e.takeUnless { it.isJsonNull }?.let { if (it.isJsonPrimitive) it.asString else it.toString() } }
        .orEmpty()
