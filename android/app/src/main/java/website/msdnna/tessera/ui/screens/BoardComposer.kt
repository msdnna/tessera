package website.msdnna.tessera.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.FlowRowScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
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
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import website.msdnna.tessera.data.model.BoardView
import website.msdnna.tessera.ui.components.IonIcon
import website.msdnna.tessera.ui.components.IonIconButton
import website.msdnna.tessera.ui.components.TButton
import website.msdnna.tessera.ui.components.TConfirmPopover
import website.msdnna.tessera.ui.components.TDropdown
import website.msdnna.tessera.ui.components.TMenuDivider
import website.msdnna.tessera.ui.components.TMenuItem
import website.msdnna.tessera.ui.components.TTextField
import website.msdnna.tessera.ui.components.clickableNoRipple
import website.msdnna.tessera.ui.components.dashedBorder
import website.msdnna.tessera.ui.theme.PriorityLabels
import website.msdnna.tessera.ui.theme.RadiusMd
import website.msdnna.tessera.ui.theme.RadiusSm
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.viewmodels.BoardFilter
import website.msdnna.tessera.ui.viewmodels.BoardUiState
import website.msdnna.tessera.ui.viewmodels.BoardViewModel
import website.msdnna.tessera.ui.viewmodels.DueFilter
import website.msdnna.tessera.ui.viewmodels.SortField
import website.msdnna.tessera.util.Ion

private val DueChipLabels = mapOf(
    DueFilter.Overdue to "Просроченные",
    DueFilter.Today to "Сегодня",
    DueFilter.Week to "Ближайшая неделя",
    DueFilter.Has to "Со сроком",
    DueFilter.None to "Без срока",
)

/** Namespace of a tag name ("S: bug" → "S: ", "effort::small" → "effort::"). */
private fun tagNamespace(name: String): String {
    val i = name.indexOf("::")
    if (i >= 0) return name.substring(0, i + 2)
    val j = name.indexOf(": ")
    if (j >= 0) return name.substring(0, j + 2)
    return ""
}

/**
 * The board composer bar (web `KanbanBoard` parity): grouping / multi-level sort /
 * filters as removable chips, an "add" menu and an inline title search, all in one
 * bordered wrapping bar. Mutations apply to [vm] immediately.
 */
@Composable
fun BoardComposerBar(state: BoardUiState, vm: BoardViewModel, modifier: Modifier = Modifier) {
    val c = Tessera.colors
    val f = state.filter
    FlowRow(
        modifier
            .clip(RoundedCornerShape(RadiusMd))
            .border(1.dp, c.border, RoundedCornerShape(RadiusMd))
            .background(c.surface)
            .padding(horizontal = 8.dp, vertical = 5.dp)
            .heightIn(min = 40.dp),
        horizontalArrangement = Arrangement.spacedBy(6.dp),
        verticalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        GroupChip(state, vm)
        state.sortLevels.forEachIndexed { i, level ->
            val label = SortField.fromKey(level.field)?.label ?: level.field
            val arrow = if (level.dir == "desc") "↓" else "↑"
            FacetChip("Сорт: $label $arrow", onClick = { vm.toggleSortDir(i) }, onRemove = { vm.removeSortLevel(i) })
        }
        f.priorities.sorted().forEach { p ->
            FacetChip(
                "Приоритет: ${PriorityLabels.getOrElse(p) { "—" }}",
                onRemove = { vm.setFilter(f.copy(priorities = f.priorities - p)) },
            )
        }
        f.assigneeIds.forEach { id ->
            FacetChip(
                "Исполнитель: ${state.membersMap[id]?.name ?: "—"}",
                onRemove = { vm.setFilter(f.copy(assigneeIds = f.assigneeIds - id)) },
            )
        }
        f.tagIds.forEach { id ->
            FacetChip(
                "Тег: ${state.tags[id]?.name ?: "—"}",
                onRemove = { vm.setFilter(f.copy(tagIds = f.tagIds - id)) },
            )
        }
        if (f.due != DueFilter.All) {
            FacetChip(
                "Срок: ${DueChipLabels[f.due] ?: ""}",
                onRemove = { vm.setFilter(f.copy(due = DueFilter.All)) },
            )
        }
        AddFacetButton(state, vm)
        if (hasClearable(state)) {
            Box(
                Modifier.clip(CircleShape).clickableNoRipple {
                    vm.clearFilter()
                    vm.clearSort()
                }
                    .padding(horizontal = 4.dp),
                contentAlignment = Alignment.Center,
            ) { Text("×", color = c.text3, fontSize = 16.sp) }
        }
        ComposerSearch(f.query, { vm.setFilter(f.copy(query = it)) }, Modifier.weight(1f))
    }
}

private fun hasClearable(state: BoardUiState): Boolean = state.sortLevels.isNotEmpty() ||
    state.filter.priorities.isNotEmpty() || state.filter.assigneeIds.isNotEmpty() ||
    state.filter.tagIds.isNotEmpty() || state.filter.due != DueFilter.All

