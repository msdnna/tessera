package website.msdnna.tessera.ui.screens

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
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Dialog
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import com.google.gson.JsonObject
import website.msdnna.tessera.data.model.GitlabSyncAction
import website.msdnna.tessera.data.model.GitlabSyncRun
import website.msdnna.tessera.ui.components.IonIcon
import website.msdnna.tessera.ui.components.TButton
import website.msdnna.tessera.ui.components.TButtonKind
import website.msdnna.tessera.ui.components.TCard
import website.msdnna.tessera.ui.components.TesseraLoader
import website.msdnna.tessera.ui.components.clickableNoRipple
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

private val TriggerLabel = mapOf("manual" to "вручную", "auto" to "авто")
private val FieldLabels = mapOf(
    "title" to "Заголовок", "description" to "Описание", "priority" to "Приоритет",
    "column" to "Колонка", "completed" to "Статус", "due" to "Срок", "start" to "Начало",
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
            Text(it, color = ERR, fontSize = 13.sp)
        }
        state.message?.let {
            Spacer(Modifier.height(10.dp))
            Text(it, color = c.primary, fontSize = 13.sp)
        }
        Spacer(Modifier.height(12.dp))

        if (state.runs.isEmpty()) {
            Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                Text("Журнал пуст — синхронизация ещё не запускалась", color = c.text3, fontSize = 14.sp)
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
    Column {
        Row(
            Modifier.fillMaxWidth().clip(RoundedCornerShape(RadiusSm)).clickableNoRipple(onClick = onToggle)
                .padding(vertical = 8.dp, horizontal = 6.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            KindChip(run.kind)
            Spacer(Modifier.width(8.dp))
            Column(Modifier.weight(1f)) {
                Text(localDateTimeLabel(run.startedAt), color = c.text1, fontSize = 13.sp)
                Text(
                    "${TriggerLabel[run.trigger] ?: run.trigger} · ${runCounts(run)}",
                    color = c.text3, fontSize = 11.sp,
                )
            }
            Box(Modifier.size(8.dp).clip(CircleShape).background(statusColor(run.status)))
        }
        if (expanded) {
            Column(Modifier.padding(start = 14.dp, bottom = 6.dp)) {
                when {
                    loadingActions -> Text("Загрузка…", color = c.text3, fontSize = 12.sp, modifier = Modifier.padding(6.dp))

                    actions.isNullOrEmpty() -> Text("Нет записанных действий", color = c.text3, fontSize = 12.sp, modifier = Modifier.padding(6.dp))

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
                            Text(FieldLabels[k] ?: k, color = c.text3, fontSize = 12.sp, modifier = Modifier.width(96.dp))
                            Text(fmtVal(k, f.get("before")), color = ERR, fontSize = 12.5.sp)
                            Text(" → ", color = c.text3, fontSize = 12.5.sp)
                            Text(fmtVal(k, f.get("after")), color = c.text1, fontSize = 12.5.sp)
                        }
                    }
                }
            }
            // pull: created snapshot
            detail.objOrNull("after")?.let { after ->
                DetailSection {
                    orderedKeys(after).forEach { k ->
                        Row(Modifier.padding(vertical = 3.dp)) {
                            Text(FieldLabels[k] ?: k, color = c.text3, fontSize = 12.sp, modifier = Modifier.width(96.dp))
                            Text(fmtVal(k, after.get(k)), color = c.text1, fontSize = 12.5.sp)
                        }
                    }
                }
            }
            // tags
            detail.objOrNull("tags")?.let { tags ->
                DetailSection {
                    Text("Теги", color = c.text3, fontSize = 12.sp)
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
                        Text("Новые комментарии ($added)", color = c.text3, fontSize = 12.sp)
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
            // push payload + result/error
            if (isPush) {
                DetailSection {
                    Row(Modifier.padding(vertical = 3.dp)) {
                        Text("Действие", color = c.text3, fontSize = 12.sp, modifier = Modifier.width(96.dp))
                        Text(pushPayloadText(detail), color = c.text1, fontSize = 12.5.sp)
                    }
                    if (action.status == "fail") {
                        Text(
                            action.error.ifBlank { detail.str("error").ifBlank { "Ошибка доставки" } },
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
                TButton("Закрыть", onClick = onDismiss, kind = TButtonKind.Secondary, modifier = Modifier.weight(1f))
                if (canRetry) {
                    TButton("Повторить", onClick = onRetry, loading = retrying, icon = Ion.REFRESH, modifier = Modifier.weight(1f))
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

private fun runCounts(run: GitlabSyncRun): String {
    if (run.kind == "push") return "${run.actionCount} дост."
    val parts = buildList {
        if (run.createdCount > 0) add("+${run.createdCount}")
        if (run.updatedCount > 0) add("~${run.updatedCount}")
    }
    return parts.joinToString(" ").ifBlank { "без изменений" }
}

private fun orderedKeys(obj: JsonObject): List<String> = FieldOrder.filter { obj.has(it) }

private fun fmtVal(key: String, el: com.google.gson.JsonElement?): String {
    if (el == null || el.isJsonNull) return "—"
    val raw = if (el.isJsonPrimitive) el.asString else el.toString()
    if (raw.isBlank()) return "—"
    return when (key) {
        "priority" -> PriorityLabels.getOrNull(raw.toDoubleOrNull()?.toInt() ?: -1) ?: raw
        "completed" -> if (raw == "true") "Выполнено" else "Не выполнено"
        "due", "start" -> dueLabel(raw)
        else -> raw
    }
}

private fun pushPayloadText(detail: JsonObject): String {
    val p = detail.objOrNull("payload") ?: JsonObject()
    return when (detail.str("change_kind")) {
        "state" -> if (p.str("state") == "closed") "закрыть issue" else "открыть issue"
        "priority" -> "приоритет → " + (PriorityLabels.getOrNull(p.str("priority").toDoubleOrNull()?.toInt() ?: -1) ?: p.str("priority"))
        "comment" -> p.str("body")
        else -> p.toString()
    }
}

private fun JsonObject.objOrNull(key: String): JsonObject? =
    get(key)?.takeIf { it.isJsonObject }?.asJsonObject

private fun JsonObject.str(key: String): String =
    get(key)?.takeUnless { it.isJsonNull }?.takeIf { it.isJsonPrimitive }?.asString ?: ""

private fun JsonObject.strList(key: String): List<String> =
    get(key)?.takeIf { it.isJsonArray }?.asJsonArray
        ?.mapNotNull { e -> e.takeUnless { it.isJsonNull }?.let { if (it.isJsonPrimitive) it.asString else it.toString() } }
        .orEmpty()
