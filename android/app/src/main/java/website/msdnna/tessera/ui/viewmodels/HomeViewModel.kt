package website.msdnna.tessera.ui.viewmodels

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import website.msdnna.tessera.data.model.Tag
import website.msdnna.tessera.data.model.WorkspaceSummary
import website.msdnna.tessera.data.model.WorkspaceTask
import website.msdnna.tessera.data.repository.HomeRepository
import website.msdnna.tessera.ui.UiText
import website.msdnna.tessera.util.DateFormatPrefs
import website.msdnna.tessera.util.dayKeyIn
import website.msdnna.tessera.util.dueDayKey
import website.msdnna.tessera.util.errorMessage

enum class HomeFilter { Me, All, Overdue, Today, Week, Completed }

data class HomeUiState(
    val loading: Boolean = true,
    val error: UiText? = null,
    val summary: WorkspaceSummary = WorkspaceSummary(),
    val tasks: List<WorkspaceTask> = emptyList(),
    val tags: Map<String, Tag> = emptyMap(),
    val filter: HomeFilter = HomeFilter.Me,
    val meId: String = "",
    /**
     * Календарь пользователя, пришедший из композиции через [HomeViewModel.setDateFormat].
     *
     * Плитки над списком считает СЕРВЕР, и зону для них клиент сообщает сам —
     * `?tz=` из `AppContainer.callerZone()` (преф профиля, иначе зона устройства).
     * Пока список под ними бакетился зоной устройства, у человека с префом,
     * разошедшимся с телефоном, «Просрочено: 3» стояло над списком из другого числа
     * задач. Здесь тот же преф, чтобы половины одного экрана считали один календарь.
     */
    val dateFormat: DateFormatPrefs = DateFormatPrefs.Default,
) {
    /** The task list for the active filter — mirrors HomeView's per-card filter. */
    val visibleTasks: List<WorkspaceTask>
        get() {
            val today = dayKeyIn(dateFormat.zone, 0)
            val weekEnd = dayKeyIn(dateFormat.zone, DAYS_IN_WEEK)
            return tasks.filter { t ->
                // День срока — тот же, что на карточке (dueDayKey), а не срез сырой
                // строки; и границы выше, и он сам читаются в [dateFormat].
                val due = dueDayKey(t.dueDate, dateFormat)
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

    /**
     * The calendar the list buckets days in, handed down from composition
     * (`LocalDateFormat`) — the same preference the summary request already sends
     * the server as `?tz=`. See [HomeUiState.dateFormat].
     */
    fun setDateFormat(fmt: DateFormatPrefs) {
        if (_state.value.dateFormat == fmt) return
        _state.update { it.copy(dateFormat = fmt) }
    }

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
