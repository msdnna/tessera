package website.msdnna.tessera.ui.viewmodels

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import website.msdnna.tessera.data.model.GitlabConflict
import website.msdnna.tessera.data.realtime.RealtimeClient
import website.msdnna.tessera.data.realtime.RealtimeEvent
import website.msdnna.tessera.data.repository.GitlabRepository

data class ConflictsUiState(
    val list: List<GitlabConflict> = emptyList(),
    /** Resolver modal open (focused on [focusTaskId] when set). */
    val resolverOpen: Boolean = false,
    val focusTaskId: String? = null,
    val resolving: Boolean = false,
) {
    val count: Int get() = list.size
    val taskIds: Set<String> get() = list.map { it.taskId }.toSet()
    fun has(taskId: String): Boolean = taskIds.contains(taskId)
}

/**
 * Workspace-scoped GitLab write-back conflicts (the Android mirror of the web
 * `stores/conflicts`). Drives the count badges, the «Конфликт» card pill and the
 * resolver modal. Online-first: a live `gitlab.conflict` event reloads the list.
 */
class ConflictsViewModel(
    private val repo: GitlabRepository = GitlabRepository(),
) : ViewModel() {
    private val _state = MutableStateFlow(ConflictsUiState())
    val state: StateFlow<ConflictsUiState> = _state.asStateFlow()

    private var workspaceId: String = ""
    private var realtime: RealtimeClient? = null

    fun load(workspaceId: String) {
        this.workspaceId = workspaceId
        ensureRealtime()
        reload()
    }

    fun reload() {
        if (workspaceId.isBlank()) {
            _state.update { it.copy(list = emptyList()) }
            return
        }
        viewModelScope.launch {
            val list = repo.conflicts(workspaceId)
            _state.update { it.copy(list = list) }
        }
    }

    fun openResolver(taskId: String? = null) {
        _state.update { it.copy(resolverOpen = true, focusTaskId = taskId) }
    }

    fun closeResolver() {
        _state.update { it.copy(resolverOpen = false, focusTaskId = null) }
    }

    /** Resolve a conflict (ours|theirs|manual); reloads on success. */
    fun resolve(c: GitlabConflict, resolution: String, value: Map<String, String>? = null) {
        _state.update { it.copy(resolving = true) }
        viewModelScope.launch {
            runCatching { repo.resolveConflict(c.taskId, c.id, resolution, value) }
            // Drop it locally for snappy UI; the realtime echo / reload reconciles.
            _state.update {
                val remaining = it.list.filterNot { x -> x.id == c.id }
                it.copy(
                    list = remaining,
                    resolving = false,
                    resolverOpen = if (remaining.isEmpty()) false else it.resolverOpen,
                )
            }
            reload()
        }
    }

    private fun ensureRealtime() {
        if (realtime != null) return
        realtime = RealtimeClient(::onRealtimeEvent).also { it.connect() }
    }

    private fun onRealtimeEvent(ev: RealtimeEvent) {
        if (ev.type == "gitlab.conflict" && (workspaceId.isBlank() || ev.scope == workspaceId)) reload()
    }

    override fun onCleared() {
        realtime?.close()
        realtime = null
    }
}
