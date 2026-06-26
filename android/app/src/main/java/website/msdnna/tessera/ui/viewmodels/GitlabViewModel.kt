package website.msdnna.tessera.ui.viewmodels

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import website.msdnna.tessera.data.model.GitlabIntegration
import website.msdnna.tessera.data.model.GitlabSetIntegrationRequest
import website.msdnna.tessera.data.model.TagPrefixEntry
import website.msdnna.tessera.data.repository.BoardOption
import website.msdnna.tessera.data.repository.GitlabRepository
import website.msdnna.tessera.util.errorMessage

data class GitlabUiState(
    val loading: Boolean = true,
    val error: String? = null,
    val message: String? = null,
    // connection
    val connected: Boolean = false,
    val baseUrl: String = "",
    val glUsername: String = "",
    val connecting: Boolean = false,
    // integration + pickers
    val integration: GitlabIntegration? = null,
    val boards: List<BoardOption> = emptyList(),
    val columns: List<String> = emptyList(),
    /** The target project's loaded tag-prefix display names (canonical → label),
     *  for the prefix-rule "понятное имя" editor + merge-on-save. */
    val prefixNames: Map<String, String> = emptyMap(),
    val saving: Boolean = false,
    val syncing: Boolean = false,
)

/** Owns the GitLab settings screen: the per-user connection, the per-workspace
 *  integration config + manual sync, and the board/column lists its editor needs. */
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
                var integ: GitlabIntegration? = null
                var cols: List<String> = emptyList()
                var prefixNames: Map<String, String> = emptyMap()
                if (conn.connected) {
                    integ = repo.integration(workspaceId)
                    integ.boardId?.let { bid ->
                        cols = runCatching { repo.columnNames(bid) }.getOrDefault(emptyList())
                        boards.find { it.id == bid }?.projectId?.takeIf { it.isNotBlank() }?.let { pid ->
                            prefixNames = runCatching { repo.tagPrefixes(pid).associate { p -> p.prefix to p.label } }
                                .getOrDefault(emptyMap())
                        }
                    }
                }
                _state.update {
                    it.copy(
                        loading = false, connected = conn.connected, baseUrl = conn.baseUrl,
                        glUsername = conn.glUsername, integration = integ, boards = boards,
                        columns = cols, prefixNames = prefixNames,
                    )
                }
            } catch (e: Exception) {
                _state.update { it.copy(loading = false, error = errorMessage(e)) }
            }
        }
    }

    fun connect(workspaceId: String, baseUrl: String, token: String) {
        viewModelScope.launch {
            _state.update { it.copy(connecting = true, error = null) }
            try {
                val conn = repo.connect(baseUrl.trim().trimEnd('/'), token.trim())
                val integ = repo.integration(workspaceId)
                _state.update {
                    it.copy(
                        connecting = false, connected = conn.connected, baseUrl = conn.baseUrl,
                        glUsername = conn.glUsername, integration = integ, message = "Подключено как @${conn.glUsername}",
                    )
                }
            } catch (e: Exception) {
                _state.update { it.copy(connecting = false, error = errorMessage(e)) }
            }
        }
    }

    fun disconnect() {
        viewModelScope.launch {
            try {
                repo.disconnect()
                _state.update { it.copy(connected = false, glUsername = "", integration = null, message = "GitLab отключён") }
            } catch (e: Exception) {
                _state.update { it.copy(error = errorMessage(e)) }
            }
        }
    }

    fun loadColumns(boardId: String) {
        viewModelScope.launch {
            val cols = runCatching { repo.columnNames(boardId) }.getOrDefault(emptyList())
            _state.update { it.copy(columns = cols) }
        }
    }

    /** Loads the prefix display names of the project that owns [boardId] — invoked
     *  when the user picks a different target board so the rule editor prefills. */
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

    /**
     * Saves the integration. [ruleLabels] (canonical prefix → label, blank = remove)
     * is merged into the project's existing prefix display names and PUT first — the
     * editor only manages prefixes that have a rule, so unmanaged ones survive (web
     * GitLabModal merge-save). [projectId] is the target board's project.
     */
    fun save(workspaceId: String, projectId: String?, ruleLabels: Map<String, String>, req: GitlabSetIntegrationRequest) {
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
                val integ = repo.setIntegration(workspaceId, req)
                _state.update { it.copy(saving = false, integration = integ, message = "Настройки сохранены") }
            } catch (e: Exception) {
                _state.update { it.copy(saving = false, error = errorMessage(e)) }
            }
        }
    }

    fun sync(workspaceId: String) {
        viewModelScope.launch {
            _state.update { it.copy(syncing = true, error = null) }
            // The sync now runs in the background server-side (a large batch used to
            // drop the long request). Kick it off, then poll the journal for the run
            // to finish so we can still show totals.
            val baselineId = runCatching { repo.syncRuns(workspaceId).firstOrNull()?.id }.getOrNull()
            try {
                repo.sync(workspaceId)
            } catch (e: Exception) {
                _state.update { it.copy(syncing = false, error = errorMessage(e)) }
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
                        it.copy(syncing = false, error = "Синхронизация не удалась: ${run.error.ifBlank { "ошибка" }}")
                    } else {
                        it.copy(
                            syncing = false,
                            message = "Синхронизировано: ${run.createdCount + run.updatedCount} (+${run.createdCount}, ~${run.updatedCount})",
                        )
                    }
                }
                return@launch
            }
            _state.update { it.copy(syncing = false, message = "Синхронизация выполняется в фоне — см. журнал") }
        }
    }

    fun clearMessage() = _state.update { it.copy(message = null, error = null) }
}
