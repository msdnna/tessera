package website.msdnna.tessera.ui.viewmodels

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import website.msdnna.tessera.data.model.Milestone
import website.msdnna.tessera.data.model.WorkspaceMilestone
import website.msdnna.tessera.data.repository.MilestoneRepository
import website.msdnna.tessera.ui.UiText
import website.msdnna.tessera.util.errorMessage

data class MilestonesUiState(
    val loading: Boolean = true,
    val error: UiText? = null,
    val milestones: List<WorkspaceMilestone> = emptyList(),
    /** Closed «Этапы» hidden by default (web «Активные/Все» toggle). */
    val showClosed: Boolean = false,
) {
    /** Active-or-all rows, grouped by project, in server order. */
    val visible: List<WorkspaceMilestone>
        get() = if (showClosed) milestones else milestones.filter { !it.isClosed }
}

/** Per-project milestone manager («Этапы» editor) state, opened over the screen. */
data class MilestoneManagerState(
    val projectId: String = "",
    val projectName: String = "",
    val glCapable: Boolean = false,
    val loading: Boolean = true,
    val saving: Boolean = false,
    val error: UiText? = null,
    val milestones: List<Milestone> = emptyList(),
)

/**
 * Owns the workspace «Этапы» aggregate (cross-project roadmap) plus the per-project
 * milestone manager. Online-first: every mutation reloads from the server.
 */
class MilestoneViewModel(
    private val repo: MilestoneRepository = MilestoneRepository(),
) : ViewModel() {
    private val _state = MutableStateFlow(MilestonesUiState())
    val state: StateFlow<MilestonesUiState> = _state.asStateFlow()

    private val _manager = MutableStateFlow<MilestoneManagerState?>(null)
    val manager: StateFlow<MilestoneManagerState?> = _manager.asStateFlow()

    private var workspaceId: String = ""

    fun load(workspaceId: String) {
        this.workspaceId = workspaceId
        _state.update { it.copy(loading = true, error = null) }
        viewModelScope.launch {
            runCatching { repo.workspaceList(workspaceId) }
                .onSuccess { list -> _state.update { it.copy(loading = false, milestones = list) } }
                .onFailure { e -> _state.update { it.copy(loading = false, error = errorMessage(e)) } }
        }
    }

    fun toggleShowClosed() = _state.update { it.copy(showClosed = !it.showClosed) }

    private fun reloadWorkspace() {
        if (workspaceId.isBlank()) return
        viewModelScope.launch {
            runCatching { repo.workspaceList(workspaceId) }
                .onSuccess { list -> _state.update { it.copy(milestones = list) } }
        }
    }

    // ── per-project manager ────────────────────────────────────────────────────

    fun openManager(projectId: String, projectName: String, glCapable: Boolean) {
        _manager.value = MilestoneManagerState(
            projectId = projectId, projectName = projectName, glCapable = glCapable, loading = true,
        )
        loadManager(projectId)
    }

    fun closeManager() {
        _manager.value = null
    }

    private fun loadManager(projectId: String) {
        viewModelScope.launch {
            runCatching { repo.list(projectId) }
                .onSuccess { ms -> _manager.update { it?.copy(loading = false, milestones = ms) } }
                .onFailure { e -> _manager.update { it?.copy(loading = false, error = errorMessage(e)) } }
        }
    }

    private fun mutateManager(block: suspend (String) -> Unit) {
        val projectId = _manager.value?.projectId ?: return
        _manager.update { it?.copy(saving = true, error = null) }
        viewModelScope.launch {
            runCatching { block(projectId) }
                .onFailure { e -> _manager.update { it?.copy(error = errorMessage(e)) } }
            _manager.update { it?.copy(saving = false) }
            loadManager(projectId)
            reloadWorkspace()
        }
    }

    fun createMilestone(title: String, startDate: String?, dueDate: String?) = mutateManager { projectId ->
        if (title.isBlank()) return@mutateManager
        repo.create(projectId, title, startDate, dueDate)
    }

    fun updateMilestone(
        milestoneId: String,
        title: String,
        description: String,
        startDate: String?,
        dueDate: String?,
        state: String,
    ) = mutateManager {
        repo.update(milestoneId, title, description, startDate, dueDate, state)
    }

    fun toggleState(m: Milestone) = mutateManager {
        repo.update(
            m.id, m.title, m.description, m.startDate, m.dueDate,
            if (m.isClosed) "active" else "closed",
        )
    }

    fun deleteMilestone(milestoneId: String) = mutateManager {
        repo.delete(milestoneId)
    }

    fun pushToGitlab(milestoneId: String) = mutateManager {
        repo.pushToGitlab(milestoneId)
    }
}
