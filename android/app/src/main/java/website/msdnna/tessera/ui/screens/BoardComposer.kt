package website.msdnna.tessera.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.gestures.detectDragGesturesAfterLongPress
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
import androidx.compose.foundation.layout.wrapContentHeight
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.composed
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.hapticfeedback.HapticFeedback
import androidx.compose.ui.hapticfeedback.HapticFeedbackType
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.layout.IntrinsicMeasurable
import androidx.compose.ui.layout.IntrinsicMeasureScope
import androidx.compose.ui.layout.LayoutModifier
import androidx.compose.ui.layout.Measurable
import androidx.compose.ui.layout.MeasureResult
import androidx.compose.ui.layout.MeasureScope
import androidx.compose.ui.layout.boundsInWindow
import androidx.compose.ui.layout.onGloballyPositioned
import androidx.compose.ui.platform.LocalHapticFeedback
import androidx.compose.ui.platform.LocalResources
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringArrayResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Constraints
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.zIndex
import website.msdnna.tessera.R
import website.msdnna.tessera.data.model.BoardView
import website.msdnna.tessera.ui.TestTags
import website.msdnna.tessera.ui.components.IonIcon
import website.msdnna.tessera.ui.components.IonIconButton
import website.msdnna.tessera.ui.components.MemberAvatar
import website.msdnna.tessera.ui.components.TButton
import website.msdnna.tessera.ui.components.TConfirmPopover
import website.msdnna.tessera.ui.components.TDropdown
import website.msdnna.tessera.ui.components.TMenuDivider
import website.msdnna.tessera.ui.components.TMenuItem
import website.msdnna.tessera.ui.components.TTextField
import website.msdnna.tessera.ui.components.clickableNoRipple
import website.msdnna.tessera.ui.components.dashedBorder
import website.msdnna.tessera.ui.theme.ConflictAmber
import website.msdnna.tessera.ui.theme.RadiusMd
import website.msdnna.tessera.ui.theme.RadiusSm
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.viewmodels.BoardUiState
import website.msdnna.tessera.ui.viewmodels.BoardViewMode
import website.msdnna.tessera.ui.viewmodels.BoardViewModel
import website.msdnna.tessera.ui.viewmodels.SortField
import website.msdnna.tessera.util.BoardFilter
import website.msdnna.tessera.util.DueFilter
import website.msdnna.tessera.util.GitlabAuthor
import website.msdnna.tessera.util.Ion
import website.msdnna.tessera.util.boardGitlabAuthors
import website.msdnna.tessera.util.buildTagGroups
import website.msdnna.tessera.util.columnCaption
import website.msdnna.tessera.util.prefixLabel
import website.msdnna.tessera.util.tagNamespace

