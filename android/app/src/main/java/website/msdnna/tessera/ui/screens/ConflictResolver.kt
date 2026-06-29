package website.msdnna.tessera.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import website.msdnna.tessera.data.model.ConflictField
import website.msdnna.tessera.data.model.GitlabConflict
import website.msdnna.tessera.ui.components.IonIconButton
import website.msdnna.tessera.ui.components.TButton
import website.msdnna.tessera.ui.components.TButtonKind
import website.msdnna.tessera.ui.components.TTextField
import website.msdnna.tessera.ui.theme.PriorityLabels
import website.msdnna.tessera.ui.theme.RadiusLg
import website.msdnna.tessera.ui.theme.RadiusSm
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.viewmodels.ConflictsViewModel
import website.msdnna.tessera.util.Ion
import website.msdnna.tessera.util.diffSegments

private val TheirsColor = Color(0xFFD03050) // GitLab (red)

/**
 * GitLab write-back conflict resolver (web `ConflictResolverModal` parity). Shows
 * the diverged field(s) three ways — Было (base) / GitLab (theirs) / Моё (ours) —
 * with inline diff highlighting on text fields, and resolves via «Моё» / «GitLab» /
 * «Объединить вручную». Manual merge is hidden for discrete fields (state/priority).
 */
@Composable
fun ConflictResolverModal(vm: ConflictsViewModel, onDismiss: () -> Unit) {
    val c = Tessera.colors
    val state by vm.state.collectAsStateWithLifecycle()
    val conflicts = state.list
    // The conflict to show: the focused task's, else the first.
    val selected = remember(conflicts, state.focusTaskId) {
        conflicts.firstOrNull { it.taskId == state.focusTaskId } ?: conflicts.firstOrNull()
    }

    if (selected == null) {
        // Nothing left to resolve → close.
        androidx.compose.runtime.LaunchedEffect(Unit) { onDismiss() }
        return
    }

    var manual by remember(selected.id) { mutableStateOf(false) }
    val manualValues = remember(selected.id) { mutableStateMapOf<String, String>() }

    Dialog(onDismissRequest = onDismiss, properties = DialogProperties(usePlatformDefaultWidth = false)) {
        Column(
            Modifier.fillMaxWidth(0.96f).fillMaxHeight(0.9f).clip(RoundedCornerShape(RadiusLg)).background(c.surface),
        ) {
            // Header.
            Row(
                Modifier.fillMaxWidth().padding(start = 18.dp, end = 10.dp, top = 16.dp, bottom = 8.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Column(Modifier.weight(1f)) {
                    Text("Конфликт GitLab", color = c.text1, fontSize = 17.sp, fontWeight = FontWeight.SemiBold)
                    val num = selected.taskNumber?.let { "#$it · " } ?: ""
                    Text("$num${selected.taskTitle}", color = c.text3, fontSize = 12.sp, maxLines = 2)
                }
                IonIconButton(Ion.CLOSE, onClick = onDismiss, boxSize = 32.dp, iconSize = 18.dp, tint = c.text3)
            }
            if (conflicts.size > 1) {
                Text(
                    "Ещё конфликтов: ${conflicts.size - 1}",
                    color = c.text3,
                    fontSize = 12.sp,
                    modifier = Modifier.padding(horizontal = 18.dp, vertical = 2.dp),
                )
            }

            Column(Modifier.weight(1f).fillMaxWidth().verticalScroll(rememberScrollState()).padding(horizontal = 16.dp)) {
                selected.fields.forEach { f ->
                    FieldBlock(f = f, manual = manual, manualValues = manualValues)
                    Spacer(Modifier.height(12.dp))
                }
            }

            // Actions.
            Column(Modifier.fillMaxWidth().padding(16.dp)) {
                if (manual) {
                    TButton(
                        "Сохранить объединение",
                        onClick = { vm.resolve(selected, "manual", manualValues.toMap()) },
                        modifier = Modifier.fillMaxWidth(),
                        loading = state.resolving,
                    )
                    Spacer(Modifier.height(8.dp))
                    TButton("Отмена", kind = TButtonKind.Secondary, onClick = { manual = false }, modifier = Modifier.fillMaxWidth())
                } else {
                    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        TButton("Принять моё", onClick = { vm.resolve(selected, "ours") }, modifier = Modifier.weight(1f), loading = state.resolving)
                        TButton("Принять GitLab", kind = TButtonKind.Secondary, onClick = { vm.resolve(selected, "theirs") }, modifier = Modifier.weight(1f))
                    }
                    if (selected.manualAllowed) {
                        Spacer(Modifier.height(8.dp))
                        TButton(
                            "Объединить вручную…",
                            kind = TButtonKind.Ghost,
                            onClick = {
                                selected.fields.forEach { manualValues[it.field] = it.theirs }
                                manual = true
                            },
                            modifier = Modifier.fillMaxWidth(),
                        )
                    }
                }
            }
        }
    }
}

