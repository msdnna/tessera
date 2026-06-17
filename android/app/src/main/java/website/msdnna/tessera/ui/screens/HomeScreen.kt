package website.msdnna.tessera.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
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
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import website.msdnna.tessera.data.model.WorkspaceTask
import website.msdnna.tessera.ui.components.ColorDot
import website.msdnna.tessera.ui.components.ErrorState
import website.msdnna.tessera.ui.components.LoadingState
import website.msdnna.tessera.ui.components.TagChip
import website.msdnna.tessera.ui.components.clickableNoRipple
import website.msdnna.tessera.ui.theme.PriorityColors
import website.msdnna.tessera.ui.theme.RadiusMd
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.accentGradient
import website.msdnna.tessera.ui.viewmodels.HomeFilter
import website.msdnna.tessera.ui.viewmodels.HomeUiState
import website.msdnna.tessera.ui.viewmodels.HomeViewModel
import website.msdnna.tessera.util.isoDateKey
import website.msdnna.tessera.util.shortDate

private data class StatCard(val filter: HomeFilter, val label: String, val color: Color, val count: (HomeUiState) -> Int)

private val StatCards = listOf(
    StatCard(HomeFilter.Me, "Мои задачи", Color(0xFF7C5CFF)) { it.summary.assigned },
    StatCard(HomeFilter.All, "Все активные", Color(0xFF6B7280)) { it.summary.active },
    StatCard(HomeFilter.Overdue, "Просрочено", Color(0xFFE0533D)) { it.summary.overdue },
    StatCard(HomeFilter.Today, "Сегодня", Color(0xFFE0A418)) { it.summary.dueToday },
    StatCard(HomeFilter.Week, "На неделе", Color(0xFF2F80ED)) { it.summary.dueWeek },
    StatCard(HomeFilter.Completed, "Выполнено", Color(0xFF18A058)) { it.summary.completed },
)

/** Home / "Моя работа": a greeting, summary stat cards and a filtered task list. */
@Composable
fun HomeScreen(
    workspaceId: String,
    userName: String,
    userId: String,
    onOpenTask: (boardId: String, taskId: String) -> Unit,
) {
    val c = Tessera.colors
    val vm: HomeViewModel = viewModel()
    val state by vm.state.collectAsStateWithLifecycle()

    LaunchedEffect(workspaceId, userId) {
        if (workspaceId.isNotBlank()) vm.load(workspaceId, userId)
    }

    Column(Modifier.fillMaxSize().background(c.bg)) {
        Text(
            "Привет, ${userName.ifBlank { "коллега" }} 👋",
            color = c.text1,
            fontSize = 20.sp,
            fontWeight = FontWeight.Bold,
            modifier = Modifier.padding(start = 16.dp, end = 16.dp, top = 16.dp, bottom = 12.dp),
        )

        // ── stat cards ──
        Row(
            Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()).padding(horizontal = 12.dp),
            horizontalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            StatCards.forEach { card ->
                StatCardView(card, count = card.count(state), selected = state.filter == card.filter) {
                    vm.setFilter(card.filter)
                }
            }
        }
        Spacer(Modifier.height(12.dp))

        when {
            state.loading -> LoadingState()

            state.error != null -> ErrorState(
                message = state.error ?: "Ошибка",
                onRetry = { vm.load(workspaceId, userId) },
            )

            else -> {
                val tasks = state.visibleTasks
                if (tasks.isEmpty()) {
                    Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                        Text("Задач нет", color = c.text3, fontSize = 14.sp)
                    }
                } else {
                    LazyColumn(
                        Modifier.fillMaxSize().padding(horizontal = 12.dp),
                        verticalArrangement = Arrangement.spacedBy(8.dp),
                    ) {
                        items(tasks, key = { it.id }) { task ->
                            TaskRow(task, state, onClick = { onOpenTask(task.boardId, task.id) })
                        }
                        item { Spacer(Modifier.height(16.dp)) }
                    }
                }
            }
        }
    }
}

@Composable
private fun StatCardView(card: StatCard, count: Int, selected: Boolean, onClick: () -> Unit) {
    val c = Tessera.colors
    Column(
        Modifier
            .clip(RoundedCornerShape(RadiusMd))
            .background(if (selected) accentGradient(card.color.copy(alpha = 0.16f)) else androidx.compose.ui.graphics.SolidColor(c.cardSurface))
            .border(1.dp, if (selected) card.color else c.border, RoundedCornerShape(RadiusMd))
            .clickableNoRipple(onClick = onClick)
            .padding(horizontal = 14.dp, vertical = 10.dp),
    ) {
        Text(count.toString(), color = card.color, fontSize = 22.sp, fontWeight = FontWeight.Bold)
        Text(card.label, color = c.text2, fontSize = 12.sp, fontWeight = FontWeight.Medium)
    }
}

@Composable
private fun TaskRow(task: WorkspaceTask, state: HomeUiState, onClick: () -> Unit) {
    val c = Tessera.colors
    val priorityColor = PriorityColors.getOrElse(task.priority) { PriorityColors[0] }
    val today = isoDateKey(nowIsoDate())
    val due = isoDateKey(task.dueDate)
    val overdue = due.isNotEmpty() && due < today && !task.isCompleted

    Row(
        Modifier.fillMaxWidth().clip(RoundedCornerShape(RadiusMd)).background(c.cardSurface)
            .border(1.dp, c.border, RoundedCornerShape(RadiusMd)).clickableNoRipple(onClick = onClick)
            .padding(12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Box(Modifier.size(8.dp).clip(CircleShape).background(accentGradient(priorityColor)))
        Spacer(Modifier.width(10.dp))
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
                    fontWeight = FontWeight.Medium,
                    maxLines = 1,
                    modifier = Modifier.weight(1f, fill = false),
                )
            }
            Spacer(Modifier.height(3.dp))
            Text(
                "${task.projectName} / ${task.boardName}",
                color = c.text3,
                fontSize = 11.sp,
                maxLines = 1,
            )
            val tagChips = task.tagIds.mapNotNull { state.tags[it] }.take(3)
            if (tagChips.isNotEmpty() || task.dueDate != null) {
                Spacer(Modifier.height(6.dp))
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(5.dp)) {
                    tagChips.forEach { TagChip(it.name, it.color) }
                    if (task.dueDate != null) {
                        ColorDot(if (overdue) Color(0xFFE0533D) else c.text3, sizeDp = 6)
                        Text(
                            shortDate(task.dueDate),
                            color = if (overdue) Color(0xFFE0533D) else c.text3,
                            fontSize = 11.sp,
                        )
                    }
                }
            }
        }
    }
}

/** Today's date as a UTC-midnight ISO key, to compare with due-date keys. */
private fun nowIsoDate(): String {
    val cal = java.util.Calendar.getInstance()
    val y = cal.get(java.util.Calendar.YEAR)
    val m = (cal.get(java.util.Calendar.MONTH) + 1).toString().padStart(2, '0')
    val d = cal.get(java.util.Calendar.DAY_OF_MONTH).toString().padStart(2, '0')
    return "$y-$m-${d}T00:00:00Z"
}