/** The always-present grouping chip; its dropdown picks status / all tags / a namespace. */
@Composable
private fun GroupChip(state: BoardUiState, vm: BoardViewModel) {
    val namespaces = remember(state.tagList) {
        state.tagList.mapNotNull { tagNamespace(it.name).ifEmpty { null } }.distinct().sorted()
    }
    val label = if (state.groupByTag) {
        "Группировка: теги" + (if (state.tagPrefix.isNotEmpty()) " · ${state.tagPrefix}" else "")
    } else {
        "Группировка: статусы"
    }
    var menu by remember { mutableStateOf(false) }
    Box {
        FacetChip(label, group = true, onClick = { menu = true })
        TDropdown(expanded = menu, onDismiss = { menu = false }) {
            CheckRow("По статусам", selected = !state.groupByTag) {
                menu = false
                vm.setGrouping(byTag = false)
            }
            CheckRow("По тегам (все)", selected = state.groupByTag && state.tagPrefix.isEmpty()) {
                menu = false
                vm.setGrouping(byTag = true, prefix = "")
            }
            namespaces.forEach { ns ->
                CheckRow("По тегам · $ns", selected = state.groupByTag && state.tagPrefix == ns) {
                    menu = false
                    vm.setGrouping(byTag = true, prefix = ns)
                }
            }
        }
    }
}

/** The dashed "+" button: a two-level menu adding a sort level or a filter facet. */
@Composable
private fun AddFacetButton(state: BoardUiState, vm: BoardViewModel) {
    val c = Tessera.colors
    val f = state.filter
    var menu by remember { mutableStateOf(false) }
    var category by remember { mutableStateOf<String?>(null) }
    val sortFields = SortField.entries.filter { sf -> state.sortLevels.none { it.field == sf.key } }
    fun close() {
        menu = false
        category = null
    }
    Box {
        Box(
            Modifier.size(22.dp).clip(RoundedCornerShape(RadiusSm)).dashedBorder(c.border, RadiusSm)
                .clickableNoRipple { menu = true },
            contentAlignment = Alignment.Center,
        ) { IonIcon(Ion.ADD, size = 14.dp, tint = c.primary, gradient = true) }
        TDropdown(expanded = menu, onDismiss = { close() }) {
            when (category) {
                "sort" -> {
                    BackRow { category = null }
                    sortFields.forEach { sf ->
                        TMenuItem(sf.label, onClick = {
                            vm.addSortLevel(sf)
                            close()
                        })
                    }
                }

                "fp" -> {
                    BackRow { category = null }
                    PriorityLabels.forEachIndexed { i, label ->
                        if (i !in f.priorities) {
                            TMenuItem(label, onClick = {
                                vm.setFilter(f.copy(priorities = f.priorities + i))
                                close()
                            })
                        }
                    }
                }

                "fa" -> {
                    BackRow { category = null }
                    state.members.filter { it.userId !in f.assigneeIds }.forEach { m ->
                        TMenuItem(m.name.ifBlank { m.email }, onClick = {
                            vm.setFilter(f.copy(assigneeIds = f.assigneeIds + m.userId))
                            close()
                        })
                    }
                }

                "ft" -> {
                    BackRow { category = null }
                    state.tagList.filter { it.id !in f.tagIds }.forEach { tag ->
                        TMenuItem(tag.name, onClick = {
                            vm.setFilter(f.copy(tagIds = f.tagIds + tag.id))
                            close()
                        })
                    }
                }

                "fd" -> {
                    BackRow { category = null }
                    DueChipLabels.forEach { (due, label) ->
                        TMenuItem(label, onClick = {
                            vm.setFilter(f.copy(due = due))
                            close()
                        })
                    }
                }

                else -> {
                    if (sortFields.isNotEmpty()) ArrowRow("Сортировка") { category = "sort" }
                    ArrowRow("Фильтр: приоритет") { category = "fp" }
                    if (state.members.isNotEmpty()) ArrowRow("Фильтр: исполнитель") { category = "fa" }
                    if (state.tagList.isNotEmpty()) ArrowRow("Фильтр: тег") { category = "ft" }
                    ArrowRow("Фильтр: срок") { category = "fd" }
                }
            }
        }
    }
}

@Composable
private fun CheckRow(label: String, selected: Boolean, onClick: () -> Unit) {
    TMenuItem(
        label,
        onClick = onClick,
        trailing = {
            if (selected) IonIcon(Ion.CHECK, size = 16.dp, tint = Tessera.colors.primary, gradient = true)
        },
    )
}

@Composable
private fun ArrowRow(label: String, onClick: () -> Unit) {
    TMenuItem(
        label,
        onClick = onClick,
        trailing = { IonIcon(Ion.CHEVRON_FORWARD, size = 14.dp, tint = Tessera.colors.text3) },
    )
}

