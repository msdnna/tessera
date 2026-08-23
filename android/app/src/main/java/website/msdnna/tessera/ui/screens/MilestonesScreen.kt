package website.msdnna.tessera.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
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
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.res.stringArrayResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import java.util.Calendar
import java.util.TimeZone
import website.msdnna.tessera.R
import website.msdnna.tessera.data.model.Milestone
import website.msdnna.tessera.data.model.Project
import website.msdnna.tessera.data.model.Workspace
import website.msdnna.tessera.data.model.WorkspaceMilestone
import website.msdnna.tessera.ui.components.ErrorState
import website.msdnna.tessera.ui.components.IonIcon
import website.msdnna.tessera.ui.components.IonIconButton
import website.msdnna.tessera.ui.components.LoadingState
import website.msdnna.tessera.ui.components.TButton
import website.msdnna.tessera.ui.components.TButtonKind
import website.msdnna.tessera.ui.components.TConfirmPopover
import website.msdnna.tessera.ui.components.TDropdown
import website.msdnna.tessera.ui.components.TTextField
import website.msdnna.tessera.ui.components.clickableNoRipple
import website.msdnna.tessera.ui.theme.RadiusLg
import website.msdnna.tessera.ui.theme.RadiusMd
import website.msdnna.tessera.ui.theme.RadiusSm
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.accentGradient
import website.msdnna.tessera.ui.viewmodels.MilestoneViewModel
import website.msdnna.tessera.util.Estimation
import website.msdnna.tessera.util.Ion
import website.msdnna.tessera.util.Milestones
import website.msdnna.tessera.util.longDate
import website.msdnna.tessera.util.millisToUtcIso

/**
 * Workspace «Этапы» — a cross-project milestone roadmap (web `MilestonesView`
 * parity). Rows group by project; each shows a date range, an accent-gradient
 * progress bar (done/total), Σ estimate and a GitLab badge. Tapping a row deep-links
 * to that project's board filtered by the milestone. The per-project gear opens the
 * milestone manager (CRUD + opt-in GitLab push).
 */
@Composable
fun MilestonesScreen(
    workspaceId: String,
    projects: List<Project>,
    workspace: Workspace?,
    glProjectId: String?,
    onOpenMilestone: (projectId: String, milestoneId: String) -> Unit,
    vm: MilestoneViewModel = viewModel(key = "milestones"),
) {
    val c = Tessera.colors
    val state by vm.state.collectAsStateWithLifecycle()
    val manager by vm.manager.collectAsStateWithLifecycle()

    androidx.compose.runtime.LaunchedEffect(workspaceId) { vm.load(workspaceId) }

    fun estimationFor(projectId: String) =
        Estimation.resolve(projects.firstOrNull { it.id == projectId }, workspace)

    Column(Modifier.fillMaxSize().background(c.bg)) {
        // Активные / Все toggle row.
        Row(
            Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 10.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            SegToggle(
                left = stringResource(R.string.milestones_filter_active),
                right = stringResource(R.string.milestones_filter_all),
                rightSelected = state.showClosed,
                onToggle = { vm.toggleShowClosed() },
            )
        }

        when {
            state.loading -> LoadingState()

            state.error != null -> ErrorState(
                message = state.error ?: stringResource(R.string.common_error),
                onRetry = { vm.load(workspaceId) },
            )

            state.visible.isEmpty() -> Box(Modifier.fillMaxSize().padding(32.dp), contentAlignment = Alignment.Center) {
                Text(
                    stringResource(
                        if (state.showClosed) R.string.milestones_empty_all else R.string.milestones_empty_active,
                    ),
                    color = c.text3,
                    fontSize = 14.sp,
                )
            }

            else -> {
                val grouped = state.visible.groupBy { it.projectId }
                val order = state.visible.map { it.projectId }.distinct()
                LazyColumn(
                    Modifier.fillMaxSize(),
                    contentPadding = androidx.compose.foundation.layout.PaddingValues(bottom = 24.dp),
                ) {
                    order.forEach { pid ->
                        val rows = grouped[pid].orEmpty()
                        val pname = rows.firstOrNull()?.projectName ?: ""
                        item(key = "h-$pid") {
                            ProjectHeader(
                                name = pname,
                                count = rows.size,
                                onManage = {
                                    vm.openManager(pid, pname, glCapable = pid == glProjectId)
                                },
                            )
                        }
                        items(rows, key = { it.id }) { m ->
                            MilestoneRow(
                                m = m,
                                estimateText = Estimation.format(m.estimateSum, estimationFor(m.projectId)),
                                onClick = { onOpenMilestone(m.projectId, m.id) },
                            )
                        }
                    }
                }
            }
        }
    }

    manager?.let { mgr ->
        MilestoneManagerModal(vm = vm, mgr = mgr, onDismiss = { vm.closeManager() })
    }
}