// Карта уровня файла держит id ресурсов, а не готовые подписи: со строками фильтр
// «срок» застыл бы на языке первого рендера и не пережил смену языка в профиле.
private val DueChipLabels = mapOf(
    DueFilter.Overdue to R.string.due_filter_overdue,
    DueFilter.Today to R.string.due_filter_today,
    DueFilter.Week to R.string.due_filter_week,
    DueFilter.Has to R.string.due_filter_has,
    DueFilter.None to R.string.due_filter_none,
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
    onExitArchive: () -> Unit = {},
    modifier: Modifier = Modifier,
) {
    val c = Tessera.colors
    val barRes = LocalResources.current
    val f = state.filter
    val clearable = hasClearable(state)
    val sortDrag = remember { SortDragState() }
    Box(
        modifier
            // Collapsed the bar is pinned to exactly one chip row and clips the rest;
            // expanded it grows with its content but never below the 36dp tool buttons.
            // Pinning the *height* (rather than capping FlowRow's `maxLines`) is
            // deliberate — see [ComposerRowHeight].
            .then(if (expanded) Modifier.heightIn(min = 36.dp) else Modifier.height(ComposerRowHeight))
            .clip(RoundedCornerShape(RadiusMd))
            .border(1.dp, c.border, RoundedCornerShape(RadiusMd))
            .background(c.surface)
            .clickableNoRipple { setExpanded(true) },
    ) {
        FlowRow(
            Modifier
                .fillMaxWidth()
                // Collapsed rows past the first are clipped away by the Box, so the
                // content has to hang from the top; expanded, a short bar centres.
                .align(if (expanded) Alignment.Center else Alignment.TopStart)
                // Dim the bar's content while collapsed/unfocused so it reads as one
                // tap-to-expand surface (mirrors the web composer).
                .alpha(if (expanded) 1f else 0.62f)
                .padding(start = 8.dp, top = 8.dp, bottom = 8.dp, end = if (clearable) 28.dp else 8.dp),
            // Inter-chip and inter-row gaps match the bar's 8dp edge padding so a
            // multi-row (expanded / overflowing) bar isn't cramped vertically.
            horizontalArrangement = Arrangement.spacedBy(8.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            // Scope chip, always first: archive (amber, web `.facet-archive`) or sprint
            // (accent, web `.facet-accent`). They are mutually exclusive — the archive is
            // a board-wide read-only scope that ignores the sprint narrowing, so showing
            // both would claim a milestone filter that the archive listing doesn't apply.
            if (state.archivedMode) {
                FacetChip(
                    stringResource(R.string.composer_scope_archive),
                    icon = Ion.ARCHIVE,
                    amber = true,
                    onRemove = onExitArchive,
                )
            } else {
                // The board is server-side narrowed to one milestone; the chip reads as
                // the scope the rest of the chips filter *within*, and × drops it and
                // reloads the full board.
                state.milestoneScope?.let { scope ->
                    FacetChip(
                        milestoneScopeLabel(scope, state),
                        icon = Ion.RIBBON,
                        accent = true,
                        onRemove = { vm.setMilestoneScope(null) },
                    )
                }
            }
            // Subtask expansion (web `.subtasks-chip`): icon-only, right after the
            // scope chip, accent-tinted while cards show their subtasks expanded.
            FacetChip(
                "",
                icon = Ion.GIT_BRANCH,
                accent = state.subtasksExpanded,
                onClick = { vm.toggleSubtasksExpanded() },
            )
            GroupChip(state, vm)
            SortChips(state, vm, sortDrag, enabled = expanded)
            f.priorities.sorted().forEach { p ->
                FacetChip(
                    stringArrayResource(R.array.task_priority_labels).getOrElse(p) { "—" },
                    icon = Ion.FLAG,
                    onRemove = { vm.setFilter(f.copy(priorities = f.priorities - p)) },
                )
            }
            f.assigneeIds.forEach { id ->
                val name = if (id.startsWith("gl:")) {
                    val u = id.removePrefix("gl:")
                    state.gitlabMembers.find { it.glUsername == u }?.let { it.glName.ifBlank { it.glUsername } } ?: u
                } else {
                    state.membersMap[id]?.name ?: "—"
                }
                FacetChip(
                    name,
                    icon = Ion.PERSON,
                    onRemove = { vm.setFilter(f.copy(assigneeIds = f.assigneeIds - id)) },
                )
            }
            f.authorIds.forEach { id ->
                FacetChip(
                    authorLabel(id, state),
                    icon = Ion.PENCIL,
                    onRemove = { vm.setFilter(f.copy(authorIds = f.authorIds - id)) },
                )
            }
            f.tagIds.forEach { id ->
                FacetChip(
                    state.tags[id]?.name ?: "—",
                    icon = Ion.PRICETAG,
                    onRemove = { vm.setFilter(f.copy(tagIds = f.tagIds - id)) },
                )
            }
            f.statuses.forEach { id ->
                FacetChip(
                    state.sortedColumns.find { it.id == id }?.let { columnCaption(barRes, it) } ?: "—",
                    icon = Ion.LIST,
                    onRemove = { vm.setFilter(f.copy(statuses = f.statuses - id)) },
                )
            }
            f.milestoneIds.forEach { id ->
                FacetChip(
                    if (id == "__none__") {
                        stringResource(R.string.task_milestone_none)
                    } else {
                        state.milestonesMap[id]?.title ?: "—"
                    },
                    icon = Ion.RIBBON,
                    onRemove = { vm.setFilter(f.copy(milestoneIds = f.milestoneIds - id)) },
                )
            }
            if (f.due != DueFilter.All) {
                FacetChip(
                    DueChipLabels[f.due]?.let { stringResource(it) } ?: "",
                    icon = Ion.CALENDAR,
                    onRemove = { vm.setFilter(f.copy(due = DueFilter.All)) },
                )
            }
            AddFacetButton(state, vm)
            // Collapsed the field is inert anyway (the overlay below eats its taps),
            // so it is swapped for a plain label — that keeps the weighted text field
            // out of the layout while the bar is collapsing. See [ComposerRowHeight].
            if (expanded) {
                ComposerSearch(f.query, { vm.setFilter(f.copy(query = it)) }, Modifier.weight(1f).zeroIntrinsicWidth())
            } else {
                CollapsedSearchLabel(f.query)
            }
        }
        // Right-edge clear-all (×) — vertically centred like the web composer.
        // Expand/collapse is driven by tapping the bar / tapping outside it.
        if (clearable) {
            Box(
                Modifier.align(Alignment.CenterEnd).padding(end = 6.dp).clip(CircleShape)
                    .clickableNoRipple { vm.clearComposer() }
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
            Box(
                Modifier.matchParentSize()
                    .testTag(TestTags.BOARD_COMPOSER_EXPAND)
                    .clickableNoRipple { setExpanded(true) },
            )
        }
    }
}

// The sprint scope counts as clearable: it lives in the bar as a chip, so the ×
// that "clears the bar" has to drop it too (else the board stays narrowed).
private fun hasClearable(state: BoardUiState): Boolean = state.sortLevels.isNotEmpty() ||
    // …but only while its chip is on screen: in the archive the scope chip gives way to
    // the archive one, and a × for something invisible reads as a stuck bar.
    (state.milestoneScope != null && !state.archivedMode) ||
    state.filter.priorities.isNotEmpty() || state.filter.assigneeIds.isNotEmpty() ||
    state.filter.authorIds.isNotEmpty() || state.filter.tagIds.isNotEmpty() ||
    state.filter.statuses.isNotEmpty() || state.filter.milestoneIds.isNotEmpty() ||
    state.filter.due != DueFilter.All

/** Label for the server-side sprint scope chip (web `milestoneScopeLabel`). */
@Composable
private fun milestoneScopeLabel(scope: String, state: BoardUiState): String = when (scope) {
    "backlog" -> stringResource(R.string.sidebar_backlog)
    else -> state.milestonesMap[scope]?.title ?: stringResource(R.string.composer_scope_milestone)
}

/** Display name for an author-facet value: a workspace member, a GitLab member, or —
 *  for an issue opened by someone outside the roster — the name the synced task carries. */
private fun authorLabel(id: String, state: BoardUiState): String {
    if (!id.startsWith("gl:")) return state.membersMap[id]?.name ?: "—"
    val login = id.removePrefix("gl:")
    state.gitlabMembers.find { it.glUsername == login }?.let { return it.glName.ifBlank { it.glUsername } }
    return boardGitlabAuthors(state.tasks).find { it.username == login }?.name ?: login
}

/** The always-present grouping chip; its dropdown picks status / all tags / a namespace. */
@Composable
private fun GroupChip(state: BoardUiState, vm: BoardViewModel) {
    // Distinct namespaces present in the tags, labelled by their friendly name and
    // sorted by that label (web `tagPrefixOptions`).
    val res = LocalResources.current
    val namespaces = remember(state.tagList, state.prefixNames, res) {
        state.tagList.mapNotNull { tagNamespace(it.name).ifEmpty { null } }.distinct()
            .map { it to prefixLabel(res, it, state.prefixNames) }
            .sortedBy { it.second.lowercase() }
    }
    // Assignee / no-grouping are only meaningful on the swimlane (timeline/Gantt) views.
    val timelineLike = state.viewMode == BoardViewMode.Timeline || state.viewMode == BoardViewMode.Gantt
    val label = when (state.groupMode) {
        "tag" -> stringResource(R.string.composer_group_tag) +
            (if (state.tagPrefix.isNotEmpty()) " · ${prefixLabel(res, state.tagPrefix, state.prefixNames)}" else "")

        "milestone" -> stringResource(R.string.composer_group_milestone)

        "assignee" -> stringResource(R.string.composer_group_assignee)

        "none" -> stringResource(R.string.composer_group_none)

        else -> stringResource(R.string.composer_group_status)
    }
    var menu by remember { mutableStateOf(false) }
    Box {
        FacetChip(
            label,
            icon = Ion.ALBUMS,
            group = true,
            onClick = { menu = true },
            modifier = Modifier.testTag(TestTags.BOARD_GROUP),
        )
        TDropdown(expanded = menu, onDismiss = { menu = false }, scrollable = true) {
            CheckRow(
                stringResource(R.string.composer_group_by_status),
                selected = state.groupMode == "status",
                tag = TestTags.BOARD_GROUP_STATUS,
            ) {
                menu = false
                vm.setGrouping("status")
            }
            CheckRow(
                stringResource(R.string.composer_group_by_tags_all),
                selected = state.groupMode == "tag" && state.tagPrefix.isEmpty(),
                tag = TestTags.BOARD_GROUP_TAGS,
            ) {
                menu = false
                vm.setGrouping("tag", prefix = "")
            }
            namespaces.forEach { (ns, nsLabel) ->
                CheckRow(
                    stringResource(R.string.composer_group_by_tags_prefix, nsLabel),
                    selected = state.groupMode == "tag" && state.tagPrefix == ns,
                    tag = TestTags.boardGroupTagPrefix(ns),
                ) {
                    menu = false
                    vm.setGrouping("tag", prefix = ns)
                }
            }
            // «По этапам» — only when the project actually has milestones.
            if (state.milestones.isNotEmpty()) {
                CheckRow(
                    stringResource(R.string.composer_group_by_milestone),
                    selected = state.groupMode == "milestone",
                ) {
                    menu = false
                    vm.setGrouping("milestone")
                }
            }
            if (timelineLike) {
                CheckRow(
                    stringResource(R.string.composer_group_by_assignee),
                    selected = state.groupMode == "assignee",
                ) {
                    menu = false
                    vm.setGrouping("assignee")
                }
                CheckRow(stringResource(R.string.composer_group_by_none), selected = state.groupMode == "none") {
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
    val res = LocalResources.current
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
                        TMenuItem(stringResource(sf.labelRes), onClick = {
                            vm.addSortLevel(sf)
                            close()
                        })
                    }
                }

                "fp" -> {
                    BackRow { category = null }
                    stringArrayResource(R.array.task_priority_labels).forEachIndexed { i, label ->
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
                        val nm = m.name.ifBlank { m.email }
                        TMenuItem(
                            nm,
                            onClick = {
                                vm.setFilter(f.copy(assigneeIds = f.assigneeIds + m.userId))
                                close()
                            },
                            leading = { MemberAvatar(22.dp, nm, userId = m.userId) },
                        )
                    }
                    // GitLab-only assignees (no Tessera account) filter by "gl:<username>",
                    // matched against a card's gitlab_assignees (web parity).
                    val glUnlinked = state.gitlabMembers.filter {
                        it.tesseraUserId == null && "gl:${it.glUsername}" !in f.assigneeIds
                    }
                    if (glUnlinked.isNotEmpty()) {
                        MenuSectionHeader("GitLab")
                        glUnlinked.forEach { m ->
                            val nm = m.glName.ifBlank { m.glUsername }
                            TMenuItem(
                                nm,
                                onClick = {
                                    vm.setFilter(f.copy(assigneeIds = f.assigneeIds + "gl:${m.glUsername}"))
                                    close()
                                },
                                leading = { MemberAvatar(22.dp, nm, avatarUrl = m.glAvatarUrl, muted = true) },
                            )
                        }
                    }
                }

                "fc" -> {
                    BackRow { category = null }
                    state.members.filter { it.userId !in f.authorIds }.forEach { m ->
                        val nm = m.name.ifBlank { m.email }
                        TMenuItem(
                            nm,
                            onClick = {
                                vm.setFilter(f.copy(authorIds = f.authorIds + m.userId))
                                close()
                            },
                            leading = { MemberAvatar(22.dp, nm, userId = m.userId) },
                        )
                    }
                    // GitLab logins: the project's GitLab members plus the authors actually
                    // seen on the board (an issue can be opened by someone outside the
                    // roster). A login already covered by a linked Tessera row is skipped
                    // so one person never shows up twice (web parity).
                    val seen = state.glLoginByUserId.values.toMutableSet()
                    val glAuthors = buildList {
                        state.gitlabMembers.forEach { add(GitlabAuthor(it.glUsername, it.glName.ifBlank { it.glUsername }, it.glAvatarUrl)) }
                        addAll(boardGitlabAuthors(state.tasks))
                    }.filter { a ->
                        a.username.isNotBlank() && "gl:${a.username}" !in f.authorIds && seen.add(a.username)
                    }
                    if (glAuthors.isNotEmpty()) {
                        MenuSectionHeader("GitLab")
                        glAuthors.forEach { a ->
                            TMenuItem(
                                a.name,
                                onClick = {
                                    vm.setFilter(f.copy(authorIds = f.authorIds + "gl:${a.username}"))
                                    close()
                                },
                                leading = { MemberAvatar(22.dp, a.name, avatarUrl = a.avatarUrl, muted = true) },
                            )
                        }
                    }
                }

                "ft" -> {
                    BackRow { category = null }
                    // Group the pickable tags by prefix; show section headers only
                    // when more than one group exists (web «Фильтр: тег»). GitLab
                    // meta-labels (status/priority/…) are hidden from the picker.
                    val groups = buildTagGroups(
                        LocalResources.current,
                        state.tagList.filter { it.id !in f.tagIds },
                        state.prefixNames,
                        state.metaTagPrefixes,
                    )
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
                    DueChipLabels.forEach { (due, labelRes) ->
                        TMenuItem(stringResource(labelRes), onClick = {
                            vm.setFilter(f.copy(due = due))
                            close()
                        })
                    }
                }

                "fs" -> {
                    BackRow { category = null }
                    state.sortedColumns.filter { it.id !in f.statuses }.forEach { col ->
                        TMenuItem(columnCaption(res, col), onClick = {
                            vm.setFilter(f.copy(statuses = f.statuses + col.id))
                            close()
                        })
                    }
                }

                "fm" -> {
                    BackRow { category = null }
                    // addMilestoneFilter (not setFilter): picking a milestone here
                    // also drops the server-side sprint scope, so the accent scope
                    // chip is replaced by this grey facet (web parity).
                    state.milestones.filter { it.id !in f.milestoneIds }.forEach { m ->
                        TMenuItem(m.title, onClick = {
                            vm.addMilestoneFilter(m.id)
                            close()
                        })
                    }
                    if ("__none__" !in f.milestoneIds) {
                        TMenuItem(stringResource(R.string.task_milestone_none), onClick = {
                            vm.addMilestoneFilter("__none__")
                            close()
                        })
                    }
                }

                else -> {
                    if (sortFields.isNotEmpty()) {
                        ArrowRow(stringResource(R.string.composer_add_sort)) { category = "sort" }
                    }
                    if (timeline && state.sortedColumns.isNotEmpty()) {
                        ArrowRow(stringResource(R.string.composer_add_filter_status)) { category = "fs" }
                    }
                    ArrowRow(stringResource(R.string.composer_add_filter_priority)) { category = "fp" }
                    if (state.members.isNotEmpty()) {
                        ArrowRow(stringResource(R.string.composer_add_filter_assignee)) { category = "fa" }
                        ArrowRow(stringResource(R.string.composer_add_filter_author)) { category = "fc" }
                    }
                    if (state.tagList.isNotEmpty()) {
                        ArrowRow(stringResource(R.string.composer_add_filter_tag)) { category = "ft" }
                    }
                    if (state.milestones.isNotEmpty()) {
                        ArrowRow(stringResource(R.string.composer_add_filter_milestone)) { category = "fm" }
                    }
                    ArrowRow(stringResource(R.string.composer_add_filter_due)) { category = "fd" }
                }
            }
        }
    }
}

@Composable
private fun CheckRow(label: String, selected: Boolean, tag: String? = null, onClick: () -> Unit) {
    TMenuItem(
        label,
        onClick = onClick,
        trailing = {
            if (selected) IonIcon(Ion.CHECK, size = 16.dp, tint = Tessera.colors.primary, gradient = true)
        },
        modifier = if (tag != null) Modifier.testTag(tag) else Modifier,
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
    TMenuItem(stringResource(R.string.composer_back), onClick = onClick)
    TMenuDivider()
}

/** Chip height — web `.facet { height: 22px }`, matching the dashed «+» button so a
 *  row of chips has one baseline. */
private val FacetChipHeight = 22.dp

/**
 * Height of one collapsed composer row: a chip plus the bar's 8dp top/bottom padding.
 *
 * The collapsed bar used to be produced by `FlowRow(maxLines = 1, overflow = Clip)`
 * while [ComposerSearch] sat in the same row with `Modifier.weight(1f)`. That pairing
 * — a line-limited FlowRow plus a weighted child whose intrinsic width is forced to 0
 * ([zeroIntrinsicWidth]) — is what froze the app mid-collapse: collapsing also plays
 * the toolbar's `expandHorizontally` animation, so the bar is re-measured at a new
 * (shrinking) width on every frame, and once the chips no longer fit the row the
 * line-break/weight pass has no space left to hand the weighted child. Widening the
 * chips with icons is what started reaching that state, which is why it only appeared
 * with the icon chips and only "every other time" — it depends on the exact frame
 * width at which the chips stop fitting.
 *
 * So the collapsed bar now clips by *height* instead: FlowRow is left unlimited (a
 * plain wrap, no overflow state, no intrinsic pass) and the enclosing Box is pinned to
 * one row and clips the rest. The rendered result is the same single row as before.
 */
private val ComposerRowHeight = FacetChipHeight + 16.dp

/**
 * Live state of a sort-chip long-press drag: which chip is lifted, how far it has
 * travelled, and where each chip sits (window coords, like the board's own DnD).
 *
 * The move is committed **once, on release** — unlike the web `<draggable>` there
 * is no live reshuffling, so the recorded rects stay valid for the whole gesture
 * (a live swap would move the chips out from under their own bounds mid-drag). The
 * chip under the lifted one is ringed instead, so the drop is still previewed.
 */
private class SortDragState {
    var from by mutableIntStateOf(-1)
    var offset by mutableStateOf(Offset.Zero)
    val bounds = mutableStateMapOf<Int, Rect>()

    val active: Boolean get() = from >= 0

    /** Index of the chip under the lifted chip's centre, or -1 (itself / outside). */
    val target: Int
        get() {
            val src = bounds[from] ?: return -1
            val point = src.center + offset
            return bounds.entries.firstOrNull { it.key != from && it.value.contains(point) }?.key ?: -1
        }

    fun start(index: Int) {
        from = index
        offset = Offset.Zero
    }

    fun reset() {
        from = -1
        offset = Offset.Zero
    }
}

/**
 * The sort-level chips: tap flips the direction, long-press-and-drag reorders them.
 * Level order IS sort precedence (first = primary), which is why the drop persists
 * through [BoardViewModel.moveSortLevel] rather than living in local UI state.
 */
@Composable
private fun FlowRowScope.SortChips(
    state: BoardUiState,
    vm: BoardViewModel,
    drag: SortDragState,
    enabled: Boolean,
) {
    val haptics = LocalHapticFeedback.current
    val levels = state.sortLevels
    // Bounds are keyed by index — drop the tail when a level is removed, else a
    // stale rect (a chip that no longer exists) can resolve as the drop target.
    LaunchedEffect(levels.size) { drag.bounds.keys.retainAll { it < levels.size } }
    val target = if (drag.active) drag.target else -1
    levels.forEachIndexed { i, level ->
        // Ключ неизвестного поля показываем как есть: он пришёл из сохранённого
        // представления, переводить в ресурсах нечего. i18n-data
        val label = SortField.fromKey(level.field)?.let { stringResource(it.labelRes) } ?: level.field
        val arrow = if (level.dir == "desc") "↓" else "↑"
        val lifted = drag.from == i
        FacetChip(
            "$label $arrow",
            icon = Ion.SORT,
            highlighted = target == i,
            onClick = { vm.toggleSortDir(i) },
            onRemove = { vm.removeSortLevel(i) },
            modifier = Modifier
                // Ahead of the layer below, so the reported rect is the chip's
                // resting place — not where the drag has translated it to.
                .onGloballyPositioned { drag.bounds[i] = it.boundsInWindow() }
                .zIndex(if (lifted) 1f else 0f)
                .graphicsLayer {
                    if (lifted) {
                        translationX = drag.offset.x
                        translationY = drag.offset.y
                        scaleX = 1.05f
                        scaleY = 1.05f
                        alpha = 0.9f
                    }
                }
                .draggableSortChip(drag, i, enabled, haptics) { from, to -> vm.moveSortLevel(from, to) },
        )
    }
}

/**
 * Arms long-press drag-reorder on a sort chip. Armed only while the composer is
 * [enabled] (expanded) — collapsed, the bar's overlay swallows gestures and a long
 * press should just expand it. [onDrop] is read through `rememberUpdatedState`, so
 * a chip whose `pointerInput` survived recomposition still commits against the
 * current levels instead of a stale closure (the `ColumnDrag` trap).
 */
private fun Modifier.draggableSortChip(
    drag: SortDragState,
    index: Int,
    enabled: Boolean,
    haptics: HapticFeedback,
    onDrop: (from: Int, to: Int) -> Unit,
): Modifier = composed {
    val latestOnDrop by rememberUpdatedState(onDrop)
    this.pointerInput(index, enabled) {
        if (!enabled) return@pointerInput
        detectDragGesturesAfterLongPress(
            onDragStart = {
                haptics.performHapticFeedback(HapticFeedbackType.LongPress)
                drag.start(index)
            },
            onDrag = { change, amount ->
                change.consume()
                drag.offset += amount
            },
            onDragEnd = {
                val to = drag.target
                drag.reset()
                if (to >= 0) latestOnDrop(index, to)
            },
            onDragCancel = { drag.reset() },
        )
    }
}

/**
 * A composer chip pill (web `.facet`): a per-kind icon in place of the old text
 * prefix («Сорт: », «Приоритет: »…), a label, optional click (group/sort) and
 * remove (×). Squared-off 6dp corners and a fixed 22dp height, matching the web
 * chip and the adjacent dashed «+» button.
 *
 * [group] / [accent] are the two tinted variants (grouping chip · sprint scope):
 * accent fill, accent icon/text/×. [highlighted] rings the chip while it is the
 * drop target of a sort-chip drag. A blank [label] gives the icon-only square chip
 * the subtask toggle uses (web `.subtasks-chip`).
 */
@Composable
private fun FacetChip(
    label: String,
    icon: String? = null,
    group: Boolean = false,
    accent: Boolean = false,
    amber: Boolean = false,
    highlighted: Boolean = false,
    onClick: (() -> Unit)? = null,
    onRemove: (() -> Unit)? = null,
    modifier: Modifier = Modifier,
) {
    val c = Tessera.colors
    val tinted = group || accent || amber
    // Amber (archive scope) carries its own ink — it is a warning tone, not the
    // workspace accent, so it stays flat: the accent gradient belongs to the accent.
    val ink = if (amber) ConflictAmber else c.primary
    val shape = RoundedCornerShape(RadiusSm)
    val iconOnly = label.isEmpty()
    Row(
        modifier
            .height(FacetChipHeight)
            .clip(shape)
            .background(
                when {
                    amber -> ConflictAmber.copy(alpha = 0.15f)
                    tinted -> c.primary.copy(alpha = 0.14f)
                    else -> c.hover
                },
            )
            .then(if (highlighted) Modifier.border(1.dp, c.primary, shape) else Modifier)
            .then(if (onClick != null) Modifier.clickableNoRipple(onClick = onClick) else Modifier)
            .padding(
                start = if (iconOnly) 6.dp else 9.dp,
                end = if (iconOnly) 6.dp else if (onRemove != null) 4.dp else 9.dp,
            ),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        if (icon != null) {
            IonIcon(
                icon,
                size = if (iconOnly) 14.dp else 13.dp,
                tint = if (tinted) ink else c.text3,
                gradient = tinted && !amber,
            )
            if (!iconOnly) Spacer(Modifier.width(4.dp))
        }
        if (!iconOnly) {
            Text(label, color = if (tinted) ink else c.text2, fontSize = 12.sp, maxLines = 1)
        }
        if (onRemove != null) {
            Spacer(Modifier.width(2.dp))
            Box(
                Modifier.clip(CircleShape).clickableNoRipple(onClick = onRemove).padding(horizontal = 3.dp),
                contentAlignment = Alignment.Center,
            ) { Text("×", color = if (tinted) ink else c.text3, fontSize = 14.sp) }
        }
    }
}

/**
 * The collapsed bar's stand-in for [ComposerSearch]: the active query, or the same
 * «Поиск…» hint. Static text, no weight — the collapsed bar must stay free of the
 * weighted text field (see [ComposerRowHeight]). Chip-height so the collapsed row
 * keeps its exact previous height.
 */
@Composable
private fun CollapsedSearchLabel(query: String) {
    val c = Tessera.colors
    Text(
        query.ifEmpty { stringResource(R.string.composer_search_hint) },
        color = if (query.isEmpty()) c.text3 else c.text1,
        fontSize = 13.sp,
        maxLines = 1,
        overflow = TextOverflow.Ellipsis,
        modifier = Modifier.height(FacetChipHeight).wrapContentHeight(Alignment.CenterVertically)
            .padding(horizontal = 4.dp),
    )
}

/** Borderless inline search inside the bar (web `.composer-search`). */
@Composable
private fun FlowRowScope.ComposerSearch(value: String, onValue: (String) -> Unit, modifier: Modifier) {
    val c = Tessera.colors
    // decorationBox — обычная лямбда, а не композиция: подсказку берём заранее.
    val hint = stringResource(R.string.composer_search_hint)
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
                Text(hint, color = c.text3, fontSize = 13.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
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
        Text(
            stringResource(R.string.views_title),
            color = c.text3, fontSize = 12.sp, fontWeight = FontWeight.Medium,
        )
        Spacer(Modifier.height(8.dp))
        if (state.savedViews.isEmpty()) {
            Text(stringResource(R.string.views_empty), color = c.text3, fontSize = 13.sp)
        } else {
            state.savedViews.forEach { view ->
                SavedViewRow(view, state.currentViewName == view.name, vm, onClose)
            }
        }
        Spacer(Modifier.height(12.dp))
        HorizontalDivider(color = c.border)
        Spacer(Modifier.height(12.dp))
        Text(
            stringResource(R.string.views_save_current),
            color = c.text3, fontSize = 12.sp, fontWeight = FontWeight.Medium,
        )
        Spacer(Modifier.height(6.dp))
        Row(verticalAlignment = Alignment.CenterVertically) {
            Box(Modifier.weight(1f)) {
                TTextField(
                    value = name,
                    onValueChange = { name = it },
                    placeholder = stringResource(R.string.views_name_hint),
                )
            }
            Spacer(Modifier.width(8.dp))
            TButton(
                stringResource(R.string.common_save),
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
                message = stringResource(R.string.views_delete_confirm, view.name),
                onConfirm = {
                    confirmDelete = false
                    vm.deleteView(view)
                },
                onDismiss = { confirmDelete = false },
            )
        }
    }
}
