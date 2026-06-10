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
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Dialog
import website.msdnna.tessera.data.model.Task
import website.msdnna.tessera.ui.components.ColorDot
import website.msdnna.tessera.ui.components.IonIcon
import website.msdnna.tessera.ui.components.IonIconButton
import website.msdnna.tessera.ui.components.TButton
import website.msdnna.tessera.ui.components.TButtonKind
import website.msdnna.tessera.ui.components.TConfirmPopover
import website.msdnna.tessera.ui.components.TInputDialog
import website.msdnna.tessera.ui.components.TTextField
import website.msdnna.tessera.ui.components.TesseraLoader
import website.msdnna.tessera.ui.components.clickableNoRipple
import website.msdnna.tessera.ui.theme.PriorityLabels
import website.msdnna.tessera.ui.theme.RadiusLg
import website.msdnna.tessera.ui.theme.RadiusMd
import website.msdnna.tessera.ui.theme.RadiusSm
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.accentGradient
import website.msdnna.tessera.ui.viewmodels.BoardFilter
import website.msdnna.tessera.ui.viewmodels.BoardUiState
import website.msdnna.tessera.ui.viewmodels.BoardViewModel
import website.msdnna.tessera.ui.viewmodels.DueFilter
import website.msdnna.tessera.util.Ion
import website.msdnna.tessera.util.parseHexColor

private val TagPalette = listOf(
    "#7c5cff", "#2f80ed", "#0eb0a9", "#18a058", "#f0a020", "#e0533d", "#eb2f96", "#9aa0aa",
)

private val DueLabels = mapOf(
    DueFilter.All to "Все",
    DueFilter.Overdue to "Просрочено",
    DueFilter.Today to "Сегодня",
    DueFilter.Week to "Неделя",
    DueFilter.Has to "Со сроком",
    DueFilter.None to "Без срока",
)

/**
 * Filter chips panel (web KanbanBoard filter dropdown) — priority / due / tags /
 * assignees only. Sorting + the title search live in the toolbar, so they're not
 * here. Rendered inside the filter button's [website.msdnna.tessera.ui.components
 * .TDropdown]; changes apply immediately.
 */
@Composable
fun FilterPanel(state: BoardUiState, vm: BoardViewModel) {
    val c = Tessera.colors
    val f = state.filter
    Column(Modifier.width(300.dp).heightIn(max = 420.dp).verticalScroll(rememberScrollState()).padding(14.dp)) {
        if (f.isActive) {
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
                Text(
                    "Сбросить",
                    color = c.primary,
                    fontSize = 13.sp,
                    fontWeight = FontWeight.Medium,
                    modifier = Modifier.clickableNoRipple { vm.clearFilter() }.padding(4.dp),
                )
            }
            Spacer(Modifier.height(6.dp))
        }

        SectionLabel("Приоритет")
        FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            PriorityLabels.forEachIndexed { i, label ->
                Chip(label, active = i in f.priorities) {
                    vm.setFilter(f.copy(priorities = f.priorities.toggle(i)))
                }
            }
        }
        Spacer(Modifier.height(14.dp))

        SectionLabel("Срок")
        FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            DueFilter.entries.forEach { d ->
                Chip(DueLabels[d] ?: d.name, active = f.due == d) {
                    vm.setFilter(f.copy(due = if (f.due == d) DueFilter.All else d))
                }
            }
        }

        if (state.tagList.isNotEmpty()) {
            Spacer(Modifier.height(14.dp))
            SectionLabel("Теги")
            FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                state.tagList.forEach { tag ->
                    Chip(tag.name, active = tag.id in f.tagIds, color = parseHexColor(tag.color, c.primary)) {
                        vm.setFilter(f.copy(tagIds = f.tagIds.toggle(tag.id)))
                    }
                }
            }
        }

        if (state.members.isNotEmpty()) {
            Spacer(Modifier.height(14.dp))
            SectionLabel("Исполнители")
            FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                state.members.forEach { m ->
                    Chip(m.name.ifBlank { m.email }, active = m.userId in f.assigneeIds) {
                        vm.setFilter(f.copy(assigneeIds = f.assigneeIds.toggle(m.userId)))
                    }
                }
            }
        }
    }
}

/** Board archive: list archived cards, restore or delete permanently. */
@Composable
fun ArchiveModal(state: BoardUiState, vm: BoardViewModel, onDismiss: () -> Unit) {
    val c = Tessera.colors
    Dialog(onDismissRequest = onDismiss) {
        Column(
            Modifier.fillMaxWidth().clip(RoundedCornerShape(RadiusLg)).background(c.surface).padding(18.dp),
        ) {
            Text("Архив доски", color = c.text1, fontSize = 16.sp, fontWeight = FontWeight.SemiBold)
            Spacer(Modifier.height(12.dp))
            val archived = state.archived
            when {
                archived == null -> Box(Modifier.fillMaxWidth().height(120.dp), contentAlignment = Alignment.Center) {
                    TesseraLoader()
                }

                archived.isEmpty() -> Text("Архив пуст", color = c.text3, fontSize = 13.sp)

                else -> Column(Modifier.heightIn(max = 380.dp).verticalScroll(rememberScrollState())) {
                    archived.forEach { task -> ArchiveRow(task, vm) }
                }
            }
            Spacer(Modifier.height(14.dp))
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
                TButton("Закрыть", kind = TButtonKind.Ghost, onClick = onDismiss)
            }
        }
    }
}

