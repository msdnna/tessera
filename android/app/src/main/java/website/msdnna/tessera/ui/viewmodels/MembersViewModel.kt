package website.msdnna.tessera.ui.viewmodels

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import website.msdnna.tessera.data.model.Member
import website.msdnna.tessera.data.repository.WorkspaceRepository
import website.msdnna.tessera.util.errorMessage

data class MembersUiState(
    val loading: Boolean = true,
    val error: String? = null,
    val members: List<Member> = emptyList(),
    val busy: Boolean = false,
)

/** Workspace members modal: list, invite an existing user by email, remove. */
class MembersViewModel(
    private val repo: WorkspaceRepository = WorkspaceRepository(),
) : ViewModel() {
    private val _state = MutableStateFlow(MembersUiState())
    val state: StateFlow<MembersUiState> = _state.asStateFlow()

    private var workspaceId: String = ""

    fun load(workspaceId: String) {
        this.workspaceId = workspaceId
        _state.update { it.copy(loading = true, error = null) }
        launchCatching {
            val members = repo.members(workspaceId)
            _state.update { it.copy(loading = false, members = members) }
        }
    }

    fun invite(email: String, role: String) {
        if (email.isBlank() || workspaceId.isBlank()) return
        _state.update { it.copy(busy = true, error = null) }
        launchCatching {
            repo.addMember(workspaceId, email, role)
            _state.update { it.copy(busy = false, members = repo.members(workspaceId)) }
        }
    }

    fun remove(userId: String) = launchCatching {
        repo.removeMember(workspaceId, userId)
        _state.update { it.copy(members = repo.members(workspaceId)) }
    }

    fun changeRole(userId: String, role: String) = launchCatching {
        repo.updateMemberRole(workspaceId, userId, role)
        _state.update { it.copy(members = repo.members(workspaceId)) }
    }

    fun clearError() = _state.update { it.copy(error = null) }

    private fun launchCatching(block: suspend () -> Unit) {
        viewModelScope.launch {
            runCatching { block() }.exceptionOrNull()?.let { e ->
                _state.update { it.copy(loading = false, busy = false, error = errorMessage(e)) }
            }
        }
    }
}
