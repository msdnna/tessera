package website.msdnna.tessera.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.FlowRowOverflow
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
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.layout.IntrinsicMeasurable
import androidx.compose.ui.layout.IntrinsicMeasureScope
import androidx.compose.ui.layout.LayoutModifier
import androidx.compose.ui.layout.Measurable
import androidx.compose.ui.layout.MeasureResult
import androidx.compose.ui.layout.MeasureScope
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Constraints
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
import website.msdnna.tessera.ui.viewmodels.BoardViewMode
import website.msdnna.tessera.ui.viewmodels.BoardViewModel
import website.msdnna.tessera.ui.viewmodels.DueFilter
import website.msdnna.tessera.ui.viewmodels.SortField
import website.msdnna.tessera.util.Ion
import website.msdnna.tessera.util.buildTagGroups
import website.msdnna.tessera.util.prefixLabel
import website.msdnna.tessera.util.tagNamespace

private val DueChipLabels = mapOf(
    DueFilter.Overdue to "Просроченные",
    DueFilter.Today to "Сегодня",
    DueFilter.Week to "Ближайшая неделя",
    DueFilter.Has to "Со сроком",
    DueFilter.None to "Без срока",
)

/**
 * Hides this node's (expensive) intrinsic *width* from an enclosing FlowRow,
 * reporting 0 instead of delegating to the child.
 *
 * The composer's inline search ([ComposerSearch]) is a `BasicTextField` placed in
 * the chips [FlowRow] with `Modifier.weight(1f)`. A weighted child in a
 * `maxLines`-limited FlowRow makes FlowRow *intrinsic-measure* that child on every
 * layout pass, and a `BasicTextField`'s intrinsic-width query re-runs a full text
 * layout (horizontal-scroll + height-in-lines text measurement). Once the leading
 * chips grow wide enough to squeeze the field — e.g. a long tag-namespace grouping
 * label like "Группировка: теги · effort::" — those nested intrinsic passes peg the
 * main thread and ANR the app (frozen board, dead loader). Confirmed from a device
 * ANR: MultiContentMeasurePolicy → DefaultIntrinsicMeasurable ×8 → TextFieldSizeNode
 * → BoringLayout.isBoring.
 *
 * Width is the flow's main axis, so a constant 0 is all the weight pass needs (the
 * field still gets the line's leftover space via its weight). Real measurement and
 * the cross-axis (height) intrinsics delegate as normal, so the rendered layout is
 * unchanged.
 */
private fun Modifier.zeroIntrinsicWidth(): Modifier = this.then(
    object : LayoutModifier {
        override fun MeasureScope.measure(measurable: Measurable, constraints: Constraints): MeasureResult {
            val placeable = measurable.measure(constraints)
            return layout(placeable.width, placeable.height) { placeable.place(0, 0) }
        }

        override fun IntrinsicMeasureScope.minIntrinsicWidth(measurable: IntrinsicMeasurable, height: Int) = 0
        override fun IntrinsicMeasureScope.maxIntrinsicWidth(measurable: IntrinsicMeasurable, height: Int) = 0
        override fun IntrinsicMeasureScope.minIntrinsicHeight(measurable: IntrinsicMeasurable, width: Int) =
            measurable.minIntrinsicHeight(width)

        override fun IntrinsicMeasureScope.maxIntrinsicHeight(measurable: IntrinsicMeasurable, width: Int) =
            measurable.maxIntrinsicHeight(width)
    },
)

/**
 * The board composer bar (web `KanbanBoard` parity): grouping / multi-level sort /
 * filters as removable chips, an "add" menu and an inline title search, all in one
 * bordered wrapping bar. Mutations apply to [vm] immediately.
 *
 * Collapsed it clips to a single row so the right-side tools stay aligned; tapping
 * the bar [expanded]s it to full height (the tools slide off in [BoardToolbar]).
 * A bottom-right corner cluster carries the clear-all (×) and the expand chevron.
 */