@Composable
private fun ArchiveRow(task: Task, vm: BoardViewModel) {
    val c = Tessera.colors
    var confirmDelete by remember { mutableStateOf(false) }
    Row(Modifier.fillMaxWidth().padding(vertical = 8.dp), verticalAlignment = Alignment.CenterVertically) {
        Column(Modifier.weight(1f)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                task.number?.let {
                    Text("#$it", color = c.text3, fontSize = 12.sp)
                    Spacer(Modifier.width(6.dp))
                }
                Text(
                    task.title,
                    color = c.text1,
                    fontSize = 14.sp,
                    maxLines = 1,
                    textDecoration = if (task.isCompleted) TextDecoration.LineThrough else null,
                )
            }
        }
        Text(
            "Вернуть",
            color = c.primary,
            fontSize = 13.sp,
            fontWeight = FontWeight.Medium,
            modifier = Modifier.clickableNoRipple { vm.restoreFromArchive(task.id) }.padding(horizontal = 6.dp, vertical = 4.dp),
        )
        Box {
            IonIconButton(Ion.TRASH, onClick = { confirmDelete = true }, boxSize = 30.dp, iconSize = 16.dp, tint = c.text3)
            TConfirmPopover(
                expanded = confirmDelete,
                message = "Удалить навсегда?",
                onConfirm = {
                    confirmDelete = false
                    vm.deleteFromArchive(task.id)
                },
                onDismiss = { confirmDelete = false },
            )
        }
    }
}

/** Tag manager: create, recolour, rename and delete workspace tags. */
@Composable
fun TagManagerModal(state: BoardUiState, vm: BoardViewModel, onDismiss: () -> Unit) {
    val c = Tessera.colors
    var newName by remember { mutableStateOf("") }
    Dialog(onDismissRequest = onDismiss) {
        Column(
            Modifier.fillMaxWidth().clip(RoundedCornerShape(RadiusLg)).background(c.surface).padding(18.dp),
        ) {
            Text("Управление тегами", color = c.text1, fontSize = 16.sp, fontWeight = FontWeight.SemiBold)
            Spacer(Modifier.height(12.dp))

            if (state.tagList.isEmpty()) {
                Text("Тегов пока нет", color = c.text3, fontSize = 13.sp)
            } else {
                Column(Modifier.heightIn(max = 320.dp).verticalScroll(rememberScrollState())) {
                    state.tagList.forEach { tag -> TagRow(tag, vm) }
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

@Composable
private fun TagRow(tag: website.msdnna.tessera.data.model.Tag, vm: BoardViewModel) {
    val c = Tessera.colors
    var paletteOpen by remember { mutableStateOf(false) }
    var renaming by remember { mutableStateOf(false) }
    var confirmDelete by remember { mutableStateOf(false) }

    Column(Modifier.fillMaxWidth().padding(vertical = 6.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Box(Modifier.clickableNoRipple { paletteOpen = !paletteOpen }) {
                ColorDot(parseHexColor(tag.color, c.primary), sizeDp = 16)
            }
            Spacer(Modifier.width(10.dp))
            Text(tag.name, color = c.text1, fontSize = 14.sp, modifier = Modifier.weight(1f))
            IonIconButton(Ion.PENCIL, onClick = { renaming = true }, boxSize = 30.dp, iconSize = 15.dp, tint = c.text3)
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
        if (paletteOpen) {
            FlowRow(
                Modifier.padding(start = 26.dp, top = 6.dp),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                TagPalette.forEach { hex ->
                    val selected = tag.color.equals(hex, ignoreCase = true)
                    Box(
                        Modifier.size(22.dp).clip(CircleShape).background(accentGradient(parseHexColor(hex, c.text3)))
                            .then(if (selected) Modifier.border(2.dp, c.text1, CircleShape) else Modifier)
                            .clickableNoRipple {
                                vm.updateTag(tag.id, tag.name, hex)
                                paletteOpen = false
                            },
                    )
                }
            }
        }
    }

    if (renaming) {
        TInputDialog(
            "Переименовать тег",
            initial = tag.name,
            onConfirm = {
                vm.updateTag(tag.id, it, tag.color)
                renaming = false
            },
            onDismiss = { renaming = false },
        )
    }
}

@Composable
private fun SectionLabel(text: String) {
    Text(text, color = Tessera.colors.text3, fontSize = 12.sp, fontWeight = FontWeight.Medium, modifier = Modifier.padding(bottom = 6.dp))
}

@Composable
private fun Chip(label: String, active: Boolean, color: androidx.compose.ui.graphics.Color? = null, onClick: () -> Unit) {
    val c = Tessera.colors
    val accent = color ?: c.primary
    Box(
        Modifier.clip(RoundedCornerShape(RadiusSm))
            .background(if (active) accentGradient(accent) else SolidColor(c.surfaceAlt))
            .clickableNoRipple(onClick = onClick)
            .padding(horizontal = 12.dp, vertical = 7.dp),
    ) {
        Text(label, color = if (active) c.onPrimary else c.text2, fontSize = 13.sp, fontWeight = FontWeight.Medium)
    }
}

private fun <T> Set<T>.toggle(item: T): Set<T> = if (item in this) this - item else this + item
