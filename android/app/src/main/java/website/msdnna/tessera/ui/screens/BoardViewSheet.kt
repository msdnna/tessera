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
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import website.msdnna.tessera.data.model.BoardView
import website.msdnna.tessera.ui.components.IonIcon
import website.msdnna.tessera.ui.components.IonIconButton
import website.msdnna.tessera.ui.components.TBottomSheet
import website.msdnna.tessera.ui.components.TButton
import website.msdnna.tessera.ui.components.TConfirmPopover
import website.msdnna.tessera.ui.components.TDropdown
import website.msdnna.tessera.ui.components.TMenuItem
import website.msdnna.tessera.ui.components.TTextField
import website.msdnna.tessera.ui.components.clickableNoRipple
import website.msdnna.tessera.ui.theme.RadiusSm
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.accentGradient
import website.msdnna.tessera.ui.viewmodels.BoardUiState
import website.msdnna.tessera.ui.viewmodels.BoardViewModel
import website.msdnna.tessera.ui.viewmodels.SortField
import website.msdnna.tessera.util.Ion

/** Namespace of a tag name ("S: bug" → "S: ", "effort::small" → "effort::"). */
private fun tagNamespace(name: String): String {
    val i = name.indexOf("::")
    if (i >= 0) return name.substring(0, i + 2)
    val j = name.indexOf(": ")
    if (j >= 0) return name.substring(0, j + 2)
    return ""
}

/**
 * The "Вид" bottom sheet (mobile-adapted composer): grouping (status / all tags /
 * tag namespaces) and the multi-level sort editor. Applies immediately to the board.
 */
@Composable
fun BoardViewSheet(state: BoardUiState, vm: BoardViewModel, onDismiss: () -> Unit) {
    TBottomSheet(onDismiss = onDismiss) {
        Column(Modifier.heightIn(max = 520.dp).verticalScroll(rememberScrollState())) {
            GroupingSection(state, vm)
            Spacer(Modifier.height(18.dp))
            SortSection(state, vm)
        }
    }
}

