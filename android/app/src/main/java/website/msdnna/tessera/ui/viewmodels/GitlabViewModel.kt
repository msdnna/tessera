package website.msdnna.tessera.ui.viewmodels

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import website.msdnna.tessera.R
import website.msdnna.tessera.data.model.GitlabIntegration
import website.msdnna.tessera.data.model.GitlabIntegrationRequest
import website.msdnna.tessera.data.model.TagPrefixEntry
import website.msdnna.tessera.data.repository.BoardOption
import website.msdnna.tessera.data.repository.GitlabRepository
import website.msdnna.tessera.ui.UiText
import website.msdnna.tessera.util.errorMessage

data class GitlabUiState(
    val loading: Boolean = true,
    val error: UiText? = null,
    val message: UiText? = null,
    // connection (personal PAT — a fallback when no instance service token)
    val connected: Boolean = false,
    val baseUrl: String = "",
    val glUsername: String = "",
    val connecting: Boolean = false,
    // bindings (multiple GL-project → board per workspace) + capability flags
    val integrations: List<GitlabIntegration> = emptyList(),
    val serviceConfigured: Boolean = false,
    val isAdmin: Boolean = false,
    // pickers
    val boards: List<BoardOption> = emptyList(),
    // editor context: columns + prefix display names of the board being edited
    val columns: List<String> = emptyList(),
    // column (id, name) pairs of the edited board — for the write-back binding
    // column trigger (matches by stable id).
    val columnOptions: List<Pair<String, String>> = emptyList(),
    val prefixNames: Map<String, String> = emptyMap(),
    val saving: Boolean = false,
    // id of the binding a sync is currently running for (null = none)
    val syncingId: String? = null,
    // true when the running sync was started as a full sweep (drives which of the
    // two sync buttons shows the spinner)
    val syncingFull: Boolean = false,
)

/** Owns the GitLab settings screen: the per-user connection, the per-workspace
 *  bindings (multiple GL-project → board) with CRUD + manual sync, and the
 *  board/column lists its editor needs. Mirrors the web `GitLabModal`. */
class GitlabViewModel : ViewModel() {
    private val repo = GitlabRepository()
    private val _state = MutableStateFlow(GitlabUiState())
    val state: StateFlow<GitlabUiState> = _state.asStateFlow()

    fun loadAll(workspaceId: String) {
        viewModelScope.launch {
            _state.update { it.copy(loading = true, error = null) }
            try {
                val conn = repo.connection()
                val boards = repo.workspaceBoards(workspaceId)
                val resp = repo.integrations(workspaceId)
                _state.update {
                    it.copy(
                        loading = false, connected = conn.connected, baseUrl = conn.baseUrl,
                        glUsername = conn.glUsername, boards = boards,
                        integrations = resp.integrations, serviceConfigured = resp.serviceConfigured,
                        isAdmin = resp.isAdmin,
                    )
                }
            } catch (e: Exception) {
                _state.update { it.copy(loading = false, error = errorMessage(e)) }
            }
        }
    }

    private suspend fun reloadIntegrations(workspaceId: String) {
        runCatching { repo.integrations(workspaceId) }.getOrNull()?.let { resp ->
            _state.update {
                it.copy(
                    integrations = resp.integrations, serviceConfigured = resp.serviceConfigured,
                    isAdmin = resp.isAdmin,
                )
            }
        }
    }

    fun connect(workspaceId: String, baseUrl: String, token: String) {
        viewModelScope.launch {
            _state.update { it.copy(connecting = true, error = null) }
            try {
                val conn = repo.connect(baseUrl.trim().trimEnd('/'), token.trim())
                _state.update {
                    it.copy(
                        connecting = false, connected = conn.connected, baseUrl = conn.baseUrl,
                        glUsername = conn.glUsername,
                        message = UiText.Res(R.string.gitlab_msg_connected, listOf(conn.glUsername)),
                    )
                }
                reloadIntegrations(workspaceId)
            } catch (e: Exception) {
                _state.update { it.copy(connecting = false, error = errorMessage(e)) }
            }
        }
    }

    fun disconnect() {
        viewModelScope.launch {
            try {
                repo.disconnect()
                _state.update {
                    it.copy(
                        connected = false, glUsername = "",
                        message = UiText.Res(R.string.gitlab_msg_disconnected),
                    )
                }
            } catch (e: Exception) {
                _state.update { it.copy(error = errorMessage(e)) }
            }
        }
    }

    fun loadColumns(boardId: String) {
        viewModelScope.launch {
            val cols = runCatching { repo.columns(boardId) }.getOrDefault(emptyList())
            _state.update { it.copy(columns = cols.map { c -> c.second }, columnOptions = cols) }
        }
    }

