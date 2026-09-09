package website.msdnna.tessera.ui.viewmodels

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import retrofit2.HttpException
import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.model.Conference
import website.msdnna.tessera.data.model.ConferenceParticipant
import website.msdnna.tessera.data.model.ConferenceRecording
import website.msdnna.tessera.data.model.Member
import website.msdnna.tessera.data.realtime.ConfEvent
import website.msdnna.tessera.data.realtime.RealtimeClient
import website.msdnna.tessera.data.realtime.RealtimeEvent
import website.msdnna.tessera.data.realtime.classifyConfEvent
import website.msdnna.tessera.data.repository.ConferenceRepository
import website.msdnna.tessera.data.repository.WorkspaceRepository
import website.msdnna.tessera.ui.UiText
import website.msdnna.tessera.util.canInviteToConference
import website.msdnna.tessera.util.canModerateConference
import website.msdnna.tessera.util.confInvitable
import website.msdnna.tessera.util.confInvited
import website.msdnna.tessera.util.confRecordingDownloadable
import website.msdnna.tessera.util.confRecordingFileName
import website.msdnna.tessera.util.confRecordingOrder
import website.msdnna.tessera.util.confRoster
import website.msdnna.tessera.util.errorMessage
import website.msdnna.tessera.util.isInConfRoom

data class ConferenceLobbyUiState(
    val loading: Boolean = true,
    val error: UiText? = null,
    /** Deleted, or opened from another workspace — say so instead of showing nothing. */
    val notFound: Boolean = false,
    val conference: Conference? = null,
    val participants: List<ConferenceParticipant> = emptyList(),
    /** The workspace roster, for the invite picker. Empty until it loads (or if it fails). */
    val members: List<Member> = emptyList(),
    val meId: String = "",
    /** My role in the workspace; blank until the roster loads. */
    val workspaceRole: String = "",
    /** A join/leave/end is in flight — the pair of buttons is one control. */
    val busy: Boolean = false,
    val confirmingEnd: Boolean = false,
    /**
     * This conference's recordings (#2896 §8), failed rows included: a recording
     * that never arrived is the one people ask about.
     */
    val recordings: List<ConferenceRecording> = emptyList(),
    /**
     * A download or delete that failed, printed under the list rather than
     * toasted — this panel is below the fold, and a toast about a list nobody is
     * looking at is noise.
     */
    val recordingsError: UiText? = null,
    /** The row with a call in flight; blank when none is. */
    val recordingBusyId: String = "",
    /** Which recording a delete confirmation is about; blank when none is. */
    val confirmingDeleteId: String = "",
) {
    val inRoom: Boolean get() = isInConfRoom(participants, meId)

    val canModerate: Boolean
        get() = conference?.let { canModerateConference(it, participants, meId, workspaceRole) } == true

    val canInvite: Boolean get() = conference?.let { canInviteToConference(it) } == true

    val roster: List<ConferenceParticipant> get() = confRoster(participants)

    val invited: List<ConferenceParticipant> get() = confInvited(participants)

    val invitable: List<Member> get() = if (canInvite) confInvitable(members, participants) else emptyList()

    /** Newest first, with a running recording pinned to the top. */
    val recordingRows: List<ConferenceRecording> get() = confRecordingOrder(recordings)

    /** The row a delete confirmation is about, or null once it has gone. */
    val deleteTarget: ConferenceRecording?
        get() = recordings.firstOrNull { it.id == confirmingDeleteId }
}

/**
 * One conference — the lobby (#2896 §3, web `ConferencesView`'s detail half).
 *
 * Owns the roster and the membership actions. Media is §4/§5: this screen is
 * about who is in the call and who is being called, which is server state and
 * survives on its own.
 *
 * Membership is decided *here*, not by the room: join/leave answer the refreshed
 * conference because the first arrival flips a scheduled call to live and the
 * last exit pauses it back (#2879), and a screen that kept its own copy of the
 * status would keep offering «Войти» to a room it is already in.
 */
