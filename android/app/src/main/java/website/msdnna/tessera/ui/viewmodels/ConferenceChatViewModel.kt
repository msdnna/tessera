package website.msdnna.tessera.ui.viewmodels

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import okhttp3.MultipartBody
import retrofit2.HttpException
import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.model.ConferenceMessage
import website.msdnna.tessera.data.repository.ConferenceRepository
import website.msdnna.tessera.data.repository.WorkspaceRepository
import website.msdnna.tessera.ui.UiText
import website.msdnna.tessera.util.CONF_CHAT_PAGE
import website.msdnna.tessera.util.ChatMember
import website.msdnna.tessera.util.ConfPendingFile
import website.msdnna.tessera.util.ConfPickRefusal
import website.msdnna.tessera.util.chatDefaultBoard
import website.msdnna.tessera.util.chatMentionIds
import website.msdnna.tessera.util.chatTaskCommand
import website.msdnna.tessera.util.confChatAccept
import website.msdnna.tessera.util.confChatCanRemove
import website.msdnna.tessera.util.confChatCanSend
import website.msdnna.tessera.util.confChatMergeTail
import website.msdnna.tessera.util.confChatPrepend
import website.msdnna.tessera.util.confChatReanchor
import website.msdnna.tessera.util.confChatUnread
import website.msdnna.tessera.util.errorMessage

data class ConferenceChatUiState(
    val messages: List<ConferenceMessage> = emptyList(),
    /** There is older text above the top of the list. */
    val hasMore: Boolean = false,
    val loading: Boolean = false,
    val sending: Boolean = false,
    val error: UiText? = null,
    val draft: String = "",
    val pending: List<ConfPendingFile> = emptyList(),
    /** Why the last pick lost a file; cleared on the next one. */
    val refusal: ConfPickRefusal = ConfPickRefusal.NONE,
    val refusedName: String = "",
    /** The sheet is on screen. Held here, not in the composable: closing it must
     *  not throw away the history the reader paged in. */
    val open: Boolean = false,
    val canModerate: Boolean = false,
    val meId: String = "",
    /** The newest line the reader has actually been shown. */
    val lastSeenId: String = "",
    /** The line a confirmation is currently asking about. Removal is visible to
     *  the whole room and cannot be taken back, so it goes through a dialog —
     *  the same treatment the roster gives a kick. */
    val removing: String = "",
    // ── «/» and «@» (#3055) ──────────────────────────────────────────────
    /** The workspace's members: whom «@» offers and whose names lines draw as chips. */
    val members: List<ChatMember> = emptyList(),
    /** The boards «/задача» can put a task on, and the one picked. */
    val boards: List<ChatBoard> = emptyList(),
    val boardId: String? = null,
) {
    val unread: Int get() = if (open) 0 else confChatUnread(messages, lastSeenId, meId)

    /** The title of a «/задача …» draft (blank for the bare command), null otherwise. */
    val taskTitle: String? get() = chatTaskCommand(draft)

    val canSend: Boolean get() = taskTitle?.let { it.isNotBlank() && boardId != null && !sending }
        ?: confChatCanSend(draft, pending, sending)

    fun canRemove(message: ConferenceMessage): Boolean = confChatCanRemove(message, meId, canModerate)
}

/**
 * Turns a file the user picked into a part of a multipart request.
 *
 * An interface rather than a `Context` on the view model: reading a
 * `content://` URI is the one thing here that needs Android, and behind this
 * seam the chat's specs run without one. The implementation streams from the
 * resolver instead of handing over a `ByteArray` — five attachments at the
 * 25 MB cap is 125 MB, and a phone will not give us that.
 */
interface ConfUploadSource {
    fun part(file: ConfPendingFile): MultipartBody.Part?
}

/** A board «/задача» may land on — its name and its project's (#3055). */
data class ChatBoard(val id: String, val name: String, val project: String)

/**
 * The in-call chat (#2896 §7, web `ConferenceChat.vue`).
 *
 * Owns the message list for the life of the call rather than for the life of
 * the sheet. On the web the rail is kept alive behind `v-show` for that reason,
 * with a `visible` prop to work around the scrolling that a hidden element
 * cannot do; here the state simply outlives the composable, and the unread badge
 * falls out of the same list instead of being counted separately.
 */
