package website.msdnna.tessera.ui.viewmodels

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.model.Invitation
import website.msdnna.tessera.data.model.Member
import website.msdnna.tessera.data.repository.WorkspaceRepository
import website.msdnna.tessera.ui.UiText
import website.msdnna.tessera.util.canManageMembers
import website.msdnna.tessera.util.errorMessage
import website.msdnna.tessera.util.rawErrorText

data class MembersUiState(
    val loading: Boolean = true,
    val error: UiText? = null,
    val members: List<Member> = emptyList(),
    val invitations: List<Invitation> = emptyList(),
    val lastInviteLink: String = "",
    val busy: Boolean = false,
    /** The viewer, so their own row can stop offering self-demotion and self-removal. */
    val meId: String = "",
    /** Whether the viewer passes `requireManager` here — see [canManageMembers]. */
    val canManage: Boolean = false,
)

/** Workspace members modal: list members, invite (instant add or email link), remove. */
class MembersViewModel(
    private val repo: WorkspaceRepository = WorkspaceRepository(),
    // Injected so the dialog's rules can be tested without a DataStore; production
    // reads the signed-in profile exactly as the board does.
    private val readMeId: suspend () -> String = { AppContainer.prefs.user.first()?.id.orEmpty() },
) : ViewModel() {
    private val _state = MutableStateFlow(MembersUiState())
    val state: StateFlow<MembersUiState> = _state.asStateFlow()

    private var workspaceId: String = ""

    /**
     * The two lists are fetched behind different gates — ListMembers is
     * `requireMember`, ListInvitations is `requireManager` — so fetching them in
     * one `runCatching` threw away the roster the server had already returned
     * when the second answered 403: an empty dialog behind a red error, on the
     * one screen that answers "who else is in this workspace". They are
     * independent now, and the manager-only one is not asked for at all by
     * someone the server would refuse it to.
     */
    fun load(workspaceId: String) {
        this.workspaceId = workspaceId
        _state.update { it.copy(loading = true, error = null) }
        viewModelScope.launch {
            val meId = runCatching { readMeId() }.getOrDefault("")
            val roster = runCatching { repo.members(workspaceId) }
            val members = roster.getOrElse { e ->
                _state.update { it.copy(loading = false, meId = meId, error = errorMessage(e)) }
                return@launch
            }
            val canManage = canManageMembers(members, meId)
            _state.update {
                it.copy(loading = false, meId = meId, members = members, canManage = canManage)
            }
            if (!canManage) return@launch
            runCatching { repo.invitations(workspaceId) }
                .onSuccess { invs -> _state.update { it.copy(invitations = invs) } }
                .onFailure { e -> _state.update { it.copy(error = errorMessage(e)) } }
        }
    }

    // Add an already-registered user instantly; otherwise create an email
    // invitation (link surfaced for sharing). Mirrors the web flow.
    fun invite(email: String, role: String) {
        if (email.isBlank() || workspaceId.isBlank()) return
        _state.update { it.copy(busy = true, error = null) }
        viewModelScope.launch {
            val added = runCatching { repo.addMember(workspaceId, email, role) }
            if (added.isSuccess) {
                _state.update {
                    it.copy(busy = false, lastInviteLink = "", members = repo.members(workspaceId), invitations = repo.invitations(workspaceId))
                }
                return@launch
            }
            val failure = added.exceptionOrNull()!!
            // Ветвимся по сырому ответу сервера, а не по показываемому тексту:
            // тот переводится, и на английском интерфейсе «no user» не нашлось бы.
            if (rawErrorText(failure)?.contains("no user", ignoreCase = true) == true) {
                runCatching { repo.createInvitation(workspaceId, email, role) }
                    .onSuccess { inv ->
                        val invs = repo.invitations(workspaceId)
                        _state.update { it.copy(busy = false, lastInviteLink = inv.link, invitations = invs) }
                    }
                    .onFailure { e -> _state.update { it.copy(busy = false, error = errorMessage(e)) } }
            } else {
                _state.update { it.copy(busy = false, error = errorMessage(failure)) }
            }
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

    fun revokeInvite(invId: String) = launchCatching {
        repo.deleteInvitation(workspaceId, invId)
        _state.update { it.copy(invitations = repo.invitations(workspaceId)) }
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
