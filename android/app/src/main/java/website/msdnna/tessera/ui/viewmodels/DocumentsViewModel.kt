package website.msdnna.tessera.ui.viewmodels

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.google.gson.JsonElement
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import website.msdnna.tessera.data.model.Document
import website.msdnna.tessera.data.model.DocumentComment
import website.msdnna.tessera.data.model.DocumentVersion
import website.msdnna.tessera.data.repository.DocumentRepository
import website.msdnna.tessera.ui.UiText
import website.msdnna.tessera.util.DEFAULT_DOC_PAGE
import website.msdnna.tessera.util.DocAnnotateTarget
import website.msdnna.tessera.util.DocBlock
import website.msdnna.tessera.util.DocCrumb
import website.msdnna.tessera.util.DocPage
import website.msdnna.tessera.util.DocThread
import website.msdnna.tessera.util.DocThreadGroups
import website.msdnna.tessera.util.buildDocThreads
import website.msdnna.tessera.util.docBlockIdsInOrder
import website.msdnna.tessera.util.docChildCount
import website.msdnna.tessera.util.docOpenThreadCount
import website.msdnna.tessera.util.docTiles
import website.msdnna.tessera.util.errorMessage
import website.msdnna.tessera.util.parseDocBlocks
import website.msdnna.tessera.util.parseDocPage
import website.msdnna.tessera.util.pruneCrumbs
import website.msdnna.tessera.util.sortDocThreads
import website.msdnna.tessera.util.splitDocThreads

data class DocumentsUiState(
    val loading: Boolean = true,
    val error: UiText? = null,
    /** Every document of the workspace, flat, as the API returns them. */
    val docs: List<Document> = emptyList(),
    /** Containers drilled into; empty is the root level. */
    val trail: List<DocCrumb> = emptyList(),
    /** [docs] filtered to the level [trail] points at — what the grid draws. */
    val tiles: List<Document> = emptyList(),
    /** A create / rename / delete is in flight; the surface stays visible. */
    val busy: Boolean = false,
    val openId: String? = null,
    // The body is a second request, so the reader opens before it arrives.
    val opening: Boolean = false,
    val open: Document? = null,
    val blocks: List<DocBlock> = emptyList(),
    /** Page geometry of the open document — the reader's margins (#2821, #2827). */
    val page: DocPage = DEFAULT_DOC_PAGE,
    /** Children of the open document — the reader's «показать вложенные». */
    val openChildCount: Int = 0,
    /** Annotations on the open document (§5) — the badge and the sheet. */
    val comments: DocCommentsState = DocCommentsState(),
    /** Version journal of the open document (§6) — loaded only while it is up. */
    val history: DocHistoryState = DocHistoryState(),
)

/**
 * Annotation threads for the open document (#2730, §5 of #2894).
 *
 * The list is refetched after every write rather than patched in place, exactly
 * as the web panel does it: the same document is open for several people at
 * once, so a locally spliced list is stale the moment a colleague replies — and
 * this is a handful of rows, not a feed.
 */
data class DocCommentsState(
    /** The sheet is up. Threads are loaded whether it is or not — the reader's
     *  badge has to say how many discussions are waiting before it is opened. */
    val sheetOpen: Boolean = false,
    val loading: Boolean = false,
    /** A write is in flight; the sheet stays readable and the composer waits. */
    val busy: Boolean = false,
    val error: UiText? = null,
    val threads: List<DocThread> = emptyList(),
    val groups: DocThreadGroups = DocThreadGroups(),
    /** The block the sheet was opened on; blank means the whole document. */
    val focusBlockId: String = "",
    /** The block's text as it read at the moment of the tap — stored with a new
     *  thread so a rewritten paragraph still says what was discussed. */
    val quote: String = "",
    /** Block ids in document order, for telling an anchor apart from a deleted one. */
    val blockIds: List<String> = emptyList(),
) {
    val openCount: Int get() = docOpenThreadCount(threads)

    /** Adopts a freshly loaded list: threading, ordering and grouping all hang
     *  off it, and deriving them here keeps the three in step. */
    internal fun withComments(comments: List<DocumentComment>): DocCommentsState {
        val threads = sortDocThreads(buildDocThreads(comments))
        return copy(
            loading = false,
            error = null,
            threads = threads,
            groups = splitDocThreads(threads, blockIds),
        )
    }

    /** Re-groups the threads already loaded against a new set of block ids —
     *  what an edit to the document changes without touching a single comment. */
    internal fun regrouped(blockIds: List<String>): DocCommentsState =
        copy(blockIds = blockIds, groups = splitDocThreads(threads, blockIds))
}

