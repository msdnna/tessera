package website.msdnna.tessera.ui.viewmodels

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import website.msdnna.tessera.data.model.Conference
import website.msdnna.tessera.data.realtime.ConfEvent
import website.msdnna.tessera.data.realtime.RealtimeClient
import website.msdnna.tessera.data.realtime.RealtimeEvent
import website.msdnna.tessera.data.realtime.classifyConfEvent
import website.msdnna.tessera.data.repository.ConferenceRepository
import website.msdnna.tessera.ui.UiText
import website.msdnna.tessera.util.errorMessage

data class ConferencesUiState(
    val loading: Boolean = true,
    val error: UiText? = null,
    val list: List<Conference> = emptyList(),
    /** `null` is the «Все» tab; otherwise a `ConferenceStatus` value. */
    val filter: String? = null,
    // ── the schedule dialog ───────────────────────────────────────────────
    val composing: Boolean = false,
    val saving: Boolean = false,
    val createError: UiText? = null,
    /** The row whose delete confirmation is open, if any. */
    val pendingDelete: Conference? = null,
    /** The conference whose lobby (§3) is open over the list, if any. */
    val openId: String? = null,
)

/**
 * The conferences list (#2896 §2, web `ConferencesView`'s list half).
 *
 * Holds the workspace's calls, the filter tab and the schedule dialog. Opening
 * one is §3; this screen deliberately stops at "which calls exist".
 */
class ConferencesViewModel(
    private val repo: ConferenceRepository = ConferenceRepository(),
) : ViewModel() {
    private val _state = MutableStateFlow(ConferencesUiState())
    val state: StateFlow<ConferencesUiState> = _state.asStateFlow()

    private var workspaceId: String = ""
    private var realtime: RealtimeClient? = null
    private var reloadJob: Job? = null

    fun load(workspaceId: String) {
        this.workspaceId = workspaceId
        _state.update { it.copy(loading = true, error = null) }
        ensureRealtime()
        launchCatching {
            val list = repo.list(workspaceId, _state.value.filter)
            _state.update { it.copy(loading = false, list = list) }
        }
    }

    /** [status] is null for «Все». The server does the filtering, as on the web —
     *  the counters a row shows are computed by the list query, not derivable here. */
    fun setFilter(status: String?) {
        if (_state.value.filter == status) return
        _state.update { it.copy(filter = status) }
        if (workspaceId.isNotBlank()) load(workspaceId)
    }

    // ── the lobby (§3) ────────────────────────────────────────────────────

    fun open(conference: Conference) = _state.update { it.copy(openId = conference.id) }

    fun closeLobby() = _state.update { it.copy(openId = null) }

    // ── schedule dialog ───────────────────────────────────────────────────

    fun compose() = _state.update { it.copy(composing = true, createError = null) }

    fun cancelCompose() = _state.update { it.copy(composing = false, createError = null) }

    /**
     * Creates a conference and closes the dialog. [scheduledAtIso] is null for
     * "start it whenever"; [ttlDays] has already been validated by the form.
     *
     * A failure keeps the dialog open with its text intact — a rejected title
     * (empty after trimming, or too long) is worth another try, and a dialog that
     * closed on error would throw the description away with it.
     */
    fun create(title: String, description: String, scheduledAtIso: String?, ttlDays: Int) {
        if (workspaceId.isBlank() || _state.value.saving) return
        _state.update { it.copy(saving = true, createError = null) }
        viewModelScope.launch {
            val result = runCatching {
                repo.create(
                    workspaceId = workspaceId,
                    title = title,
                    description = description,
                    scheduledAt = scheduledAtIso,
                    recordingTtlDays = ttlDays,
                )
            }
            result.fold(
                onSuccess = { created ->
                    // Straight into the new call's lobby, as on the web: scheduling
                    // one is how you get somewhere to invite people, and a list
                    // that just grew a row leaves that step to be found.
                    _state.update { it.copy(saving = false, composing = false, openId = created.id) }
                    refetch()
                },
                onFailure = { e -> _state.update { it.copy(saving = false, createError = errorMessage(e)) } },
            )
        }
    }

    // ── delete ────────────────────────────────────────────────────────────

    fun askDelete(conference: Conference) = _state.update { it.copy(pendingDelete = conference) }

    fun cancelDelete() = _state.update { it.copy(pendingDelete = null) }

    fun confirmDelete() {
        val target = _state.value.pendingDelete ?: return
        // Close the lobby if it is the one being deleted — it would otherwise sit
        // over the list showing a call that no longer exists.
        _state.update { it.copy(pendingDelete = null, openId = it.openId?.takeIf { id -> id != target.id }) }
        launchCatching {
            repo.delete(target.id)
            // Drop the row first, then refetch: the counters of the *other* rows
            // are the server's to compute, and a list left standing until the
            // refetch lands still offers a call that is already gone.
            _state.update { s -> s.copy(list = s.list.filterNot { it.id == target.id }) }
            refetch()
        }
    }

    /**
     * Re-reads the list without raising [ConferencesUiState.loading] — a refresh
     * that follows an action the user just took must not blank the screen they
     * are looking at into a spinner.
     */
    private suspend fun refetch() {
        if (workspaceId.isBlank()) return
        val list = repo.list(workspaceId, _state.value.filter)
        _state.update { it.copy(list = list) }
    }

    fun clearError() = _state.update { it.copy(error = null) }

    // ── realtime ──────────────────────────────────────────────────────────

    private fun ensureRealtime() {
        if (realtime != null) return
        realtime = RealtimeClient(::onRealtimeEvent).also { it.connect() }
    }

    /**
     * A colleague starting a call has to appear without a pull-to-refresh —
     * «идёт сейчас» is the whole reason to open this screen.
     *
     * Roster events reload the list too, unlike in the room (§6): the rows carry
     * «в комнате: N», so a join here is a change to the list even though the
     * conference row itself is untouched. Recording events are not: the egress
     * worker reports in repeatedly during a call, and none of it is visible on
     * this screen.
     */
    private fun onRealtimeEvent(ev: RealtimeEvent) {
        when (classifyConfEvent(ev.type, ev.scope, workspaceId)) {
            ConfEvent.LIST, ConfEvent.PARTICIPANTS -> scheduleReload()
            ConfEvent.RECORDING, ConfEvent.OTHER -> Unit
        }
    }

    /** Debounced: ending a call emits an end plus a leave per participant. */
    private fun scheduleReload() {
        reloadJob?.cancel()
        reloadJob = viewModelScope.launch {
            delay(REALTIME_DEBOUNCE_MS)
            // Silent on failure: a dropped socket frame is not something to put a
            // red line under a list that is still perfectly readable.
            runCatching { refetch() }
        }
    }

    override fun onCleared() {
        realtime?.close()
        realtime = null
    }

    private fun launchCatching(block: suspend () -> Unit) {
        viewModelScope.launch {
            val result = runCatching { block() }
            result.exceptionOrNull()?.let { e ->
                _state.update { it.copy(loading = false, error = errorMessage(e)) }
            }
        }
    }

    private companion object {
        const val REALTIME_DEBOUNCE_MS = 250L
    }
}
