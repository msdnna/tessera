package website.msdnna.tessera.ui.viewmodels

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import java.util.Calendar
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import website.msdnna.tessera.data.model.Tag
import website.msdnna.tessera.data.model.WorkspaceSummary
import website.msdnna.tessera.data.model.WorkspaceTask
import website.msdnna.tessera.data.repository.HomeRepository
import website.msdnna.tessera.util.errorMessage
import website.msdnna.tessera.util.isoDateKey

enum class HomeFilter { Me, All, Overdue, Today, Week, Completed }

data class HomeUiState(
    val loading: Boolean = true,
    val error: String? = null,
    val summary: WorkspaceSummary = WorkspaceSummary(),
    val tasks: List<WorkspaceTask> = emptyList(),
    val tags: Map<String, Tag> = emptyMap(),
    val filter: HomeFilter = HomeFilter.Me,
    val meId: String = "",
) {
    /** The task list for the active filter — mirrors HomeView's per-card filter. */
    val visibleTasks: List<WorkspaceTask>
        get() {
            val today = dateKeyOffset(0)
            val weekEnd = dateKeyOffset(DAYS_IN_WEEK)
            return tasks.filter { t ->
                val due = isoDateKey(t.dueDate)
                when (filter) {
                    HomeFilter.Me -> !t.isCompleted && meId in t.assigneeIds
                    HomeFilter.All -> !t.isCompleted
                    HomeFilter.Overdue -> !t.isCompleted && due.isNotEmpty() && due < today
                    HomeFilter.Today -> !t.isCompleted && due == today
                    HomeFilter.Week -> !t.isCompleted && due.isNotEmpty() && due >= today && due < weekEnd
                    HomeFilter.Completed -> t.isCompleted
                }
            }
        }
}

/** Home / "Моя работа" dashboard: summary counts + a filterable task list. */
class HomeViewModel(
    private val repo: HomeRepository = HomeRepository(),
) : ViewModel() {
    private val _state = MutableStateFlow(HomeUiState())
    val state: StateFlow<HomeUiState> = _state.asStateFlow()

    private var workspaceId: String = ""

    fun load(workspaceId: String, meId: String) {
        this.workspaceId = workspaceId
        _state.update { it.copy(loading = true, error = null, meId = meId) }
        launchCatching {
            val summary = repo.summary(workspaceId)
            val tasks = repo.tasks(workspaceId)
            val tags = repo.tags(workspaceId)
            _state.update {
                it.copy(
                    loading = false,
                    summary = summary,
                    tasks = tasks,
                    tags = tags.associateBy { t -> t.id },
                )
            }
        }
    }

    fun reload() {
        if (workspaceId.isNotBlank()) load(workspaceId, _state.value.meId)
    }

    fun setFilter(filter: HomeFilter) = _state.update { it.copy(filter = filter) }

    private fun launchCatching(block: suspend () -> Unit) {
        viewModelScope.launch {
            val result = runCatching { block() }
            result.exceptionOrNull()?.let { e ->
                _state.update { it.copy(loading = false, error = errorMessage(e)) }
            }
        }
    }
}

private const val DAYS_IN_WEEK = 7

/** A local `yyyy-MM-dd` key for today + [days], matching due-date keys. */
private fun dateKeyOffset(days: Int): String {
    val cal = Calendar.getInstance().apply { add(Calendar.DAY_OF_MONTH, days) }
    val y = cal.get(Calendar.YEAR)
    val m = (cal.get(Calendar.MONTH) + 1).toString().padStart(2, '0')
    val d = cal.get(Calendar.DAY_OF_MONTH).toString().padStart(2, '0')
    return "$y-$m-$d"
}