@Composable
private fun ProjectHeader(name: String, count: Int, onManage: () -> Unit) {
    val c = Tessera.colors
    Row(
        Modifier.fillMaxWidth().padding(start = 16.dp, end = 12.dp, top = 14.dp, bottom = 6.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(name, color = c.text2, fontSize = 13.sp, fontWeight = FontWeight.SemiBold)
        Spacer(Modifier.width(8.dp))
        Text("$count", color = c.text3, fontSize = 12.sp)
        Spacer(Modifier.weight(1f))
        IonIconButton(Ion.SETTINGS, onClick = onManage, boxSize = 30.dp, iconSize = 16.dp, tint = c.text3)
    }
}

@Composable
private fun MilestoneRow(m: WorkspaceMilestone, estimateText: String, onClick: () -> Unit) {
    val c = Tessera.colors
    val range = Milestones.range(m.startDate, m.dueDate)
    val pct = if (m.taskCount > 0) (m.doneCount.toFloat() / m.taskCount.toFloat()).coerceIn(0f, 1f) else 0f
    Column(
        Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 4.dp)
            .clip(RoundedCornerShape(RadiusMd))
            .background(c.cardSurface)
            .border(1.dp, c.border, RoundedCornerShape(RadiusMd))
            .clickableNoRipple(onClick = onClick)
            .padding(horizontal = 14.dp, vertical = 12.dp)
            .alpha(if (m.isClosed) 0.62f else 1f),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            IonIcon(Ion.ROCKET, size = 15.dp, tint = c.text2)
            Spacer(Modifier.width(8.dp))
            Text(
                m.title,
                color = c.text1,
                fontSize = 15.sp,
                fontWeight = FontWeight.Medium,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.weight(1f, fill = false),
            )
            if (m.isLinked) {
                Spacer(Modifier.width(8.dp))
                GitlabBadge()
            }
            Spacer(Modifier.weight(1f))
            if (m.isClosed) {
                Text(stringResource(R.string.milestones_closed), color = c.text3, fontSize = 11.sp)
            }
        }
        if (range.isNotEmpty()) {
            Spacer(Modifier.height(4.dp))
            Text(range, color = c.text3, fontSize = 12.sp)
        }
        Spacer(Modifier.height(8.dp))
        // Accent-gradient progress bar.
        Box(
            Modifier.fillMaxWidth().height(6.dp).clip(RoundedCornerShape(3.dp)).background(c.surfaceAlt),
        ) {
            if (pct > 0f) {
                Box(
                    Modifier.fillMaxHeight().fillMaxWidth(pct).clip(RoundedCornerShape(3.dp))
                        .background(accentGradient(c.primary)),
                )
            }
        }
        Spacer(Modifier.height(6.dp))
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text(
                if (m.taskCount > 0) {
                    "✓ ${m.doneCount}/${m.taskCount}"
                } else {
                    stringResource(R.string.milestones_no_tasks)
                },
                color = c.text3,
                fontSize = 12.sp,
            )
            if (estimateText.isNotBlank()) {
                Spacer(Modifier.width(10.dp))
                Text("Σ $estimateText", color = c.text3, fontSize = 12.sp)
            }
        }
    }
}