@Composable
private fun GroupingSection(state: BoardUiState, vm: BoardViewModel) {
    val namespaces = remember(state.tagList) {
        state.tagList.mapNotNull { tagNamespace(it.name).ifEmpty { null } }.distinct().sorted()
    }
    SectionLabel("Группировка")
    FlowRow(
        horizontalArrangement = Arrangement.spacedBy(8.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        SelectChip("Статус", active = !state.groupByTag) { vm.setGrouping(byTag = false) }
        SelectChip("Все теги", active = state.groupByTag && state.tagPrefix.isEmpty()) {
            vm.setGrouping(byTag = true, prefix = "")
        }
        namespaces.forEach { ns ->
            SelectChip(ns, active = state.groupByTag && state.tagPrefix == ns) {
                vm.setGrouping(byTag = true, prefix = ns)
            }
        }
    }
}

@Composable
private fun SortSection(state: BoardUiState, vm: BoardViewModel) {
    val c = Tessera.colors
    Row(verticalAlignment = Alignment.CenterVertically) {
        Box(Modifier.weight(1f)) { SectionLabel("Сортировка") }
        if (state.sortLevels.isNotEmpty()) {
            Text(
                "Сбросить",
                color = c.primary,
                fontSize = 13.sp,
                fontWeight = FontWeight.Medium,
                modifier = Modifier.clickableNoRipple { vm.clearSort() }.padding(4.dp),
            )
        }
    }
    if (state.sortLevels.isEmpty()) {
        Text("Вручную (порядок карточек)", color = c.text3, fontSize = 13.sp, modifier = Modifier.padding(vertical = 2.dp))
    } else {
        state.sortLevels.forEachIndexed { i, level ->
            val field = SortField.fromKey(level.field)
            Row(
                Modifier.fillMaxWidth().padding(vertical = 4.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text("${i + 1}", color = c.text3, fontSize = 12.sp, modifier = Modifier.width(18.dp))
                Text(field?.label ?: level.field, color = c.text1, fontSize = 14.sp, modifier = Modifier.weight(1f))
                // Direction toggle: ↑ ascending / ↓ descending.
                Box(
                    Modifier.clip(RoundedCornerShape(RadiusSm)).background(c.surfaceAlt)
                        .clickableNoRipple { vm.toggleSortDir(i) }
                        .padding(horizontal = 10.dp, vertical = 5.dp),
                ) {
                    Text(
                        if (level.dir == "desc") "↓ убыв." else "↑ возр.",
                        color = c.text2,
                        fontSize = 12.sp,
                        fontWeight = FontWeight.Medium,
                    )
                }
                Spacer(Modifier.width(4.dp))
                IonIconButton(Ion.CLOSE, onClick = { vm.removeSortLevel(i) }, boxSize = 30.dp, iconSize = 14.dp, tint = c.text3)
            }
        }
    }
    Spacer(Modifier.height(8.dp))
    AddSortLevel(state, vm)
}

@Composable
private fun AddSortLevel(state: BoardUiState, vm: BoardViewModel) {
    val c = Tessera.colors
    val available = SortField.entries.filter { f -> state.sortLevels.none { it.field == f.key } }
    if (available.isEmpty()) return
    var menu by remember { mutableStateOf(false) }
    Box {
        Row(
            Modifier.clip(RoundedCornerShape(RadiusSm)).clickableNoRipple { menu = true }
                .padding(vertical = 4.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            IonIcon(Ion.ADD, size = 16.dp, tint = c.primary, gradient = true)
            Spacer(Modifier.width(6.dp))
            Text("Добавить уровень", color = c.primary, fontSize = 13.sp, fontWeight = FontWeight.Medium)
        }
        TDropdown(expanded = menu, onDismiss = { menu = false }) {
            available.forEach { field ->
                TMenuItem(field.label, onClick = {
                    menu = false
                    vm.addSortLevel(field)
                })
            }
        }
    }
}

/**
 * The "Представления" bottom sheet: apply / delete saved server-side views and
 * save the current toolbar state under a name (web save/load popovers).
 */
@Composable
fun SavedViewsSheet(state: BoardUiState, vm: BoardViewModel, onDismiss: () -> Unit) {
    val c = Tessera.colors
    var name by remember { mutableStateOf(state.currentViewName.orEmpty()) }
    TBottomSheet(onDismiss = onDismiss) {
        SectionLabel("Представления")
        if (state.savedViews.isEmpty()) {
            Text("Сохранённых видов пока нет", color = c.text3, fontSize = 13.sp, modifier = Modifier.padding(vertical = 2.dp))
        } else {
            Column(Modifier.heightIn(max = 280.dp).verticalScroll(rememberScrollState())) {
                state.savedViews.forEach { view -> SavedViewRow(view, state.currentViewName == view.name, vm, onDismiss) }
            }
        }
        Spacer(Modifier.height(14.dp))
        HorizontalDivider(color = c.border)
        Spacer(Modifier.height(14.dp))
        Text("Сохранить текущий вид", color = c.text3, fontSize = 12.sp, fontWeight = FontWeight.Medium)
        Spacer(Modifier.height(6.dp))
        Row(verticalAlignment = Alignment.CenterVertically) {
            Box(Modifier.weight(1f)) {
                TTextField(value = name, onValueChange = { name = it }, placeholder = "Название вида")
            }
            Spacer(Modifier.width(8.dp))
            TButton(
                "Сохранить",
                enabled = name.isNotBlank(),
                onClick = {
                    vm.saveView(name.trim())
                    name = ""
                },
            )
        }
    }
}

@Composable
private fun SavedViewRow(view: BoardView, current: Boolean, vm: BoardViewModel, onDismiss: () -> Unit) {
    val c = Tessera.colors
    var confirmDelete by remember { mutableStateOf(false) }
    Row(
        Modifier.fillMaxWidth().padding(vertical = 6.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        if (current) {
            IonIcon(Ion.CHECK, size = 16.dp, tint = c.primary, gradient = true)
            Spacer(Modifier.width(8.dp))
        }
        Text(
            view.name,
            color = if (current) c.primary else c.text1,
            fontSize = 14.sp,
            fontWeight = if (current) FontWeight.Medium else FontWeight.Normal,
            modifier = Modifier.weight(1f).clickableNoRipple {
                vm.applyView(view)
                onDismiss()
            },
        )
        Box {
            IonIconButton(Ion.TRASH, onClick = { confirmDelete = true }, boxSize = 30.dp, iconSize = 15.dp, tint = c.text3)
            TConfirmPopover(
                expanded = confirmDelete,
                message = "Удалить вид «${view.name}»?",
                onConfirm = {
                    confirmDelete = false
                    vm.deleteView(view)
                },
                onDismiss = { confirmDelete = false },
            )
        }
    }
}

@Composable
private fun SectionLabel(text: String) {
    Text(
        text,
        color = Tessera.colors.text3,
        fontSize = 12.sp,
        fontWeight = FontWeight.Medium,
        modifier = Modifier.padding(bottom = 8.dp),
    )
}

@Composable
private fun SelectChip(label: String, active: Boolean, color: Color? = null, onClick: () -> Unit) {
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