/**
 * The version journal of the open document (#2731, §6 of #2894).
 *
 * Bodies are fetched one version at a time and cached here: the journal lists
 * fifty entries and a document is up to a megabyte of JSON, so the list endpoint
 * carries none of them. The cache is never invalidated by age — a version is
 * immutable once its editing session is over, and the only mutable one (the
 * session being typed into right now) is dropped from the cache with every
 * reload of the list.
 *
 * Unlike the comments state this one is emptied when the panel closes: nothing
 * outside it reads the journal, and holding a megabyte of restored bodies for a
 * panel nobody is looking at buys nothing.
 */
data class DocHistoryState(
    val sheetOpen: Boolean = false,
    val loading: Boolean = false,
    /** A snapshot or a restore is in flight; the list stays readable. */
    val busy: Boolean = false,
    val error: UiText? = null,
    /** Newest first, as the server returns them. */
    val versions: List<DocumentVersion> = emptyList(),
    /** The entry being compared; blank means the list is just being read. */
    val selectedId: String = "",
    val bodies: Map<String, JsonElement?> = emptyMap(),
    /** Bumped by every completed restore. The editor, if one is open, is holding
     *  the pre-rollback text — this is what tells the screen to reload it. */
    val restoreTick: Int = 0,
) {
    val selected: DocumentVersion? get() = versions.firstOrNull { it.id == selectedId }

    /** What the selection is compared against: the newest entry, which is the
     *  question a journal answers most often («что изменилось с тех пор»). */
    val baseline: DocumentVersion? get() = versions.firstOrNull()

    /** Both bodies in hand. Until then the panel shows its loading state rather
     *  than an empty comparison, which would read as «изменений нет». */
    val ready: Boolean
        get() {
            val sel = selected ?: return false
            val base = baseline ?: return false
            return bodies.containsKey(sel.id) && bodies.containsKey(base.id)
        }

    /**
     * Adopts a freshly loaded list.
     *
     * The newest entry is the live editing session and its body goes on changing
     * as people type, so a cached copy of it would show the comparison as it was
     * some minutes ago and call it current. A selection whose entry is no longer
     * in the list (pruned by retention) is dropped rather than left pointing at
     * nothing.
     */
    internal fun withVersions(versions: List<DocumentVersion>): DocHistoryState = copy(
        loading = false,
        error = null,
        versions = versions,
        bodies = versions.firstOrNull()?.let { bodies - it.id } ?: bodies,
        selectedId = if (versions.any { it.id == selectedId }) selectedId else "",
    )
}

/**
 * Documents module (#2718, #2894). A grid of one nesting level plus a
 * breadcrumb trail, mirroring the web view after the review of #2726: a tile
 * always opens the document, and nesting is walked from the reader.
 *
 * Reading the body still belongs to the reader; writing it is the editor's job
 * and arrives with the WebView surface (§4 of #2894). Everything around the
 * body — create, rename, delete, nest — is here.
 */