@Composable
private fun GitlabBadge() {
    val c = Tessera.colors
    Row(
        Modifier.clip(RoundedCornerShape(RadiusSm)).background(c.surfaceAlt)
            .padding(horizontal = 6.dp, vertical = 2.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        IonIcon(Ion.GITLAB, size = 12.dp, tint = c.text3)
        Spacer(Modifier.width(3.dp))
        Text("GitLab", color = c.text3, fontSize = 10.sp)
    }
}

/** A two-segment pill toggle (Активные | Все). */
@Composable
private fun SegToggle(left: String, right: String, rightSelected: Boolean, onToggle: () -> Unit) {
    val c = Tessera.colors
    Row(
        Modifier.clip(RoundedCornerShape(RadiusMd)).background(c.surfaceAlt).padding(2.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        SegItem(left, selected = !rightSelected) { if (rightSelected) onToggle() }
        SegItem(right, selected = rightSelected) { if (!rightSelected) onToggle() }
    }
}

@Composable
private fun SegItem(label: String, selected: Boolean, onClick: () -> Unit) {
    val c = Tessera.colors
    Box(
        Modifier.clip(RoundedCornerShape(RadiusSm))
            .then(if (selected) Modifier.background(accentGradient(c.primary)) else Modifier)
            .clickableNoRipple(onClick = onClick)
            .padding(horizontal = 14.dp, vertical = 6.dp),
    ) {
        Text(
            label,
            color = if (selected) c.onPrimary else c.text2,
            fontSize = 13.sp,
            fontWeight = FontWeight.Medium,
        )
    }
}

// ── manager modal ─────────────────────────────────────────────────────────────

@Composable
private fun MilestoneManagerModal(
    vm: MilestoneViewModel,
    mgr: website.msdnna.tessera.ui.viewmodels.MilestoneManagerState,
    onDismiss: () -> Unit,
) {
    val c = Tessera.colors
    Dialog(onDismissRequest = onDismiss, properties = DialogProperties(usePlatformDefaultWidth = false)) {
        Column(
            Modifier.fillMaxWidth(0.96f).fillMaxHeight(0.9f).clip(RoundedCornerShape(RadiusLg)).background(c.surface),
        ) {
            Row(
                Modifier.fillMaxWidth().padding(start = 18.dp, end = 10.dp, top = 16.dp, bottom = 8.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Column(Modifier.weight(1f)) {
                    Text(
                        stringResource(R.string.nav_milestones),
                        color = c.text1,
                        fontSize = 18.sp,
                        fontWeight = FontWeight.SemiBold,
                    )
                    if (mgr.projectName.isNotBlank()) {
                        Text(mgr.projectName, color = c.text3, fontSize = 12.sp)
                    }
                }
                IonIconButton(Ion.CLOSE, onClick = onDismiss, boxSize = 32.dp, iconSize = 18.dp, tint = c.text3)
            }
            mgr.error?.let {
                Text(
                    it,
                    color = website.msdnna.tessera.ui.theme.TesseraDanger,
                    fontSize = 12.sp,
                    modifier = Modifier.padding(horizontal = 18.dp, vertical = 4.dp),
                )
            }
            Column(Modifier.weight(1f).fillMaxWidth().verticalScroll(rememberScrollState()).padding(horizontal = 14.dp)) {
                if (mgr.loading) {
                    Box(Modifier.fillMaxWidth().padding(40.dp), contentAlignment = Alignment.Center) { LoadingState() }
                } else {
                    if (mgr.milestones.isEmpty()) {
                        Text(
                            stringResource(R.string.milestones_manager_empty),
                            color = c.text3,
                            fontSize = 13.sp,
                            modifier = Modifier.padding(vertical = 10.dp),
                        )
                    }
                    mgr.milestones.forEach { m ->
                        ManagerRow(vm = vm, m = m, glCapable = mgr.glCapable)
                    }
                    Spacer(Modifier.height(12.dp))
                    CreateMilestoneRow(onCreate = { t, s, d -> vm.createMilestone(t, s, d) })
                    Spacer(Modifier.height(16.dp))
                }
            }
        }
    }
}

@Composable
private fun ManagerRow(vm: MilestoneViewModel, m: Milestone, glCapable: Boolean) {
    val c = Tessera.colors
    var editing by remember(m.id) { mutableStateOf(false) }
    var confirmDelete by remember(m.id) { mutableStateOf(false) }

    Column(
        Modifier.fillMaxWidth().padding(vertical = 4.dp).clip(RoundedCornerShape(RadiusMd))
            .border(1.dp, c.border, RoundedCornerShape(RadiusMd)).padding(12.dp),
    ) {
        if (editing) {
            MilestoneEditor(
                initialTitle = m.title,
                initialStart = m.startDate,
                initialDue = m.dueDate,
                onSave = { t, s, d ->
                    vm.updateMilestone(m.id, t, m.description, s, d, m.state)
                    editing = false
                },
                onCancel = { editing = false },
            )
        } else {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Column(Modifier.weight(1f).alpha(if (m.isClosed) 0.6f else 1f)) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Text(
                            m.title,
                            color = c.text1,
                            fontSize = 14.sp,
                            fontWeight = FontWeight.Medium,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis,
                            modifier = Modifier.weight(1f, fill = false),
                        )
                        if (m.isLinked) {
                            Spacer(Modifier.width(6.dp))
                            GitlabBadge()
                        }
                    }
                    val range = Milestones.range(m.startDate, m.dueDate)
                    if (range.isNotEmpty()) {
                        Text(range, color = c.text3, fontSize = 12.sp)
                    }
                }
                // GitLab-sourced milestones are read-only locally (managed by the sync).
                if (!m.isLinked) {
                    IonIconButton(
                        if (m.isClosed) Ion.REFRESH else Ion.CHECK_CIRCLE,
                        onClick = { vm.toggleState(m) },
                        boxSize = 30.dp,
                        iconSize = 16.dp,
                        tint = c.text3,
                    )
                    IonIconButton(Ion.PENCIL, onClick = { editing = true }, boxSize = 30.dp, iconSize = 15.dp, tint = c.text3)
                    if (glCapable) {
                        IonIconButton(Ion.GITLAB, onClick = { vm.pushToGitlab(m.id) }, boxSize = 30.dp, iconSize = 16.dp, tint = c.text3)
                    }
                    Box {
                        IonIconButton(Ion.TRASH, onClick = { confirmDelete = true }, boxSize = 30.dp, iconSize = 15.dp, tint = c.text3)
                        TConfirmPopover(
                            expanded = confirmDelete,
                            message = stringResource(R.string.milestones_delete_confirm),
                            onConfirm = {
                                confirmDelete = false
                                vm.deleteMilestone(m.id)
                            },
                            onDismiss = { confirmDelete = false },
                        )
                    }
                }
            }
        }
    }
}