@Composable
private fun FieldBlock(
    f: ConflictField,
    manual: Boolean,
    manualValues: androidx.compose.runtime.snapshots.SnapshotStateMap<String, String>,
) {
    val c = Tessera.colors
    Column(Modifier.fillMaxWidth()) {
        Text(fieldLabel(f.field).uppercase(), color = c.text3, fontSize = 11.sp, fontWeight = FontWeight.SemiBold)
        Spacer(Modifier.height(6.dp))
        if (manual) {
            TTextField(
                value = manualValues[f.field] ?: f.theirs,
                onValueChange = { manualValues[f.field] = it },
                singleLine = !f.isText,
                modifier = Modifier.fillMaxWidth(),
            )
        } else {
            ValueCard("Было", displayValue(f.field, f.base), borderColor = c.border, textColor = c.text3)
            Spacer(Modifier.height(6.dp))
            ValueCard(
                "GitLab",
                annotated = if (f.isText) diffAnnotated(f.base, f.theirs, TheirsColor) else null,
                plain = if (f.isText) null else displayValue(f.field, f.theirs),
                borderColor = TheirsColor,
                textColor = c.text1,
            )
            Spacer(Modifier.height(6.dp))
            ValueCard(
                "Моё",
                annotated = if (f.isText) diffAnnotated(f.base, f.ours, c.primary) else null,
                plain = if (f.isText) null else displayValue(f.field, f.ours),
                borderColor = c.primary,
                textColor = c.text1,
            )
        }
    }
}

@Composable
private fun ValueCard(
    label: String,
    plain: String? = null,
    annotated: AnnotatedString? = null,
    borderColor: Color,
    textColor: Color,
) {
    val c = Tessera.colors
    Column(
        Modifier.fillMaxWidth().clip(RoundedCornerShape(RadiusSm))
            .border(1.dp, borderColor.copy(alpha = 0.6f), RoundedCornerShape(RadiusSm))
            .padding(10.dp),
    ) {
        Text(label, color = borderColor, fontSize = 10.sp, fontWeight = FontWeight.SemiBold)
        Spacer(Modifier.height(4.dp))
        if (annotated != null) {
            Text(annotated, color = textColor, fontSize = 13.sp)
        } else {
            Text(plain.orEmpty().ifBlank { "—" }, color = textColor, fontSize = 13.sp)
        }
    }
}

/** Inline diff: unchanged spans plain, changed spans highlighted in [hue] at 30% fill. */
private fun diffAnnotated(base: String, cur: String, hue: Color): AnnotatedString = buildAnnotatedString {
    diffSegments(base, cur).forEach { seg ->
        if (seg.changed) {
            withStyle(SpanStyle(background = hue.copy(alpha = 0.3f))) { append(seg.text) }
        } else {
            append(seg.text)
        }
    }
}

private fun fieldLabel(field: String): String = when (field) {
    "due" -> "Срок"
    "estimate" -> "Оценка"
    "title" -> "Заголовок"
    "description" -> "Описание"
    "state" -> "Статус"
    "priority" -> "Приоритет"
    else -> field
}

private fun displayValue(field: String, raw: String): String = when {
    raw.isBlank() -> ""
    field == "state" -> if (raw == "closed") "Закрыта" else "Открыта"
    field == "priority" -> raw.toIntOrNull()?.let { PriorityLabels.getOrNull(it) } ?: raw
    else -> raw
}