    /** Loads the prefix display names of the project that owns [boardId] — invoked
     *  when the editor targets a different board so the rule editor prefills. */
    fun loadPrefixNamesForBoard(boardId: String?) {
        val pid = _state.value.boards.find { it.id == boardId }?.projectId
        if (pid.isNullOrBlank()) {
            _state.update { it.copy(prefixNames = emptyMap()) }
            return
        }
        viewModelScope.launch {
            val names = runCatching { repo.tagPrefixes(pid).associate { it.prefix to it.label } }.getOrDefault(emptyMap())
            _state.update { it.copy(prefixNames = names) }
        }
    }

    /** Prime the editor context (columns + prefix names) for an existing binding. */
    fun prepareEditor(integ: GitlabIntegration?) {
        val boardId = integ?.boardId
        if (boardId.isNullOrBlank()) {
            _state.update { it.copy(columns = emptyList(), columnOptions = emptyList(), prefixNames = emptyMap()) }
            return
        }
        loadColumns(boardId)
        loadPrefixNamesForBoard(boardId)
    }

    /**
     * Create ([existingId] == null) or update a binding. [ruleLabels] (canonical
     * prefix → label, blank = remove) is merged into the target project's tag-prefix
     * store first (web GitLabModal merge-save). [onDone] closes the editor on success.
     */
    fun saveIntegration(
        workspaceId: String,
        existingId: String?,
        projectId: String?,
        ruleLabels: Map<String, String>,
        req: GitlabIntegrationRequest,
        onDone: () -> Unit,
    ) {
        viewModelScope.launch {
            _state.update { it.copy(saving = true, error = null) }
            try {
                if (!projectId.isNullOrBlank()) {
                    val merged = _state.value.prefixNames.toMutableMap()
                    ruleLabels.forEach { (key, raw) ->
                        if (key.isBlank()) return@forEach
                        val label = raw.trim()
                        if (label.isNotBlank()) merged[key] = label else merged.remove(key)
                    }
                    runCatching { repo.setTagPrefixes(projectId, merged.map { TagPrefixEntry(it.key, it.value) }) }
                        .onSuccess { _state.update { st -> st.copy(prefixNames = merged) } }
                }
                if (existingId == null) {
                    repo.createIntegration(workspaceId, req)
                } else {
                    repo.updateIntegration(workspaceId, existingId, req)
                }
                reloadIntegrations(workspaceId)
                _state.update { it.copy(saving = false, message = UiText.Res(R.string.gitlab_msg_saved)) }
                onDone()
            } catch (e: Exception) {
                _state.update { it.copy(saving = false, error = errorMessage(e)) }
            }
        }
    }

    fun deleteIntegration(workspaceId: String, id: String) {
        viewModelScope.launch {
            try {
                repo.deleteIntegration(workspaceId, id)
                reloadIntegrations(workspaceId)
                _state.update { it.copy(message = UiText.Res(R.string.gitlab_msg_binding_deleted)) }
            } catch (e: Exception) {
                _state.update { it.copy(error = errorMessage(e)) }
            }
        }
    }

    /** [full] = true runs a full sweep (`?mode=full`) — it also re-checks issues the
     *  incremental pull skips, so deletes and drift in GitLab land on the board. */
    fun sync(workspaceId: String, integrationId: String, full: Boolean = false) {
        viewModelScope.launch {
            _state.update { it.copy(syncingId = integrationId, syncingFull = full, error = null) }
            // The sync runs in the background server-side (a large batch used to drop
            // the long request). Kick it off, then poll the workspace journal for the
            // run to finish so we can still show totals.
            val baselineId = runCatching { repo.syncRuns(workspaceId).firstOrNull()?.id }.getOrNull()
            try {
                repo.sync(workspaceId, integrationId, full)
            } catch (e: Exception) {
                _state.update { it.copy(syncingId = null, syncingFull = false, error = errorMessage(e)) }
                return@launch
            }
            repeat(900) {
                // 900 × 2s ≈ 30 min cap
                kotlinx.coroutines.delay(2000)
                val runs = runCatching { repo.syncRuns(workspaceId) }.getOrNull() ?: return@repeat
                val run = runs.firstOrNull { it.kind == "pull" && it.finishedAt != null && it.id != baselineId }
                    ?: return@repeat
                _state.update {
                    if (run.status == "error") {
                        it.copy(
                            syncingId = null, syncingFull = false,
                            error = if (run.error.isBlank()) {
                                UiText.Res(R.string.gitlab_err_sync_failed_unknown)
                            } else {
                                UiText.Res(R.string.gitlab_err_sync_failed, listOf(run.error))
                            },
                        )
                    } else {
                        it.copy(
                            syncingId = null, syncingFull = false,
                            message = UiText.Res(
                                R.string.gitlab_msg_synced,
                                listOf(
                                    run.createdCount + run.updatedCount,
                                    run.createdCount,
                                    run.updatedCount,
                                ),
                            ),
                        )
                    }
                }
                reloadIntegrations(workspaceId)
                return@launch
            }
            _state.update {
                it.copy(
                    syncingId = null, syncingFull = false,
                    message = UiText.Res(R.string.gitlab_msg_sync_background),
                )
            }
        }
    }

    fun clearMessage() = _state.update { it.copy(message = null, error = null) }
}