@Composable
private fun CreateMilestoneRow(onCreate: (title: String, start: String?, due: String?) -> Unit) {
    val c = Tessera.colors
    var title by remember { mutableStateOf("") }
    var start by remember { mutableStateOf<String?>(null) }
    var due by remember { mutableStateOf<String?>(null) }
    Column(
        Modifier.fillMaxWidth().clip(RoundedCornerShape(RadiusMd)).background(c.surfaceAlt).padding(12.dp),
    ) {
        Text(
            stringResource(R.string.milestones_new),
            color = c.text2,
            fontSize = 13.sp,
            fontWeight = FontWeight.Medium,
        )
        Spacer(Modifier.height(8.dp))
        TTextField(
            value = title,
            onValueChange = { title = it },
            placeholder = stringResource(R.string.milestones_title_hint),
            modifier = Modifier.fillMaxWidth(),
        )
        Spacer(Modifier.height(8.dp))
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            DateChip(stringResource(R.string.milestones_date_start), start, onPick = { start = it }, modifier = Modifier.weight(1f))
            DateChip(stringResource(R.string.milestones_date_due), due, onPick = { due = it }, modifier = Modifier.weight(1f))
        }
        Spacer(Modifier.height(10.dp))
        TButton(
            stringResource(R.string.common_create),
            onClick = {
                if (title.isNotBlank()) {
                    onCreate(title.trim(), start, due)
                    title = ""
                    start = null
                    due = null
                }
            },
            enabled = title.isNotBlank(),
        )
    }
}

