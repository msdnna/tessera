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
import website.msdnna.tessera.data.model.AccessRequest
import website.msdnna.tessera.data.model.Invitation
import website.msdnna.tessera.data.model.Member
import website.msdnna.tessera.data.model.Workspace
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
    /** The workspace itself — its name is what a transfer is confirmed with (#3072). */
    val workspace: Workspace? = null,
    /** Pending access requests (#3077), managers only. */
    val accessRequests: List<AccessRequest> = emptyList(),
    /** The viewer left the workspace — the dialog closes. */
    val left: Boolean = false,
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
        // Всё состояние ниже принадлежит ОДНОМУ пространству, а ViewModel живёт
        // дольше окна: `viewModel()` в MembersModal привязан к активности, не к
        // диалогу, так что открытие на другом пространстве заставало в нём
        // ростер, приглашения и ссылку предыдущего (#2932). Видно это было не
        // только мельканием: ветка с упавшим ростером возвращается, ничего не
        // записав, — и чужой список оставался на экране под красной плашкой,
        // выдавая себя за этот. Ссылка вдобавок — носитель токена приглашения в
        // то, другое пространство.
        _state.update {
            it.copy(
                loading = true,
                error = null,
                members = emptyList(),
                invitations = emptyList(),
                lastInviteLink = "",
                canManage = false,
                workspace = null,
                accessRequests = emptyList(),
                left = false,
            )
        }
        viewModelScope.launch {
            val meId = runCatching { readMeId() }.getOrDefault("")
            val roster = runCatching { repo.members(workspaceId) }
            if (stale(workspaceId)) return@launch
            val members = roster.getOrElse { e ->
                _state.update { it.copy(loading = false, meId = meId, error = errorMessage(e)) }
                return@launch
            }
            val canManage = canManageMembers(members, meId)
            _state.update {
                it.copy(loading = false, meId = meId, members = members, canManage = canManage)
            }
            val ws = runCatching { repo.workspace(workspaceId) }.getOrNull()
            if (stale(workspaceId)) return@launch
            _state.update { it.copy(workspace = ws) }
            if (!canManage) return@launch
            val invitations = runCatching { repo.invitations(workspaceId) }
            val requests = runCatching { repo.accessRequests(workspaceId) }
            if (stale(workspaceId)) return@launch
            invitations
                .onSuccess { invs -> _state.update { it.copy(invitations = invs) } }
                .onFailure { e -> _state.update { it.copy(error = errorMessage(e)) } }
            requests.onSuccess { reqs -> _state.update { it.copy(accessRequests = reqs) } }
        }
    }

    /**
     * Ответ пришёл уже не про то пространство, что открыто сейчас.
     *
     * Две загрузки бывают в полёте одновременно: диалог закрыли, переключили
     * пространство и открыли снова, пока первый запрос ещё идёт. Без этой
     * проверки поздний ответ дописывал бы в состояние ростер предыдущего — ту же
     * картину, что и отсутствие сброса выше, только с задержкой.
     */
    private fun stale(requested: String): Boolean = workspaceId != requested

    // Add an already-registered user instantly; otherwise create an email
    // invitation (link surfaced for sharing). Mirrors the web flow.
    fun invite(email: String, role: String) {
        if (email.isBlank() || workspaceId.isBlank()) return
        _state.update { it.copy(busy = true, error = null) }
        viewModelScope.launch {
            val added = runCatching { repo.addMember(workspaceId, email, role) }
            if (added.isSuccess) {
                _state.update { it.copy(lastInviteLink = "") }
                refreshLists()
                return@launch
            }
            val failure = added.exceptionOrNull()!!
            // Ветвимся по сырому ответу сервера, а не по показываемому тексту:
            // тот переводится, и на английском интерфейсе «no user» не нашлось бы.
            if (rawErrorText(failure)?.contains("no user", ignoreCase = true) == true) {
                runCatching { repo.createInvitation(workspaceId, email, role) }
                    .onSuccess { inv ->
                        // Ссылку показываем ДО перечитывания списка: приглашение уже
                        // создано, и при выключенном SMTP она — единственный носитель
                        // токена. Упавший следом GET не должен её съесть.
                        _state.update { it.copy(lastInviteLink = inv.link) }
                        refreshLists()
                    }
                    .onFailure { e -> _state.update { it.copy(busy = false, error = errorMessage(e)) } }
            } else {
                _state.update { it.copy(busy = false, error = errorMessage(failure)) }
            }
        }
    }

    /**
     * Перечитывает ростер и приглашения после успешной записи — и не роняет
     * приложение, если сеть мигнула ровно тут.
     *
     * Раньше оба запроса стояли прямо в аргументах `_state.copy(...)` внутри
     * `_state.update { }` в [invite]. Это давало две беды сразу. Первая: бросок
     * из них уходил из `viewModelScope.launch` без обработчика — не «ошибка в
     * диалоге», а падение процесса, причём участник на сервере уже добавлен, а
     * приглашение уже создано. Вторая: `update` — inline-цикл с
     * compareAndSet-ретраем, так что любое конкурентное обновление состояния за
     * те сотни миллисекунд, пока идут запросы, молча повторяло оба GET.
     *
     * Отсюда правило: сеть — до `update`, а `update` остаётся чистой подстановкой
     * уже полученных значений.
     */
    private suspend fun refreshLists() {
        val members = runCatching { repo.members(workspaceId) }
        val invitations = runCatching { repo.invitations(workspaceId) }
        _state.update {
            it.copy(
                busy = false,
                members = members.getOrDefault(it.members),
                invitations = invitations.getOrDefault(it.invitations),
                // Сама запись прошла, упало только перечитывание — говорим об этом,
                // но не выдаём за отказ в добавлении.
                error = (members.exceptionOrNull() ?: invitations.exceptionOrNull())
                    ?.let(::errorMessage) ?: it.error,
            )
        }
    }

    // Ниже — то же правило, что в [refreshLists]: сначала запрос, потом `update`.
    // Здесь бросок ловит launchCatching, так что падения не было, но повтор
    // compareAndSet переспрашивал бы сервер второй раз.
    fun remove(userId: String) = launchCatching {
        repo.removeMember(workspaceId, userId)
        val members = repo.members(workspaceId)
        _state.update { it.copy(members = members) }
    }

    fun changeRole(userId: String, role: String) = launchCatching {
        repo.updateMemberRole(workspaceId, userId, role)
        val members = repo.members(workspaceId)
        _state.update { it.copy(members = members) }
    }

    fun revokeInvite(invId: String) = launchCatching {
        repo.deleteInvitation(workspaceId, invId)
        val invitations = repo.invitations(workspaceId)
        _state.update { it.copy(invitations = invitations) }
    }

    /** Hand the workspace to [userId] (#3072): the owner only — the server checks. */
    fun transferOwner(userId: String) = launchCatching {
        repo.transferOwner(workspaceId, userId)
        val members = repo.members(workspaceId)
        _state.update { it.copy(members = members, canManage = canManageMembers(members, it.meId)) }
    }

    /** Leave the workspace (#3063) — refused to the owner by the server too. */
    fun leave() = launchCatching {
        repo.leave(workspaceId)
        _state.update { it.copy(left = true) }
    }

    /** Let an access request's author in as a member, or decline it (#3077). */
    fun decideAccess(reqId: String, approve: Boolean) = launchCatching {
        if (approve) repo.approveAccess(workspaceId, reqId) else repo.declineAccess(workspaceId, reqId)
        val requests = repo.accessRequests(workspaceId)
        val members = repo.members(workspaceId)
        _state.update { it.copy(accessRequests = requests, members = members) }
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