class ConferenceChatViewModel(
    private val repo: ConferenceRepository = ConferenceRepository(),
    private val workspaces: WorkspaceRepository = WorkspaceRepository(),
) : ViewModel() {
    private val _state = MutableStateFlow(ConferenceChatUiState())
    val state: StateFlow<ConferenceChatUiState> = _state.asStateFlow()

    private var conferenceId: String = ""

    /** The nudge we last answered, so re-entering the composable does not refetch. */
    private var servedNudge: Int = -1

    /** Point the chat at a call. Idempotent: called from a `LaunchedEffect`. */
    fun bind(conferenceId: String, canModerate: Boolean) {
        val fresh = this.conferenceId != conferenceId
        this.conferenceId = conferenceId
        if (fresh) {
            servedNudge = -1
            _state.value = ConferenceChatUiState(canModerate = canModerate)
            viewModelScope.launch {
                val me = runCatching { AppContainer.prefs.user.first()?.id }.getOrNull().orEmpty()
                _state.update { it.copy(meId = me) }
                loadTail(first = true)
            }
        } else {
            // Promotion mid-call is a thing the room socket reports: a host who
            // hands over the room leaves everyone else's delete buttons wrong
            // until the next state frame says so.
            _state.update { it.copy(canModerate = canModerate) }
        }
    }

    /**
     * The room said the chat changed.
     *
     * Answered even while the sheet is closed: the badge has to say how many
     * lines arrived, and the nudge itself carries no payload to count. The
     * fetched tail is also what makes opening the sheet instant.
     */
    fun onNudge(nudge: Int) {
        if (conferenceId.isBlank() || nudge <= 0 || nudge == servedNudge) return
        servedNudge = nudge
        viewModelScope.launch { loadTail() }
    }

    private var workspaceId: String = ""
    private var defaultBoardId: String? = null
    private var boardsLoaded = false

    /**
     * What «@» and «/задача» need (#3055): the call's workspace, its members and
     * the board a task lands on by default (the server's `default_board_id`).
     * Idempotent, like [bind].
     */
    fun setContext(workspaceId: String, members: List<ChatMember>, defaultBoardId: String?) {
        if (this.workspaceId != workspaceId) boardsLoaded = false
        this.workspaceId = workspaceId
        this.defaultBoardId = defaultBoardId
        _state.update { it.copy(members = members) }
    }

    private fun loadBoards() {
        if (boardsLoaded || workspaceId.isBlank()) return
        boardsLoaded = true
        val ws = workspaceId
        viewModelScope.launch {
            val boards = runCatching {
                workspaces.projects(ws).flatMap { p ->
                    runCatching { workspaces.boards(p.id) }.getOrDefault(emptyList())
                        .map { ChatBoard(it.id, it.name, p.name) }
                }
            }.getOrDefault(emptyList())
            if (workspaceId != ws) return@launch
            _state.update { st ->
                st.copy(
                    boards = boards,
                    boardId = st.boardId?.takeIf { id -> boards.any { it.id == id } }
                        ?: chatDefaultBoard(boards.map { it.id }, defaultBoardId),
                )
            }
        }
    }

    fun pickBoard(id: String) = _state.update { it.copy(boardId = id) }

    /** Opening marks everything on screen as read. */
    fun open() = _state.update { it.copy(open = true, lastSeenId = it.messages.lastOrNull()?.id.orEmpty()) }

    fun close() = _state.update { it.copy(open = false, lastSeenId = it.messages.lastOrNull()?.id.orEmpty()) }

    private suspend fun loadTail(first: Boolean = false) {
        val id = conferenceId
        _state.update { it.copy(loading = true) }
        val page = runCatching { repo.messages(id, limit = CONF_CHAT_PAGE) }
        if (conferenceId != id) return
        page.fold(
            onSuccess = { fetched ->
                _state.update { s ->
                    val merge = confChatMergeTail(s.messages, fetched.messages)
                    s.copy(
                        loading = false,
                        messages = merge.messages,
                        // The page speaks only for its own older edge. While the
                        // pages the reader paged in are still above it, what they
                        // said about older text stands; once the merge has fallen
                        // back to the page alone, the page is all we know.
                        hasMore = if (merge.stitched) s.hasMore else fetched.hasMore,
                        error = null,
                        // Joining a call mid-conversation is not a hundred unread
                        // messages: the history was not addressed to us, and a
                        // badge about it would be noise on every call we join.
                        // A closed sheet keeps its anchor — but only as long as
                        // the line it names is still there: a deletion reaches
                        // us as a page without it (see `confChatReanchor`).
                        lastSeenId = if (first || s.open) {
                            merge.messages.lastOrNull()?.id.orEmpty()
                        } else {
                            confChatReanchor(s.messages, merge.messages, s.lastSeenId)
                        },
                    )
                }
            },
            onFailure = { e ->
                // The conference went away under us; the room around this sheet
                // already says so, and a second red line would only repeat it.
                val gone = (e as? HttpException)?.code() in GONE_CODES
                _state.update { it.copy(loading = false, error = if (gone) null else errorMessage(e)) }
            },
        )
    }

    /** «Показать более ранние». */
    fun loadOlder() {
        val s = _state.value
        if (s.loading || !s.hasMore || conferenceId.isBlank()) return
        val oldest = s.messages.firstOrNull() ?: return
        val id = conferenceId
        _state.update { it.copy(loading = true) }
        viewModelScope.launch {
            val page = runCatching { repo.messages(id, oldest.createdAt, oldest.id, CONF_CHAT_PAGE) }
            if (conferenceId != id) return@launch
            page.fold(
                onSuccess = { fetched ->
                    _state.update {
                        it.copy(
                            loading = false,
                            messages = confChatPrepend(fetched.messages, it.messages),
                            hasMore = fetched.hasMore,
                        )
                    }
                },
                onFailure = { e -> _state.update { it.copy(loading = false, error = errorMessage(e)) } },
            )
        }
    }

    // ── composer ──────────────────────────────────────────────────────────

    fun setDraft(text: String) {
        _state.update { it.copy(draft = text) }
        if (chatTaskCommand(text) != null) loadBoards()
    }

    fun attach(picked: List<ConfPendingFile>) = _state.update {
        val result = confChatAccept(it.pending, picked)
        it.copy(pending = result.pending, refusal = result.refusal, refusedName = result.refusedName)
    }

    fun dropPending(uri: String) = _state.update {
        it.copy(pending = it.pending.filterNot { f -> f.uri == uri }, refusal = ConfPickRefusal.NONE)
    }

    fun dismissRefusal() = _state.update { it.copy(refusal = ConfPickRefusal.NONE, refusedName = "") }

    /**
     * Sends the composer.
     *
     * The answer is appended rather than refetched: the server hands back the
     * stored row, which is fresher than anything a second round trip could
     * bring. The composer is cleared only once that row is in hand — clearing on
     * the press and failing afterwards loses the text a person just typed.
     */
    fun send(source: ConfUploadSource) {
        val s = _state.value
        if (!s.canSend || conferenceId.isBlank()) return
        val id = conferenceId
        val body = s.draft.trim()
        val files = s.pending
        val task = s.taskTitle
        val board = s.boardId
        val mentions = chatMentionIds(body, s.members)
        _state.update { it.copy(sending = true, error = null) }
        viewModelScope.launch {
            val sent = runCatching {
                when {
                    // «/задача …» (#3055): the task and its chat line; any queued
                    // files stay for the next message.
                    task != null && board != null -> repo.createTask(id, task, board)

                    files.isEmpty() -> repo.postMessage(id, body, mentions)

                    else -> {
                        val parts = files.mapNotNull { source.part(it) }
                        // Every part failed to open — sending the caption alone would
                        // silently drop the files the message was about.
                        require(parts.isNotEmpty()) { "no readable attachments" }
                        repo.postMessageWithFiles(id, body, parts, mentions)
                    }
                }
            }
            if (conferenceId != id) return@launch
            sent.fold(
                onSuccess = { message ->
                    _state.update { st ->
                        val list = if (st.messages.any { it.id == message.id }) st.messages else st.messages + message
                        st.copy(
                            sending = false,
                            draft = "",
                            pending = if (task != null) st.pending else emptyList(),
                            refusal = ConfPickRefusal.NONE,
                            messages = list,
                            lastSeenId = list.lastOrNull()?.id.orEmpty(),
                        )
                    }
                },
                onFailure = { e -> _state.update { it.copy(sending = false, error = errorMessage(e)) } },
            )
        }
    }

    fun askRemove(messageId: String) = _state.update { it.copy(removing = messageId) }

    fun cancelRemove() = _state.update { it.copy(removing = "") }

    /**
     * Removes the line the dialog is asking about, on the server and then here.
     *
     * Not optimistically: the check this mirrors lives on the server, and a row
     * that came back from a refused delete would reappear on the next nudge
     * anyway — after the reader had been told it was gone.
     */
    fun confirmRemove() {
        val messageId = _state.value.removing
        _state.update { it.copy(removing = "") }
        if (messageId.isBlank()) return
        viewModelScope.launch {
            runCatching { repo.deleteMessage(messageId) }.fold(
                onSuccess = {
                    _state.update { s ->
                        s.copy(messages = s.messages.filterNot { it.id == messageId })
                    }
                },
                onFailure = { e -> _state.update { it.copy(error = errorMessage(e)) } },
            )
        }
    }

    /**
     * Fetches an attachment into the cache and hands the file to the screen,
     * which opens it through our `FileProvider`.
     *
     * The failure is reported on the chat's own error line rather than swallowed:
     * a download that produces no file and no sentence reads as a dead button,
     * and the usual reason for one here is that the call took the network with it.
     */
    fun download(cacheDir: java.io.File, attachmentId: String, filename: String, onReady: (java.io.File) -> Unit) {
        if (attachmentId.isBlank()) return
        viewModelScope.launch {
            runCatching { repo.downloadAttachment(cacheDir, attachmentId, filename) }.fold(
                onSuccess = onReady,
                onFailure = { e -> _state.update { it.copy(error = errorMessage(e)) } },
            )
        }
    }

    fun clearError() = _state.update { it.copy(error = null) }

    private companion object {
        /** The conference is gone or no longer ours — not something to shout about. */
        val GONE_CODES = setOf(403, 404)
    }
}