class ConferenceLobbyViewModel(
    private val repo: ConferenceRepository = ConferenceRepository(),
    private val workspaces: WorkspaceRepository = WorkspaceRepository(),
) : ViewModel() {
    private val _state = MutableStateFlow(ConferenceLobbyUiState())
    val state: StateFlow<ConferenceLobbyUiState> = _state.asStateFlow()

    private var conferenceId: String = ""
    private var realtime: RealtimeClient? = null
    private var reloadJob: Job? = null

    fun load(conferenceId: String) {
        if (conferenceId.isBlank()) return
        this.conferenceId = conferenceId
        _state.update {
            // Everything but the id is dropped: opening a second conference while
            // the first is on screen must not show the previous call's roster
            // under the new title for as long as the fetch takes.
            ConferenceLobbyUiState(loading = true, meId = it.meId)
        }
        viewModelScope.launch {
            if (_state.value.meId.isBlank()) {
                val me = runCatching { AppContainer.prefs.user.first()?.id }.getOrNull().orEmpty()
                _state.update { it.copy(meId = me) }
            }
            val result = runCatching { repo.get(conferenceId) }
            result.fold(
                onSuccess = { detail ->
                    if (this@ConferenceLobbyViewModel.conferenceId != conferenceId) return@fold
                    _state.update {
                        it.copy(
                            loading = false,
                            conference = detail.conference,
                            participants = detail.participants,
                        )
                    }
                    ensureRealtime(detail.conference.workspaceId)
                    loadRecordings()
                    loadMembers(detail.conference.workspaceId)
                },
                onFailure = { e ->
                    if (this@ConferenceLobbyViewModel.conferenceId != conferenceId) return@fold
                    val gone = (e as? HttpException)?.code() in GONE_CODES
                    _state.update {
                        it.copy(
                            loading = false,
                            notFound = gone,
                            error = if (gone) null else errorMessage(e),
                        )
                    }
                },
            )
        }
    }

    /**
     * The workspace roster: both halves of the invite control and my own role
     * come from it. Silent on failure — the picker then lists nobody, which is
     * not something to put a red line under a lobby that reads perfectly well.
     */
    private suspend fun loadMembers(workspaceId: String) {
        if (workspaceId.isBlank()) return
        val members = runCatching { workspaces.members(workspaceId) }.getOrNull() ?: return
        _state.update { s ->
            s.copy(
                members = members,
                workspaceRole = members.firstOrNull { it.userId == s.meId }?.role.orEmpty(),
            )
        }
    }

    // ── membership ────────────────────────────────────────────────────────

    fun join() = act { repo.join(conferenceId).conference }

    fun leave() = act { repo.leave(conferenceId).conference }

    fun askEnd() = _state.update { it.copy(confirmingEnd = true) }

    fun cancelEnd() = _state.update { it.copy(confirmingEnd = false) }

    fun confirmEnd() {
        _state.update { it.copy(confirmingEnd = false) }
        act { repo.end(conferenceId) }
    }

    /**
     * Runs a membership call, then re-reads the roster from the server rather
     * than patching in the one row that came back: joining also changes who else
     * the server now considers present (the pause that ends a call drops
     * everyone), and a locally patched list would disagree with it.
     */
    private fun act(call: suspend () -> Conference) {
        if (conferenceId.isBlank() || _state.value.busy) return
        _state.update { it.copy(busy = true, error = null) }
        viewModelScope.launch {
            val result = runCatching {
                val conference = call()
                conference to repo.participants(conferenceId)
            }
            result.fold(
                onSuccess = { (conference, participants) ->
                    _state.update { it.copy(busy = false, conference = conference, participants = participants) }
                },
                onFailure = { e -> _state.update { it.copy(busy = false, error = errorMessage(e)) } },
            )
        }
    }

    // ── invite ────────────────────────────────────────────────────────────

    /**
     * One tap invites one person (the picker has no batch select, as on the web).
     * The refetch is what removes them from the picker and adds them to
     * «Приглашены» — the two lists are both derived from the roster, so there is
     * one place for them to disagree and this keeps it empty.
     */
    fun invite(userId: String) {
        if (conferenceId.isBlank() || userId.isBlank()) return
        viewModelScope.launch {
            val result = runCatching {
                repo.invite(conferenceId, listOf(userId))
                repo.participants(conferenceId)
            }
            result.fold(
                onSuccess = { participants -> _state.update { it.copy(participants = participants) } },
                onFailure = { e -> _state.update { it.copy(error = errorMessage(e)) } },
            )
        }
    }

    fun clearError() = _state.update { it.copy(error = null) }

    // ── recordings (#2896 §8) ─────────────────────────────────────────────

    /**
     * Everyone who may enter the call may see this list — the recording *is* the
     * meeting, and whoever could have attended can watch it back. Only a
     * moderator may delete a row, which mirrors exactly who may start one.
     *
     * Silent on failure. A server without the egress worker has no recordings
     * endpoint worth a red line under a lobby that reads perfectly well; the
     * honest refusal belongs on the button that tried to start one (§8, room).
     */
    private suspend fun loadRecordings() {
        val id = conferenceId
        val rows = runCatching { repo.recordings(id) }.getOrNull() ?: return
        if (conferenceId != id) return
        _state.update { it.copy(recordings = rows) }
    }

    /**
     * Fetches an mp4 into the cache and hands the file to the screen, which opens
     * it through our `FileProvider` — the same path the chat's attachments take,
     * and for the same reason: a bare `GET` at the API carries no credential.
     */
    fun downloadRecording(cacheDir: java.io.File, recording: ConferenceRecording, onReady: (java.io.File) -> Unit) {
        if (!confRecordingDownloadable(recording)) return
        _state.update { it.copy(recordingBusyId = recording.id, recordingsError = null) }
        viewModelScope.launch {
            val result = runCatching {
                repo.downloadRecordingTo(cacheDir, recording.id, confRecordingFileName(recording))
            }
            _state.update { it.copy(recordingBusyId = "") }
            result.fold(
                onSuccess = onReady,
                onFailure = { e -> _state.update { it.copy(recordingsError = errorMessage(e)) } },
            )
        }
    }

    fun askDeleteRecording(id: String) = _state.update { it.copy(confirmingDeleteId = id) }

    fun cancelDeleteRecording() = _state.update { it.copy(confirmingDeleteId = "") }

    /** Behind a confirmation: the file is erased server-side and nothing brings
     *  it back — unlike a message, which at least existed in somebody's scroll. */
    fun confirmDeleteRecording() {
        val id = _state.value.confirmingDeleteId
        _state.update { it.copy(confirmingDeleteId = "") }
        if (id.isBlank()) return
        _state.update { it.copy(recordingBusyId = id, recordingsError = null) }
        viewModelScope.launch {
            val result = runCatching { repo.deleteRecording(id) }
            _state.update {
                it.copy(
                    recordingBusyId = "",
                    // Dropped from the list on success rather than refetched: the
                    // row is gone either way, and a refetch that raced the delete
                    // would put it back for a second.
                    recordings = if (result.isSuccess) it.recordings.filterNot { r -> r.id == id } else it.recordings,
                    recordingsError = result.exceptionOrNull()?.let(::errorMessage),
                )
            }
        }
    }

    fun clearRecordingsError() = _state.update { it.copy(recordingsError = null) }

    // ── realtime ──────────────────────────────────────────────────────────

    private fun ensureRealtime(workspaceId: String) {
        if (realtime != null || workspaceId.isBlank()) return
        realtime = RealtimeClient { ev -> onRealtimeEvent(ev, workspaceId) }.also { it.connect() }
    }

    /**
     * A colleague joining or ending the call has to land here without a refresh —
     * the roster is the whole content of this screen.
     *
     * A recording starting or finishing refetches the list and nothing else
     * (§8): the row is written by an egress worker this client never talks to,
     * so the only way a finished file appears under the lobby is the server
     * saying so. The roster and the conference are untouched by it — a recording
     * does not change who is in the call.
     *
     * A roster event refetches the participants only, while anything else
     * conference-shaped refetches the conference too — «Идёт сейчас» and the end
     * button both hang off its status.
     */
    private fun onRealtimeEvent(ev: RealtimeEvent, workspaceId: String) {
        when (classifyConfEvent(ev.type, ev.scope, workspaceId)) {
            ConfEvent.PARTICIPANTS -> scheduleReload(withConference = false)
            ConfEvent.LIST -> scheduleReload(withConference = true)
            ConfEvent.RECORDING -> viewModelScope.launch { loadRecordings() }
            ConfEvent.OTHER -> Unit
        }
    }

    /** Debounced: ending a call emits an end plus a leave per participant. */
    private fun scheduleReload(withConference: Boolean) {
        reloadJob?.cancel()
        reloadJob = viewModelScope.launch {
            delay(REALTIME_DEBOUNCE_MS)
            // Silent: a dropped frame is not worth a red line under a lobby whose
            // roster is at most one event out of date.
            runCatching {
                val participants = repo.participants(conferenceId)
                val conference = if (withConference) repo.get(conferenceId).conference else null
                _state.update {
                    it.copy(participants = participants, conference = conference ?: it.conference)
                }
            }
        }
    }

    override fun onCleared() {
        realtime?.close()
        realtime = null
    }

    private companion object {
        const val REALTIME_DEBOUNCE_MS = 250L

        /** A deleted conference answers 404; one in another workspace, 403. */
        val GONE_CODES = setOf(403, 404)
    }
}