class DocumentsViewModel(
    private val repo: DocumentRepository = DocumentRepository(),
) : ViewModel() {
    private val _state = MutableStateFlow(DocumentsUiState())
    val state: StateFlow<DocumentsUiState> = _state.asStateFlow()

    private var workspaceId: String = ""

    fun load(workspaceId: String) {
        this.workspaceId = workspaceId
        _state.update { it.copy(loading = true, error = null) }
        launchCatching {
            val docs = repo.list(workspaceId)
            _state.update { it.copy(loading = false).withDocs(docs) }
        }
    }

    /**
     * Opens a document. The tile's own row is shown immediately (title, so the
     * reader is never blank) while the body loads; a failure closes the reader
     * rather than leaving it stuck on a spinner.
     */
    fun open(doc: Document) {
        _state.update {
            it.copy(
                openId = doc.id,
                opening = true,
                open = doc,
                blocks = emptyList(),
                page = DEFAULT_DOC_PAGE,
                error = null,
                openChildCount = docChildCount(it.docs, doc.id),
                comments = DocCommentsState(),
                history = DocHistoryState(),
            )
        }
        viewModelScope.launch {
            val result = runCatching { repo.get(doc.id) }
            result.fold(
                onSuccess = { full ->
                    // Ignore a body that arrives after the reader moved on.
                    if (_state.value.openId != doc.id) return@fold
                    _state.update {
                        it.copy(
                            opening = false,
                            open = full,
                            blocks = parseDocBlocks(full.content),
                            page = parseDocPage(full.content),
                            comments = it.comments.copy(blockIds = docBlockIdsInOrder(full.content)),
                        )
                    }
                    loadComments(doc.id)
                },
                onFailure = { e ->
                    if (_state.value.openId != doc.id) return@fold
                    _state.update {
                        it.copy(opening = false, openId = null, open = null, error = errorMessage(e))
                    }
                },
            )
        }
    }

    /**
     * Re-reads the open document's body. What the editor leaves behind (#2894
     * §4): the reader underneath it is showing the text as it was before the
     * edit, and the editor's own copy is the one that changed.
     */
    fun refreshOpen() {
        val doc = _state.value.open ?: return
        launchCatching {
            val full = repo.get(doc.id)
            _state.update {
                if (it.openId != doc.id) {
                    it
                } else {
                    it.copy(
                        open = full,
                        blocks = parseDocBlocks(full.content),
                        page = parseDocPage(full.content),
                        // A block deleted in the editor turns its thread from
                        // anchored into detached, and the sheet says so.
                        comments = it.comments.regrouped(blockIds = docBlockIdsInOrder(full.content)),
                    )
                }
            }
            loadComments(doc.id)
        }
    }

    fun close() = _state.update {
        it.copy(
            openId = null,
            open = null,
            blocks = emptyList(),
            page = DEFAULT_DOC_PAGE,
            opening = false,
            comments = DocCommentsState(),
            history = DocHistoryState(),
        )
    }

    /** Walks into a container: the grid shows its children, the reader closes. */
    fun drillInto(doc: Document) = _state.update {
        it.copy(openId = null, open = null, blocks = emptyList(), page = DEFAULT_DOC_PAGE, opening = false)
            .withTrail(it.trail + DocCrumb(doc.id, doc.title))
    }

    /** Crumb tap: -1 is the root, otherwise everything after [index] is dropped. */
    fun crumbTo(index: Int) = _state.update {
        it.withTrail(if (index < 0) emptyList() else it.trail.take(index + 1))
    }

    /**
     * Creates a document at the level the grid is showing and opens it, the way
     * the web does. The list is reloaded first so the new tile is there when the
     * reader is closed again.
     */
    fun create(title: String) = mutate { parentId ->
        val doc = repo.create(workspaceId, title.trim(), parentId = parentId)
        refresh()
        open(doc)
    }

    /**
     * Creates a document under the open one and opens *that*. The parent becomes
     * a trail step, so closing the reader lands on the level the new document
     * lives at instead of the one it was created from.
     */
    fun createNested(title: String) = mutate {
        val parent = _state.value.open ?: return@mutate
        val doc = repo.create(workspaceId, title.trim(), parentId = parent.id)
        refresh()
        _state.update { it.withTrail(it.trail + DocCrumb(parent.id, parent.title)) }
        open(doc)
    }

    fun rename(title: String) = mutate {
        val doc = _state.value.open ?: return@mutate
        val renamed = repo.update(doc.id, title = title.trim())
        _state.update { if (it.openId == doc.id) it.copy(open = renamed) else it }
        refresh()
    }

    /**
     * Deletes the open document. A document with children needs `recursive`, and
     * the count is what the confirmation was worded with — so it is read once and
     * used for both, rather than asked about one document and sent about another.
     */
    fun remove() = mutate {
        val doc = _state.value.open ?: return@mutate
        repo.delete(doc.id, recursive = _state.value.openChildCount > 0)
        close()
        // The trail may have been standing on it (or on one of its children).
        refresh()
    }

    // ── Annotations (§5) ──────────────────────────────────────────────────────

    /**
     * Opens the discussion sheet.
     *
     * [target] is the tap that got here: the block, the text it held at that
     * moment, and the document's block ids as the *editor* sees them right now.
     * Called without one (the reader's button, or the editor's bar) the sheet
     * shows everything and a new thread hangs off the document.
     */
    fun openComments(target: DocAnnotateTarget? = null) = _state.update { st ->
        val ids = target?.blockIds?.takeIf { it.isNotEmpty() } ?: st.comments.blockIds
        st.copy(
            comments = st.comments.copy(
                sheetOpen = true,
                focusBlockId = target?.blockId.orEmpty(),
                quote = target?.quote.orEmpty(),
                error = null,
            ).regrouped(ids),
        )
    }

    /** Closes the sheet. The threads stay loaded — the badge behind it counts them. */
    fun closeComments() = _state.update {
        it.copy(comments = it.comments.copy(sheetOpen = false, focusBlockId = "", quote = "", error = null))
    }

    /** Drops the anchor without closing: the same «Открепить» the web panel has,
     *  for writing a remark about the document while standing on a block. */
    fun clearCommentAnchor() = _state.update {
        it.copy(comments = it.comments.copy(focusBlockId = "", quote = ""))
    }

    fun reloadComments() {
        val id = _state.value.openId ?: return
        loadComments(id)
    }

    /** Starts a thread on the focused block, or on the document when none is. */
    fun addComment(body: String) = writeComment { docId ->
        val c = _state.value.comments
        repo.addComment(docId, body.trim(), blockId = c.focusBlockId, quote = c.quote)
    }

    fun replyComment(parentId: String, body: String) = writeComment { docId ->
        // The anchor comes from the root on the server side, so a reply carries
        // no block of its own.
        repo.addComment(docId, body.trim(), parentId = parentId)
    }

    fun editComment(commentId: String, body: String) = writeComment { repo.editComment(commentId, body.trim()) }

    fun resolveComment(commentId: String, resolved: Boolean) =
        writeComment { repo.resolveComment(commentId, resolved) }

    fun deleteComment(commentId: String) = writeComment { repo.deleteComment(commentId) }

    // ── Version journal (§6) ──────────────────────────────────────────────────

    /** Opens the journal and loads it. */
    fun openHistory() {
        val id = _state.value.openId ?: return
        _state.update { it.copy(history = DocHistoryState(sheetOpen = true, loading = true)) }
        loadVersions(id)
    }

    /** Closes it and drops what it held — see [DocHistoryState]. */
    fun closeHistory() = _state.update { it.copy(history = DocHistoryState()) }

    /**
     * Opens a version for comparison, or closes the one already open.
     *
     * Both sides are fetched, not just the selected one: the baseline is the
     * live session, and its body changes as people type.
     */
    fun selectVersion(versionId: String) {
        val next = if (versionId == _state.value.history.selectedId) "" else versionId
        _state.update { it.copy(history = it.history.copy(selectedId = next, error = null)) }
        loadDiffBodies()
    }

    /** Takes a named snapshot of the document as it stands. */
    fun snapshotVersion(label: String) = writeHistory { docId ->
        repo.snapshot(docId, label.trim())
    }

    /**
     * Rolls the document back to a version.
     *
     * The reader under the panel is showing the pre-rollback text, so it is
     * replaced from the document the server returns rather than left to a
     * refetch: the restore already answered with the new state, and asking again
     * would show the old one for as long as the round trip takes.
     */
    fun restoreVersion(versionId: String) = writeHistory { docId ->
        val doc = repo.restoreVersion(versionId)
        _state.update { st ->
            if (st.openId != docId) {
                st
            } else {
                st.copy(
                    open = doc,
                    blocks = parseDocBlocks(doc.content),
                    page = parseDocPage(doc.content),
                    // A rollback brings blocks back and takes others away, which
                    // is precisely what turns a thread anchored into detached.
                    comments = st.comments.regrouped(docBlockIdsInOrder(doc.content)),
                    history = st.history.copy(selectedId = "", restoreTick = st.history.restoreTick + 1),
                )
            }
        }
    }

    private fun loadVersions(docId: String) {
        _state.update { it.copy(history = it.history.copy(loading = true)) }
        viewModelScope.launch {
            val result = runCatching { repo.versions(docId) }
            _state.update { st ->
                if (st.openId != docId || !st.history.sheetOpen) {
                    st
                } else {
                    result.fold(
                        onSuccess = { versions -> st.copy(history = st.history.withVersions(versions)) },
                        onFailure = { e ->
                            st.copy(history = st.history.copy(loading = false, error = errorMessage(e)))
                        },
                    )
                }
            }
            // A snapshot taken while a comparison was open moved the baseline,
            // and reloading the list dropped the live body from the cache — so
            // the open comparison needs its two sides fetched again. Without
            // this the panel sits on «загружаем версию» until the reader
            // happens to tap the entry twice.
            loadDiffBodies()
        }
    }

    /** Fetches whichever of the compared bodies is not cached yet. */
    private fun loadDiffBodies() {
        val docId = _state.value.openId ?: return
        val history = _state.value.history
        if (history.selectedId.isEmpty()) return
        val wanted = listOfNotNull(history.selected?.id, history.baseline?.id)
            .distinct()
            .filterNot { history.bodies.containsKey(it) }
        if (wanted.isEmpty()) return
        viewModelScope.launch {
            val result = runCatching {
                wanted.forEach { id ->
                    val body = repo.version(id).content
                    _state.update { st ->
                        st.copy(history = st.history.copy(bodies = st.history.bodies + (id to body)))
                    }
                }
            }
            result.exceptionOrNull()?.let { e ->
                _state.update { st ->
                    if (st.openId != docId) st else st.copy(history = st.history.copy(error = errorMessage(e)))
                }
            }
        }
    }

    /**
     * Runs one journal write and re-reads the list. Same reasoning as
     * [writeComment]: a snapshot and a rollback both add entries of their own
     * server-side, and the only list that is certainly right is the fresh one.
     */
    private fun writeHistory(block: suspend (docId: String) -> Unit) {
        val docId = _state.value.openId ?: return
        if (_state.value.history.busy) return
        _state.update { it.copy(history = it.history.copy(busy = true, error = null)) }
        viewModelScope.launch {
            val result = runCatching { block(docId) }
            _state.update { it.copy(history = it.history.copy(busy = false)) }
            result.fold(
                onSuccess = { if (_state.value.openId == docId) loadVersions(docId) },
                onFailure = { e ->
                    _state.update { it.copy(history = it.history.copy(error = errorMessage(e))) }
                },
            )
        }
    }

    private fun loadComments(docId: String) {
        _state.update { it.copy(comments = it.comments.copy(loading = true)) }
        viewModelScope.launch {
            val result = runCatching { repo.comments(docId) }
            _state.update { st ->
                // The reader moved on while the list was in flight.
                if (st.openId != docId) {
                    st
                } else {
                    result.fold(
                        onSuccess = { st.copy(comments = st.comments.withComments(it)) },
                        onFailure = { e ->
                            st.copy(comments = st.comments.copy(loading = false, error = errorMessage(e)))
                        },
                    )
                }
            }
        }
    }

    /**
     * Runs one write and re-reads the list, the way the web panel does. The list
     * is not patched from the reply: the document is open elsewhere too, and the
     * only list that is certainly right is the one the server just sent.
     */
    private fun writeComment(block: suspend (docId: String) -> Unit) {
        val docId = _state.value.openId ?: return
        if (_state.value.comments.busy) return
        _state.update { it.copy(comments = it.comments.copy(busy = true, error = null)) }
        viewModelScope.launch {
            val result = runCatching { block(docId) }
            result.fold(
                onSuccess = {
                    _state.update { it.copy(comments = it.comments.copy(busy = false)) }
                    if (_state.value.openId == docId) loadComments(docId)
                },
                onFailure = { e ->
                    _state.update { it.copy(comments = it.comments.copy(busy = false, error = errorMessage(e))) }
                },
            )
        }
    }

    fun clearError() = _state.update { it.copy(error = null) }

    fun reload() = load(workspaceId)

    /** Re-reads the list without the full-screen spinner — the grid stays put. */
    private suspend fun refresh() {
        val docs = repo.list(workspaceId)
        _state.update { it.withDocs(docs) }
    }

    /**
     * Runs a mutating action with the busy flag held and the error surfaced.
     * The block is handed the level the grid is on, since that is what «here»
     * means for a create.
     */
    private fun mutate(block: suspend (parentId: String?) -> Unit) {
        if (_state.value.busy || workspaceId.isBlank()) return
        _state.update { it.copy(busy = true, error = null) }
        viewModelScope.launch {
            val parentId = _state.value.trail.lastOrNull()?.id
            val result = runCatching { block(parentId) }
            _state.update { st ->
                st.copy(busy = false, error = result.exceptionOrNull()?.let { errorMessage(it) } ?: st.error)
            }
        }
    }

    private fun launchCatching(block: suspend () -> Unit) {
        viewModelScope.launch {
            val result = runCatching { block() }
            result.exceptionOrNull()?.let { e ->
                _state.update { it.copy(loading = false, error = errorMessage(e)) }
            }
        }
    }
}

/** Adopts a freshly loaded list, re-deriving everything that hangs off it. */
private fun DocumentsUiState.withDocs(docs: List<Document>): DocumentsUiState =
    copy(docs = docs).withTrail(trail)

/**
 * Moves the grid to [trail] — pruned against the current list, because a step
 * whose document is gone would leave the grid on a level that cannot exist.
 */
private fun DocumentsUiState.withTrail(trail: List<DocCrumb>): DocumentsUiState {
    val kept = pruneCrumbs(docs, trail)
    return copy(
        trail = kept,
        tiles = docTiles(docs, kept.lastOrNull()?.id),
        openChildCount = openId?.let { docChildCount(docs, it) } ?: 0,
    )
}