@Composable
private fun MilestoneEditor(
    initialTitle: String,
    initialStart: String?,
    initialDue: String?,
    onSave: (title: String, start: String?, due: String?) -> Unit,
    onCancel: () -> Unit,
) {
    var title by remember { mutableStateOf(initialTitle) }
    var start by remember { mutableStateOf(initialStart) }
    var due by remember { mutableStateOf(initialDue) }
    Column(Modifier.fillMaxWidth()) {
        TTextField(
            value = title,
            onValueChange = { title = it },
            placeholder = stringResource(R.string.milestones_title_hint),
            modifier = Modifier.fillMaxWidth(),
        )
        Spacer(Modifier.height(8.dp))
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            DateChip(stringResource(R.string.milestones_date_start), start, onPick = { start = it }, modifier = Modifier.weight(1f))
            DateChip(stringResource(R.string.milestones_date_due), due, onPick = { due = it }, modifier = Modifier.weight(1f))
        }
        Spacer(Modifier.height(10.dp))
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            TButton(
                stringResource(R.string.common_save),
                onClick = { if (title.isNotBlank()) onSave(title.trim(), start, due) },
                modifier = Modifier.height(38.dp),
                enabled = title.isNotBlank(),
            )
            TButton(
                stringResource(R.string.common_cancel),
                kind = TButtonKind.Secondary,
                onClick = onCancel,
                modifier = Modifier.height(38.dp),
            )
        }
    }
}