@Composable
private fun BackRow(onClick: () -> Unit) {
    TMenuItem("‹ Назад", onClick = onClick)
    TMenuDivider()
}

/** A composer chip pill: a label, optional click (group/sort) and remove (×). */
@Composable
private fun FacetChip(
    label: String,
    group: Boolean = false,
    onClick: (() -> Unit)? = null,
    onRemove: (() -> Unit)? = null,
) {
    val c = Tessera.colors
    Row(
        Modifier.clip(RoundedCornerShape(50))
            .background(if (group) c.primary.copy(alpha = 0.14f) else c.hover)
            .then(if (onClick != null) Modifier.clickableNoRipple(onClick = onClick) else Modifier)
            .padding(start = 9.dp, end = if (onRemove != null) 3.dp else 9.dp, top = 3.dp, bottom = 3.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(label, color = if (group) c.text1 else c.text2, fontSize = 12.sp, maxLines = 1)
        if (onRemove != null) {
            Spacer(Modifier.width(2.dp))
            Box(
                Modifier.clip(CircleShape).clickableNoRipple(onClick = onRemove).padding(horizontal = 3.dp),
                contentAlignment = Alignment.Center,
            ) { Text("×", color = c.text3, fontSize = 14.sp) }
        }
    }
}

/** Borderless inline search inside the bar (web `.composer-search`). */
@Composable
private fun FlowRowScope.ComposerSearch(value: String, onValue: (String) -> Unit, modifier: Modifier) {
    val c = Tessera.colors
    BasicTextField(
        value = value,
        onValueChange = onValue,
        singleLine = true,
        textStyle = TextStyle(color = c.text1, fontSize = 13.sp),
        cursorBrush = SolidColor(c.primary),
        modifier = modifier.widthIn(min = 120.dp).padding(horizontal = 4.dp, vertical = 6.dp),
        decorationBox = { inner ->
            if (value.isEmpty()) Text("Поиск по названию…", color = c.text3, fontSize = 13.sp)
            inner()
        },
    )
}

/**
 * The saved-views popover (web load/save popovers merged): apply or delete a
 * server-side view, or save the current toolbar state under a name. Rendered
 * inside the toolbar's views button [TDropdown].
 */
@Composable
fun SavedViewsPopover(state: BoardUiState, vm: BoardViewModel, onClose: () -> Unit) {
    val c = Tessera.colors
    var name by remember { mutableStateOf(state.currentViewName.orEmpty()) }
    Column(Modifier.width(290.dp).heightIn(max = 440.dp).verticalScroll(rememberScrollState()).padding(14.dp)) {
        Text("Представления", color = c.text3, fontSize = 12.sp, fontWeight = FontWeight.Medium)
        Spacer(Modifier.height(8.dp))
        if (state.savedViews.isEmpty()) {
            Text("Нет сохранённых представлений", color = c.text3, fontSize = 13.sp)
        } else {
            state.savedViews.forEach { view ->
                SavedViewRow(view, state.currentViewName == view.name, vm, onClose)
            }
        }
        Spacer(Modifier.height(12.dp))
        HorizontalDivider(color = c.border)
        Spacer(Modifier.height(12.dp))
        Text("Сохранить текущий вид", color = c.text3, fontSize = 12.sp, fontWeight = FontWeight.Medium)
        Spacer(Modifier.height(6.dp))
        Row(verticalAlignment = Alignment.CenterVertically) {
            Box(Modifier.weight(1f)) {
                TTextField(value = name, onValueChange = { name = it }, placeholder = "Название")
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
private fun SavedViewRow(view: BoardView, current: Boolean, vm: BoardViewModel, onClose: () -> Unit) {
    val c = Tessera.colors
    var confirmDelete by remember { mutableStateOf(false) }
    Row(Modifier.fillMaxWidth().padding(vertical = 6.dp), verticalAlignment = Alignment.CenterVertically) {
        if (current) {
            IonIcon(Ion.CHECK, size = 15.dp, tint = c.primary, gradient = true)
            Spacer(Modifier.width(8.dp))
        }
        Text(
            view.name,
            color = if (current) c.primary else c.text1,
            fontSize = 14.sp,
            fontWeight = if (current) FontWeight.Medium else FontWeight.Normal,
            modifier = Modifier.weight(1f).clickableNoRipple {
                vm.applyView(view)
                onClose()
            },
        )
        Box {
            IonIconButton(Ion.TRASH, onClick = { confirmDelete = true }, boxSize = 30.dp, iconSize = 15.dp, tint = c.text3)
            TConfirmPopover(
                expanded = confirmDelete,
                message = "Удалить представление «${view.name}»?",
                onConfirm = {
                    confirmDelete = false
                    vm.deleteView(view)
                },
                onDismiss = { confirmDelete = false },
            )
        }
    }
}
