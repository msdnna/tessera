package website.msdnna.tessera.ui.viewmodels

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import website.msdnna.tessera.data.model.GitlabSyncAction
import website.msdnna.tessera.data.model.GitlabSyncRun
import website.msdnna.tessera.data.repository.GitlabRepository
import website.msdnna.tessera.util.errorMessage

data class GitlabJournalUiState(
    val loading: Boolean = true,
    val error: String? = null,
    val message: String? = null,
    val runs: List<GitlabSyncRun> = emptyList(),
    val expandedRunId: String? = null,
    /** runId → its loaded actions (lazily fetched on first expand). */
    val actionsByRun: Map<String, List<GitlabSyncAction>> = emptyMap(),
    val loadingActions: Boolean = false,
    /** The action whose detail/diff is shown in the detail dialog, paired with its run. */
    val selected: Pair<GitlabSyncRun, GitlabSyncAction>? = null,
    val retrying: Boolean = false,
)

/** Owns the GitLab sync-journal screen: the run list, lazily-loaded actions per
 *  run, and retrying a failed push. Mirrors the web `GitLabJournalModal`. */
class GitlabJournalViewModel : ViewModel() {
    private val repo = GitlabRepository()
    private val _state = MutableStateFlow(GitlabJournalUiState())
    val state: StateFlow<GitlabJournalUiState> = _state.asStateFlow()

    fun load(workspaceId: String) {
        viewModelScope.launch {
            _state.update {
                it.copy(loading = true, error = null, runs = emptyList(), actionsByRun = emptyMap(), expandedRunId = null, selected = null)
            }
            try {
                val runs = repo.syncRuns(workspaceId)
                _state.update { it.copy(loading = false, runs = runs) }
            } catch (e: Exception) {
                _state.update { it.copy(loading = false, error = errorMessage(e)) }
            }
        }
    }

    fun toggleRun(workspaceId: String, run: GitlabSyncRun) {
        if (_state.value.expandedRunId == run.id) {
            _state.update { it.copy(expandedRunId = null) }
            return
        }
        _state.update { it.copy(expandedRunId = run.id) }
        if (_state.value.actionsByRun.containsKey(run.id)) return
        viewModelScope.launch {
            _state.update { it.copy(loadingActions = true) }
            try {
                val actions = repo.syncActions(workspaceId, run.id)
                _state.update { it.copy(loadingActions = false, actionsByRun = it.actionsByRun + (run.id to actions)) }
            } catch (e: Exception) {
                _state.update { it.copy(loadingActions = false, error = errorMessage(e)) }
            }
        }
    }

    fun select(run: GitlabSyncRun, action: GitlabSyncAction) =
        _state.update { it.copy(selected = run to action) }

    fun closeDetail() = _state.update { it.copy(selected = null) }

    fun retry(workspaceId: String) {
        val sel = _state.value.selected ?: return
        viewModelScope.launch {
            _state.update { it.copy(retrying = true, error = null) }
            try {
                repo.retryWriteback(workspaceId, sel.first.id, sel.second.id)
                _state.update { it.copy(retrying = false, message = "Поставлено в очередь на повтор") }
            } catch (e: Exception) {
                _state.update { it.copy(retrying = false, error = errorMessage(e)) }
            }
        }
    }

    fun clearMessage() = _state.update { it.copy(message = null, error = null) }
}