/** A date field that opens a compact calendar popover; shows the picked date or a placeholder. */
@Composable
private fun DateChip(label: String, iso: String?, onPick: (String?) -> Unit, modifier: Modifier = Modifier) {
    val c = Tessera.colors
    var open by remember { mutableStateOf(false) }
    val text = if (iso != null) longDate(iso) else label
    Box(modifier) {
        Row(
            Modifier.fillMaxWidth().clip(RoundedCornerShape(RadiusSm)).border(1.dp, c.border, RoundedCornerShape(RadiusSm))
                .clickableNoRipple { open = true }.padding(horizontal = 10.dp, vertical = 8.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            IonIcon(Ion.CALENDAR, size = 14.dp, tint = c.text3)
            Spacer(Modifier.width(6.dp))
            Text(text, color = if (iso != null) c.text1 else c.text3, fontSize = 13.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
        }
        TDropdown(expanded = open, onDismiss = { open = false }) {
            DatePopover(
                initialIso = iso,
                onPick = {
                    onPick(it)
                    open = false
                },
            )
        }
    }
}

/** A compact UTC month-grid date picker (date-only → UTC-midnight ISO). */
@Composable
private fun DatePopover(initialIso: String?, onPick: (String?) -> Unit) {
    val c = Tessera.colors
    val initialCal = remember {
        Calendar.getInstance(TimeZone.getTimeZone("UTC")).apply {
            val key = if (initialIso != null && initialIso.length >= 10) initialIso.substring(0, 10) else null
            if (key != null) {
                set(Calendar.YEAR, key.substring(0, 4).toInt())
                set(Calendar.MONTH, key.substring(5, 7).toInt() - 1)
                set(Calendar.DAY_OF_MONTH, key.substring(8, 10).toInt())
            }
            set(Calendar.HOUR_OF_DAY, 0)
            set(Calendar.MINUTE, 0)
            set(Calendar.SECOND, 0)
            set(Calendar.MILLISECOND, 0)
        }
    }
    var year by remember { mutableStateOf(initialCal.get(Calendar.YEAR)) }
    var month by remember { mutableStateOf(initialCal.get(Calendar.MONTH)) } // 0-based
    val selectedKey = if (initialIso != null && initialIso.length >= 10) initialIso.substring(0, 10) else null
    // Те же массивы, что у календарного вида доски: подписи читаются на рекомпозицию,
    // поэтому смена языка перерисовывает попап, а не оставляет его на прежнем.
    val monthNames = stringArrayResource(R.array.calendar_months)
    val weekdayHeaders = stringArrayResource(R.array.calendar_weekdays_short)

    Column(Modifier.width(260.dp).padding(10.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            IonIconButton(Ion.CHEVRON_FORWARD, onClick = {
                month -= 1
                if (month < 0) {
                    month = 11
                    year -= 1
                }
            }, boxSize = 28.dp, iconSize = 14.dp, tint = c.text3, modifier = Modifier.graphicsLayer { rotationZ = 180f })
            Text(
                "${monthNames[month]} $year",
                color = c.text1,
                fontSize = 14.sp,
                fontWeight = FontWeight.Medium,
                modifier = Modifier.weight(1f),
                textAlign = androidx.compose.ui.text.style.TextAlign.Center,
            )
            IonIconButton(Ion.CHEVRON_FORWARD, onClick = {
                month += 1
                if (month > 11) {
                    month = 0
                    year += 1
                }
            }, boxSize = 28.dp, iconSize = 14.dp, tint = c.text3)
        }
        Spacer(Modifier.height(6.dp))
        Row(Modifier.fillMaxWidth()) {
            weekdayHeaders.forEach { wd ->
                Text(wd, color = c.text3, fontSize = 10.sp, modifier = Modifier.weight(1f), textAlign = androidx.compose.ui.text.style.TextAlign.Center)
            }
        }
        Spacer(Modifier.height(2.dp))
        // First weekday of the month (Mon=0..Sun=6).
        val first = Calendar.getInstance(TimeZone.getTimeZone("UTC")).apply {
            set(year, month, 1, 0, 0, 0)
            set(Calendar.MILLISECOND, 0)
        }
        val lead = (first.get(Calendar.DAY_OF_WEEK) + 5) % 7 // Mon-based offset
        val days = daysInMonthUtc(year, month)
        val cells = lead + days
        val rows = (cells + 6) / 7
        for (r in 0 until rows) {
            Row(Modifier.fillMaxWidth()) {
                for (col in 0 until 7) {
                    val idx = r * 7 + col
                    val day = idx - lead + 1
                    if (day in 1..days) {
                        val key = "%04d-%02d-%02d".format(year, month + 1, day)
                        val isSel = key == selectedKey
                        Box(
                            Modifier.weight(1f).padding(2.dp).clip(RoundedCornerShape(RadiusSm))
                                .then(if (isSel) Modifier.background(accentGradient(c.primary)) else Modifier)
                                .clickableNoRipple {
                                    val cal = Calendar.getInstance(TimeZone.getTimeZone("UTC")).apply {
                                        set(year, month, day, 0, 0, 0)
                                        set(Calendar.MILLISECOND, 0)
                                    }
                                    onPick(millisToUtcIso(cal.timeInMillis))
                                }
                                .padding(vertical = 8.dp),
                            contentAlignment = Alignment.Center,
                        ) {
                            Text("$day", color = if (isSel) c.onPrimary else c.text1, fontSize = 13.sp)
                        }
                    } else {
                        Box(Modifier.weight(1f))
                    }
                }
            }
        }
        Spacer(Modifier.height(6.dp))
        Text(
            stringResource(R.string.milestones_date_clear),
            color = c.text3,
            fontSize = 13.sp,
            modifier = Modifier.clickableNoRipple { onPick(null) }.padding(vertical = 4.dp),
        )
    }
}

private fun daysInMonthUtc(year: Int, month: Int): Int =
    Calendar.getInstance(TimeZone.getTimeZone("UTC")).apply {
        set(year, month, 1)
    }.getActualMaximum(Calendar.DAY_OF_MONTH)