@Composable
fun BoardComposerBar(
    state: BoardUiState,
    vm: BoardViewModel,
    expanded: Boolean,
    setExpanded: (Boolean) -> Unit,
    modifier: Modifier = Modifier,
) {
    val c = Tessera.colors
    val f = state.filter
    val clearable = hasClearable(state)
    Box(
        modifier
            // The bar's 36dp min height + decoration live on the Box (matching the
            // 36dp tool buttons); the FlowRow is centred within it so a single
            // collapsed row sits vertically centred, like the clear-× on the right.
            .heightIn(min = 36.dp)
            .clip(RoundedCornerShape(RadiusMd))
            .border(1.dp, c.border, RoundedCornerShape(RadiusMd))
            .background(c.surface)
            .clickableNoRipple { setExpanded(true) },
    ) {
        FlowRow(
            Modifier
                .fillMaxWidth()
                .align(Alignment.Center)
                // Dim the bar's content while collapsed/unfocused so it reads as one
                // tap-to-expand surface (mirrors the web composer).
                .alpha(if (expanded) 1f else 0.62f)
                .padding(start = 8.dp, top = 8.dp, bottom = 8.dp, end = if (clearable) 28.dp else 8.dp),
            maxLines = if (expanded) Int.MAX_VALUE else 1,
            overflow = FlowRowOverflow.Clip,
            // Inter-chip and inter-row gaps match the bar's 8dp edge padding so a
            // multi-row (expanded / overflowing) bar isn't cramped vertically.
            horizontalArrangement = Arrangement.spacedBy(8.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
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
            f.statuses.forEach { id ->
                FacetChip(
                    "Статус: ${state.sortedColumns.find { it.id == id }?.name ?: "—"}",
                    onRemove = { vm.setFilter(f.copy(statuses = f.statuses - id)) },
                )
            }
            if (f.due != DueFilter.All) {
                FacetChip(
                    "Срок: ${DueChipLabels[f.due] ?: ""}",
                    onRemove = { vm.setFilter(f.copy(due = DueFilter.All)) },
                )
            }
            AddFacetButton(state, vm)
            ComposerSearch(f.query, { vm.setFilter(f.copy(query = it)) }, Modifier.weight(1f).zeroIntrinsicWidth())
        }
        // Right-edge clear-all (×) — vertically centred like the web composer.
        // Expand/collapse is driven by tapping the bar / tapping outside it.
        if (clearable) {
            Box(
                Modifier.align(Alignment.CenterEnd).padding(end = 6.dp).clip(CircleShape)
                    .clickableNoRipple {
                        vm.clearFilter()
                        vm.clearSort()
                    }
                    .padding(horizontal = 3.dp),
                contentAlignment = Alignment.Center,
            ) { Text("×", color = c.text3, fontSize = 16.sp) }
        }
        // Collapsed: a transparent overlay covers the whole bar so a tap ANYWHERE
        // (including on a chip / add / clear) only expands it — fishing for a blank
        // spot to expand a chip-filled bar was fiddly. The chips' own taps fire
        // only once expanded (overlay gone). Outside-tap collapse is handled by the
        // scrim in BoardScreen.
        if (!expanded) {
            Box(Modifier.matchParentSize().clickableNoRipple { setExpanded(true) })
        }
    }
}

private fun hasClearable(state: BoardUiState): Boolean = state.sortLevels.isNotEmpty() ||
    state.filter.priorities.isNotEmpty() || state.filter.assigneeIds.isNotEmpty() ||
    state.filter.tagIds.isNotEmpty() || state.filter.statuses.isNotEmpty() || state.filter.due != DueFilter.All

/** The always-present grouping chip; its dropdown picks status / all tags / a namespace. */
@Composable
private fun GroupChip(state: BoardUiState, vm: BoardViewModel) {
    // Distinct namespaces present in the tags, labelled by their friendly name and
    // sorted by that label (web `tagPrefixOptions`).
    val namespaces = remember(state.tagList, state.prefixNames) {
        state.tagList.mapNotNull { tagNamespace(it.name).ifEmpty { null } }.distinct()
            .map { it to prefixLabel(it, state.prefixNames) }
            .sortedBy { it.second.lowercase() }
    }
    // Assignee / no-grouping are only meaningful on the swimlane (timeline/Gantt) views.
    val timelineLike = state.viewMode == BoardViewMode.Timeline || state.viewMode == BoardViewMode.Gantt
    val label = "Группировка: " + when (state.groupMode) {
        "tag" -> "теги" + (if (state.tagPrefix.isNotEmpty()) " · ${prefixLabel(state.tagPrefix, state.prefixNames)}" else "")
        "assignee" -> "исполнитель"
        "none" -> "без"
        else -> "статусы"
    }
    var menu by remember { mutableStateOf(false) }
    Box {
        FacetChip(label, group = true, onClick = { menu = true })
        TDropdown(expanded = menu, onDismiss = { menu = false }, scrollable = true) {
            CheckRow("По статусам", selected = state.groupMode == "status") {
                menu = false
                vm.setGrouping("status")
            }
            CheckRow("По тегам (все)", selected = state.groupMode == "tag" && state.tagPrefix.isEmpty()) {
                menu = false
                vm.setGrouping("tag", prefix = "")
            }
            namespaces.forEach { (ns, nsLabel) ->
                CheckRow("По тегам · $nsLabel", selected = state.groupMode == "tag" && state.tagPrefix == ns) {
                    menu = false
                    vm.setGrouping("tag", prefix = ns)
                }
            }
            if (timelineLike) {
                CheckRow("По исполнителю", selected = state.groupMode == "assignee") {
                    menu = false
                    vm.setGrouping("assignee")
                }
                CheckRow("Без группировки", selected = state.groupMode == "none") {
                    menu = false
                    vm.setGrouping("none")
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
    // Status sort/filter is offered only on the time-axis views — timeline & gantt
    // (the board already groups by status into columns, so it's redundant there).
    val timeline = state.viewMode == BoardViewMode.Timeline || state.viewMode == BoardViewMode.Gantt
    val sortFields = SortField.entries.filter { sf ->
        state.sortLevels.none { it.field == sf.key } && (sf != SortField.Status || timeline)
    }
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
        TDropdown(expanded = menu, onDismiss = { close() }, scrollable = true) {
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
                    // Group the pickable tags by prefix; show section headers only
                    // when more than one group exists (web «Фильтр: тег»).
                    val groups = buildTagGroups(state.tagList.filter { it.id !in f.tagIds }, state.prefixNames)
                    val headers = groups.size > 1
                    groups.forEach { g ->
                        if (headers) MenuSectionHeader(g.label)
                        g.tags.forEach { tag ->
                            TMenuItem(tag.name, onClick = {
                                vm.setFilter(f.copy(tagIds = f.tagIds + tag.id))
                                close()
                            })
                        }
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

                "fs" -> {
                    BackRow { category = null }
                    state.sortedColumns.filter { it.id !in f.statuses }.forEach { col ->
                        TMenuItem(col.name, onClick = {
                            vm.setFilter(f.copy(statuses = f.statuses + col.id))
                            close()
                        })
                    }
                }

                else -> {
                    if (sortFields.isNotEmpty()) ArrowRow("Сортировка") { category = "sort" }
                    if (timeline && state.sortedColumns.isNotEmpty()) ArrowRow("Фильтр: статус") { category = "fs" }
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

/** A non-interactive section caption inside a dropdown (groups tags by prefix). */
@Composable
private fun MenuSectionHeader(label: String) {
    Text(
        label.uppercase(),
        color = Tessera.colors.text3,
        fontSize = 10.sp,
        fontWeight = FontWeight.SemiBold,
        letterSpacing = 0.4.sp,
        modifier = Modifier.padding(start = 12.dp, end = 12.dp, top = 8.dp, bottom = 2.dp),
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
        modifier = modifier.widthIn(min = 120.dp).padding(horizontal = 4.dp, vertical = 4.dp),
        decorationBox = { inner ->
            // Single line + ellipsis: a narrow leftover width (e.g. next to a wide
            // group chip) must truncate the hint, not wrap it and grow the bar.
            if (value.isEmpty()) {
                Text("Поиск…", color = c.text3, fontSize = 13.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
            }
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
