package website.msdnna.tessera.ui.screens

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
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.TransformOrigin
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Dialog
import website.msdnna.tessera.data.model.Task
import website.msdnna.tessera.ui.components.IonIcon
import website.msdnna.tessera.ui.components.IonIconButton
import website.msdnna.tessera.ui.components.TButton
import website.msdnna.tessera.ui.components.TButtonKind
import website.msdnna.tessera.ui.components.TConfirmPopover
import website.msdnna.tessera.ui.components.TTextField
import website.msdnna.tessera.ui.components.TagChip
import website.msdnna.tessera.ui.components.TesseraLoader
import website.msdnna.tessera.ui.components.clickableNoRipple
import website.msdnna.tessera.ui.components.popupAppear
import website.msdnna.tessera.ui.theme.PriorityLabels
import website.msdnna.tessera.ui.theme.RadiusLg
import website.msdnna.tessera.ui.theme.RadiusMd
import website.msdnna.tessera.ui.theme.RadiusSm
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.accentGradient
import website.msdnna.tessera.ui.viewmodels.BoardUiState
import website.msdnna.tessera.ui.viewmodels.BoardViewModel
import website.msdnna.tessera.util.Ion
import website.msdnna.tessera.util.buildTagGroups
import website.msdnna.tessera.util.parseHexColor

private val TagPalette = listOf(
    "#7c5cff", "#2f80ed", "#0eb0a9", "#18a058", "#f0a020", "#e0533d", "#eb2f96", "#9aa0aa",
)

/** Tag manager: create, recolour, rename and delete workspace tags. */
@Composable
fun TagManagerModal(state: BoardUiState, vm: BoardViewModel, onDismiss: () -> Unit) {
    val c = Tessera.colors
    var newName by remember { mutableStateOf("") }
    // Which tag is in inline-edit; a tap on empty modal space clears it (cancel).
    var editingId by remember { mutableStateOf<String?>(null) }
    Dialog(onDismissRequest = onDismiss) {
        Column(
            Modifier.popupAppear(TransformOrigin.Center).fillMaxWidth().clip(RoundedCornerShape(RadiusLg))
                .background(c.surface).clickableNoRipple { editingId = null }.padding(18.dp),
        ) {
            Text("Управление тегами", color = c.text1, fontSize = 16.sp, fontWeight = FontWeight.SemiBold)
            Spacer(Modifier.height(12.dp))

            if (state.tagList.isEmpty()) {
                Text("Тегов пока нет", color = c.text3, fontSize = 13.sp)
            } else {
                val groups = buildTagGroups(state.tagList, state.prefixNames)
                val showHeaders = groups.size > 1
                Column(Modifier.heightIn(max = 340.dp).verticalScroll(rememberScrollState())) {
                    groups.forEach { g ->
                        if (showHeaders) {
                            Text(
                                g.label.uppercase(),
                                color = c.text3,
                                fontSize = 10.sp,
                                fontWeight = FontWeight.SemiBold,
                                letterSpacing = 0.4.sp,
                                modifier = Modifier.padding(top = 8.dp, bottom = 2.dp),
                            )
                        }
                        g.tags.forEach { tag ->
                            TagRow(
                                tag,
                                vm,
                                editing = editingId == tag.id,
                                onEdit = { editingId = tag.id },
                                onDone = { editingId = null },
                            )
                        }
                    }
                }
            }

            Spacer(Modifier.height(14.dp))
            Text("Новый тег", color = c.text3, fontSize = 12.sp, fontWeight = FontWeight.Medium)
            Spacer(Modifier.height(6.dp))
            Row(verticalAlignment = Alignment.CenterVertically) {
                Box(Modifier.weight(1f)) {
                    TTextField(value = newName, onValueChange = { newName = it }, placeholder = "Название тега")
                }
                Spacer(Modifier.width(8.dp))
                TButton(
                    "Создать",
                    enabled = newName.isNotBlank(),
                    onClick = {
                        vm.createTagStandalone(newName.trim(), TagPalette.first())
                        newName = ""
                    },
                )
            }

            Spacer(Modifier.height(14.dp))
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
                TButton("Готово", onClick = onDismiss)
            }
        }
    }
}

/**
 * One tag row (web `TagManager` parity): the tag's own badge, tapped to open an
 * inline editor (rename field + colour swatches); a trash button stays alongside.
 * [editing] is owned by the modal so a tap outside the row cancels it.
 */
@Composable
private fun TagRow(
    tag: website.msdnna.tessera.data.model.Tag,
    vm: BoardViewModel,
    editing: Boolean,
    onEdit: () -> Unit,
    onDone: () -> Unit,
) {
    val c = Tessera.colors
    var nameEdit by remember(tag.id, editing) { mutableStateOf(tag.name) }
    var confirmDelete by remember { mutableStateOf(false) }
    fun commit(color: String) = vm.updateTag(tag.id, nameEdit.trim().ifBlank { tag.name }, color)

    Column(Modifier.fillMaxWidth().padding(vertical = 6.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            if (editing) {
                Box(Modifier.weight(1f)) {
                    TTextField(value = nameEdit, onValueChange = { nameEdit = it }, placeholder = "Имя тега")
                }
                Spacer(Modifier.width(6.dp))
                IonIconButton(
                    Ion.CHECK,
                    onClick = {
                        commit(tag.color)
                        onDone()
                    },
                    boxSize = 30.dp,
                    iconSize = 16.dp,
                    tint = c.primary,
                )
            } else {
                Box(Modifier.weight(1f), contentAlignment = Alignment.CenterStart) {
                    TagChip(name = tag.name, color = tag.color, big = true, modifier = Modifier.clickableNoRipple { onEdit() })
                }
            }
            Box {
                IonIconButton(Ion.TRASH, onClick = { confirmDelete = true }, boxSize = 30.dp, iconSize = 15.dp, tint = c.text3)
                TConfirmPopover(
                    expanded = confirmDelete,
                    message = "Удалить тег? Он снимется со всех задач.",
                    onConfirm = {
                        confirmDelete = false
                        vm.deleteTag(tag.id)
                    },
                    onDismiss = { confirmDelete = false },
                )
            }
        }
        if (editing) {
            FlowRow(
                Modifier.padding(top = 8.dp),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                TagPalette.forEach { hex ->
                    val selected = tag.color.equals(hex, ignoreCase = true)
                    Box(
                        Modifier.size(22.dp).clip(CircleShape).background(accentGradient(parseHexColor(hex, c.text3)))
                            .then(if (selected) Modifier.border(2.dp, c.text1, CircleShape) else Modifier)
                            .clickableNoRipple { commit(hex) },
                    )
                }
            }
        }
    }
}
