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
                if (conn.connected) {
                    integ = repo.integration(workspaceId)
                    integ.boardId?.let { cols = runCatching { repo.columnNames(it) }.getOrDefault(emptyList()) }
                }
                _state.update {
                    it.copy(
                        loading = false, connected = conn.connected, baseUrl = conn.baseUrl,
                        glUsername = conn.glUsername, integration = integ, boards = boards, columns = cols,
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

    fun save(workspaceId: String, req: GitlabSetIntegrationRequest) {
        viewModelScope.launch {
            _state.update { it.copy(saving = true, error = null) }
            try {
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
            try {
                val r = repo.sync(workspaceId)
                _state.update {
                    it.copy(syncing = false, message = "Синхронизировано: ${r.total} (+${r.created}, ~${r.updated})")
                }
            } catch (e: Exception) {
                _state.update { it.copy(syncing = false, error = errorMessage(e)) }
            }
        }
    }

    fun clearMessage() = _state.update { it.copy(message = null, error = null) }
}
