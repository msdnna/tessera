package website.msdnna.tessera.ui.viewmodels

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.google.gson.JsonElement
import com.google.gson.JsonObject
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import retrofit2.HttpException
import website.msdnna.tessera.R
import website.msdnna.tessera.data.model.Document
import website.msdnna.tessera.data.model.DocumentApproval
import website.msdnna.tessera.data.model.DocumentComment
import website.msdnna.tessera.data.model.DocumentConverterStatus
import website.msdnna.tessera.data.model.DocumentImportResult
import website.msdnna.tessera.data.model.DocumentTaskLink
import website.msdnna.tessera.data.model.DocumentTemplate
import website.msdnna.tessera.data.model.DocumentVersion
import website.msdnna.tessera.data.model.Member
import website.msdnna.tessera.data.model.WorkspaceTask
import website.msdnna.tessera.data.repository.DocContentSave
import website.msdnna.tessera.data.repository.DocumentRepository
import website.msdnna.tessera.data.repository.TaskRepository
import website.msdnna.tessera.data.repository.WorkspaceRepository
import website.msdnna.tessera.ui.UiText
import website.msdnna.tessera.util.DEFAULT_DOC_PAGE
import website.msdnna.tessera.util.DocAnnotateTarget
import website.msdnna.tessera.util.DocBlock
import website.msdnna.tessera.util.DocCrumb
import website.msdnna.tessera.util.DocFileFormatException
import website.msdnna.tessera.util.DocImportRoute
import website.msdnna.tessera.util.DocPage
import website.msdnna.tessera.util.DocThread
import website.msdnna.tessera.util.DocThreadGroups
import website.msdnna.tessera.util.buildDocThreads
import website.msdnna.tessera.util.canRaiseApproval
import website.msdnna.tessera.util.docBlockIdsInOrder
import website.msdnna.tessera.util.docChildCount
import website.msdnna.tessera.util.docExportFormats
import website.msdnna.tessera.util.docImportLimit
import website.msdnna.tessera.util.docImportRoute
import website.msdnna.tessera.util.docOpenThreadCount
import website.msdnna.tessera.util.docTiles
import website.msdnna.tessera.util.documentApprovalState
import website.msdnna.tessera.util.errorMessage
import website.msdnna.tessera.util.parseDocBlocks
import website.msdnna.tessera.util.parseDocFile
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
    /** Task links and approval routes of the open document (§7). */
    val links: DocLinksState = DocLinksState(),
    /** The template gallery, file import and export (§8). */
    val templates: DocTemplatesState = DocTemplatesState(),
    /** A converted file waiting for the editor to parse it — see [DocPendingImport]. */
    val pendingImport: DocPendingImport? = null,
)

/**
 * A server-side import that has been uploaded but not yet turned into text
 * (§8 of #2894).
 *
 * The document exists at this point — the endpoint created it — but its body is
 * HTML (or a stored PDF), and nothing in the app can parse that into blocks.
 * The web editor can, so the payload rides along until the editor opens on that
 * document and the bridge hands it over. If the handoff never happens (the user
 * backs out) the document survives as an empty one, which is the same outcome
 * the web accepts and for the same reason: deleting something the user just
 * watched appear is worse than an empty page they can fill.
 */
data class DocPendingImport(
    val documentId: String,
    /** The `{html, page, pdf}` payload, already serialized for the bridge. */
    val payload: String,
)

/**
 * The template gallery, file import and export (#2733, #2734 — §8 of #2894).
 *
 * The saved templates are fetched when the gallery opens rather than with the
 * list: a workspace's templates are a rare need, and the section is opened to
 * read documents. The converter status is fetched once and kept — it decides
 * whether the office half of the picker is offered at all, and a section that
 * asked again on every import would be answering the same question all day.
 */
data class DocTemplatesState(
    val sheetOpen: Boolean = false,
    val loading: Boolean = false,
    /** Card id whose «Создать» is in flight — the gallery disables itself so an
     *  impatient second tap does not create a second document. */
    val busy: String = "",
    val error: UiText? = null,
    val saved: List<DocumentTemplate> = emptyList(),
    /** Null until the status has been asked for; absent is not "unavailable". */
    val converter: DocumentConverterStatus? = null,
    val importing: Boolean = false,
    /** Format being exported, empty when nothing is. */
    val exporting: String = "",
    /** Something the user has to be told about a finished import — pictures the
     *  converter dropped, or a file whose sections disagreed about geometry.
     *  Said out loud because it is the only way to know the document is a
     *  reduction of the file rather than a copy of it. */
    val notice: UiText? = null,
) {
    /** The office formats are offered only when the sidecar answered. */
    val converterAvailable: Boolean get() = converter?.available == true

    /** Why it is not, in the server's own words — it is the side that knows. */
    val converterReason: String get() = converter?.reason.orEmpty()

    /** Export formats, `html` always among them (see [docExportFormats]). */
    val exportFormats: List<String> get() = docExportFormats(converter)
}

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
 * Task links and approval routes of the open document (#2732, §7 of #2894).
 *
 * The two live in one state because they arrive on one panel and are read as one
 * question: a route is raised against the document, and the tasks it came from
 * are the reason it was. Both lists are refetched whole after every write rather
 * than patched from the response — a link is a row joined to a task's title and
 * a route is joined to its steps, and rebuilding either from a mutation's answer
 * means keeping a second copy of the server's join here, which is exactly the
 * thing that goes stale without saying so.
 *
 * The pickers ([tasks], [members]) are fetched once, lazily, and kept: composing
 * a route is a rare act, and a workspace's task list is not something to pull on
 * every document opened.
 */
data class DocLinksState(
    val sheetOpen: Boolean = false,
    val loading: Boolean = false,
    /** A write is in flight; the panel stays readable. */
    val busy: Boolean = false,
    val error: UiText? = null,
    val links: List<DocumentTaskLink> = emptyList(),
    /** Newest first, as the server returns them. */
    val approvals: List<DocumentApproval> = emptyList(),
    /** The block a new link would be pinned to, and the text it held at the tap. */
    val anchorBlockId: String = "",
    val anchorQuote: String = "",
    /** Candidates for the pickers — empty until the panel needs them. */
    val tasks: List<WorkspaceTask> = emptyList(),
    val members: List<Member> = emptyList(),
) {
    /** One open route per document. */
    val canRaise: Boolean get() = canRaiseApproval(approvals)

    /** The route the document is judged by — see [documentApprovalState]. */
    val current: DocumentApproval? get() = documentApprovalState(approvals)

    /** What the button over the document counts: links, since that is the number
     *  that means something before the panel is open. A route's status is a word,
     *  not a count, and belongs inside. */
    val linkCount: Int get() = links.size

    /** Already-linked tasks are dropped from the picker rather than offered and
     *  refused: re-linking the same pair is a no-op server-side, so offering it
     *  would be a button that appears to do nothing. */
    fun candidates(query: String): List<WorkspaceTask> {
        val linked = links.mapTo(mutableSetOf()) { it.taskId }
        val q = query.trim().lowercase()
        return tasks.asSequence()
            .filter { it.number != null && it.id !in linked }
            .filter { q.isEmpty() || "#${it.number}".contains(q) || it.title.lowercase().contains(q) }
            .take(MAX_LINK_CANDIDATES)
            .toList()
    }
}

/** As many task rows as the picker offers at once — the same cap the web uses. */
private const val MAX_LINK_CANDIDATES = 50

/** What the backend answers with when the export sidecar is not there (§8). */
private const val HTTP_UNAVAILABLE = 503

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
    // The two pickers of the links panel (§7) ask for things that are not
    // documents: the workspace's tasks and its members.
    private val taskRepo: TaskRepository = TaskRepository(),
    private val wsRepo: WorkspaceRepository = WorkspaceRepository(),
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
                links = DocLinksState(tasks = it.links.tasks, members = it.links.members),
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
                    // Loaded with the body, not with the panel: the button over
                    // the document carries the number of linked tasks, and a
                    // count that only appears once the panel has been opened is
                    // a count nobody sees.
                    loadLinks(doc.id)
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
            // The pickers survive the close: they describe the workspace, not
            // this document, and refetching a whole task list per document
            // opened is a request nobody asked for.
            links = DocLinksState(tasks = it.links.tasks, members = it.links.members),
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

    // ── Task links and approvals (§7) ─────────────────────────────────────────

    /**
     * Opens the links panel.
     *
     * [target] is the tap that got here, exactly as for the discussions: a new
     * link is pinned to the block that was tapped, with the text it held at that
     * moment. Opened from the bar instead, the link is filed against the document
     * as a whole.
     */
    fun openLinks(target: DocAnnotateTarget? = null) {
        val id = _state.value.openId ?: return
        _state.update {
            it.copy(
                links = it.links.copy(
                    sheetOpen = true,
                    loading = true,
                    error = null,
                    anchorBlockId = target?.blockId.orEmpty(),
                    anchorQuote = target?.quote.orEmpty(),
                ),
            )
        }
        loadLinks(id)
        ensureLinkPickers()
    }

    /** Closes the panel. The lists stay loaded — the badge behind it counts them. */
    fun closeLinks() = _state.update {
        it.copy(links = it.links.copy(sheetOpen = false, anchorBlockId = "", anchorQuote = "", error = null))
    }

    /** Drops the anchor without closing — the same «Открепить» the discussions
     *  have, for filing a link against the document while standing on a block. */
    fun clearLinkAnchor() = _state.update {
        it.copy(links = it.links.copy(anchorBlockId = "", anchorQuote = ""))
    }

    fun linkTask(taskId: String) = writeLinks { docId ->
        val l = _state.value.links
        repo.linkTask(docId, taskId, blockId = l.anchorBlockId, quote = l.anchorQuote)
    }

    fun unlinkTask(linkId: String) = writeLinks { repo.unlinkTask(linkId) }

    fun raiseApproval(title: String, mode: String, approvers: List<String>) = writeLinks { docId ->
        if (approvers.isEmpty()) return@writeLinks
        repo.createApproval(docId, approvers = approvers, title = title.trim(), mode = mode)
    }

    /**
     * Records the caller's signature. Whose turn it is stays the server's call;
     * the panel only avoids offering the button when it already knows it is not
     * theirs — see [canDecideNow].
     */
    fun decideApproval(approvalId: String, decision: String, comment: String = "") = writeLinks {
        repo.decideApproval(approvalId, decision, comment.trim())
    }

    fun cancelApproval(approvalId: String) = writeLinks { repo.cancelApproval(approvalId) }

    /**
     * Loads the tasks and members the pickers offer, once.
     *
     * Failures are swallowed on purpose, as on the web: the panel still lists
     * what is linked and what is being agreed — it just cannot compose anything
     * new. An error banner here would fire on a list only needed by the two
     * buttons at the bottom.
     */
    private fun ensureLinkPickers() {
        if (workspaceId.isBlank()) return
        val l = _state.value.links
        if (l.tasks.isEmpty()) {
            viewModelScope.launch {
                runCatching { taskRepo.workspaceTasks(workspaceId) }.getOrNull()?.let { tasks ->
                    _state.update { it.copy(links = it.links.copy(tasks = tasks)) }
                }
            }
        }
        if (l.members.isEmpty()) {
            viewModelScope.launch {
                runCatching { wsRepo.members(workspaceId) }.getOrNull()?.let { members ->
                    _state.update { it.copy(links = it.links.copy(members = members)) }
                }
            }
        }
    }

    private fun loadLinks(docId: String) {
        _state.update { it.copy(links = it.links.copy(loading = true)) }
        viewModelScope.launch {
            // One await for both, not two: the panel draws links and routes
            // together, and sequencing them would show it half-built for a round
            // trip.
            val result = runCatching {
                val links = repo.taskLinks(docId)
                val approvals = repo.approvals(docId)
                links to approvals
            }
            _state.update { st ->
                if (st.openId != docId) {
                    st
                } else {
                    result.fold(
                        onSuccess = { (links, approvals) ->
                            st.copy(
                                links = st.links.copy(
                                    loading = false,
                                    error = null,
                                    links = links,
                                    approvals = approvals,
                                ),
                            )
                        },
                        onFailure = { e ->
                            st.copy(links = st.links.copy(loading = false, error = errorMessage(e)))
                        },
                    )
                }
            }
        }
    }

    /** Runs one write and re-reads both lists — same reasoning as [writeComment]. */
    private fun writeLinks(block: suspend (docId: String) -> Unit) {
        val docId = _state.value.openId ?: return
        if (_state.value.links.busy) return
        _state.update { it.copy(links = it.links.copy(busy = true, error = null)) }
        viewModelScope.launch {
            val result = runCatching { block(docId) }
            _state.update { it.copy(links = it.links.copy(busy = false)) }
            result.fold(
                onSuccess = { if (_state.value.openId == docId) loadLinks(docId) },
                onFailure = { e ->
                    _state.update { it.copy(links = it.links.copy(error = errorMessage(e))) }
                },
            )
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

    // ── Templates, import and export (§8) ─────────────────────────────────────

    /**
     * Asks once whether the office sidecar is deployed.
     *
     * A failure here is answered with «нет конвертера» rather than surfaced: it
     * decides which half of one picker is offered, and an error line over the
     * document list because an optional service is down would be noise.
     */
    fun loadConverter() {
        if (_state.value.templates.converter != null) return
        viewModelScope.launch {
            val status = runCatching { repo.converterStatus() }
                .getOrDefault(DocumentConverterStatus(available = false))
            _state.update { it.copy(templates = it.templates.copy(converter = status)) }
        }
    }

    fun openTemplates() {
        _state.update { it.copy(templates = it.templates.copy(sheetOpen = true, error = null)) }
        if (workspaceId.isBlank()) return
        _state.update { it.copy(templates = it.templates.copy(loading = true)) }
        viewModelScope.launch {
            val result = runCatching { repo.templates(workspaceId) }
            _state.update { st ->
                result.fold(
                    onSuccess = { saved ->
                        st.copy(templates = st.templates.copy(loading = false, saved = saved))
                    },
                    onFailure = { e ->
                        st.copy(templates = st.templates.copy(loading = false, error = errorMessage(e)))
                    },
                )
            }
        }
    }

    fun closeTemplates() = _state.update {
        it.copy(templates = it.templates.copy(sheetOpen = false, error = null, busy = ""))
    }

    fun clearTemplatesNotice() = _state.update {
        it.copy(templates = it.templates.copy(notice = null))
    }

    /**
     * Creates a document from a gallery card and opens it.
     *
     * The two kinds of card are one call on purpose. A saved template is applied
     * *server-side* ([templateId]) — the body lives there and copying it through
     * the phone would be a round trip for nothing. A built-in has no server side
     * at all: its text is a string resource, so the caller hands the parsed body
     * in and it is written through the ordinary content endpoint, which is the
     * same path typing takes.
     */
    fun useTemplate(cardId: String, title: String, templateId: String?, content: JsonElement?) {
        if (_state.value.templates.busy.isNotEmpty() || workspaceId.isBlank()) return
        _state.update { it.copy(templates = it.templates.copy(busy = cardId, error = null)) }
        val parentId = _state.value.trail.lastOrNull()?.id
        viewModelScope.launch {
            val result = runCatching {
                val doc = repo.create(
                    workspaceId = workspaceId,
                    title = title,
                    parentId = parentId,
                    templateId = templateId,
                )
                if (content == null) doc else applyBody(doc, content)
            }
            _state.update { it.copy(templates = it.templates.copy(busy = "")) }
            result.fold(
                onSuccess = { doc ->
                    _state.update { it.copy(templates = it.templates.copy(sheetOpen = false)) }
                    refreshThenOpen(doc)
                },
                onFailure = { e ->
                    _state.update { it.copy(templates = it.templates.copy(error = errorMessage(e))) }
                },
            )
        }
    }

    /** Deletes a saved template. Documents made from it are untouched — the body
     *  was copied at creation, not referenced. */
    fun deleteTemplate(templateId: String) {
        if (_state.value.templates.busy.isNotEmpty()) return
        _state.update { it.copy(templates = it.templates.copy(busy = templateId, error = null)) }
        viewModelScope.launch {
            val result = runCatching {
                repo.deleteTemplate(templateId)
                repo.templates(workspaceId)
            }
            _state.update { st ->
                result.fold(
                    onSuccess = { saved -> st.copy(templates = st.templates.copy(busy = "", saved = saved)) },
                    onFailure = { e ->
                        st.copy(templates = st.templates.copy(busy = "", error = errorMessage(e)))
                    },
                )
            }
        }
    }

    /**
     * Turns a picked file into a document (#2733, #2734).
     *
     * Which road it takes is decided by the file name alone, because that is all
     * the picker gives us before the bytes are read — see [docImportRoute]:
     *
     *  - `.md` / `.json` are parsed here and saved like any body, so they work on
     *    an install with no converter deployed;
     *  - office formats are uploaded, converted server-side, and the HTML that
     *    comes back is left for the editor to parse (see [DocPendingImport]);
     *  - a PDF travels the same endpoint but is stored rather than converted,
     *    which is why it is not gated on the converter either.
     *
     * [fallbackTitle] names a file whose contents suggest nothing.
     */
    fun importFile(fileName: String, bytes: ByteArray, mime: String?, fallbackTitle: String) {
        if (_state.value.templates.importing || workspaceId.isBlank()) return
        val route = docImportRoute(fileName)
        if (route == DocImportRoute.UNSUPPORTED) {
            return failImport(UiText.Res(R.string.docs_import_unsupported))
        }
        if (bytes.size > docImportLimit(route)) {
            return failImport(
                UiText.Res(R.string.docs_import_too_large, listOf(docImportLimit(route) / (1024 * 1024))),
            )
        }
        if (route == DocImportRoute.OFFICE && !_state.value.templates.converterAvailable) {
            val reason = _state.value.templates.converterReason
            return failImport(
                if (reason.isNotBlank()) UiText.Raw(reason) else UiText.Res(R.string.docs_import_no_converter),
            )
        }
        _state.update { it.copy(templates = it.templates.copy(importing = true, error = null)) }
        val parentId = _state.value.trail.lastOrNull()?.id
        viewModelScope.launch {
            val result = runCatching {
                if (route == DocImportRoute.LOCAL) {
                    importLocalFile(fileName, bytes, fallbackTitle, parentId)
                } else {
                    importThroughServer(fileName, bytes, mime)
                }
            }
            _state.update { it.copy(templates = it.templates.copy(importing = false)) }
            result.fold(
                onSuccess = { doc ->
                    _state.update { it.copy(templates = it.templates.copy(sheetOpen = false)) }
                    refreshThenOpen(doc)
                },
                onFailure = { e ->
                    val text = if (e is DocFileFormatException) {
                        UiText.Res(R.string.docs_import_not_a_document)
                    } else {
                        errorMessage(e)
                    }
                    _state.update { it.copy(templates = it.templates.copy(error = text)) }
                },
            )
        }
    }

    /** The editor took the converted body (or the user left without opening it). */
    fun clearPendingImport() = _state.update { it.copy(pendingImport = null) }

    /**
     * Exports the open document and hands the file to [onFile], which shares it.
     *
     * Sharing rather than saving is the phone's shape of this: there is no
     * folder an app may write to unasked, and a file the user has to go looking
     * for is a file they do not find.
     */
    fun exportOpen(cacheDir: java.io.File, format: String, fileName: String, onFile: (java.io.File) -> Unit) {
        val docId = _state.value.openId ?: return
        if (_state.value.templates.exporting.isNotEmpty()) return
        _state.update { it.copy(templates = it.templates.copy(exporting = format, error = null)) }
        viewModelScope.launch {
            val result = runCatching { repo.exportToFile(cacheDir, docId, format, fileName) }
            _state.update { it.copy(templates = it.templates.copy(exporting = "")) }
            result.fold(
                onSuccess = onFile,
                onFailure = { e ->
                    // The failure the reader can act on is «конвертер не отвечает»;
                    // everything else is the ordinary network message.
                    val text = if (e is HttpException && e.code() == HTTP_UNAVAILABLE) {
                        UiText.Res(R.string.docs_export_unavailable)
                    } else {
                        errorMessage(e)
                    }
                    _state.update { it.copy(error = text) }
                },
            )
        }
    }

    private fun failImport(text: UiText) {
        _state.update { it.copy(templates = it.templates.copy(error = text)) }
    }

    /** `.md` / `.json`: parsed on the phone, then created and saved like typing. */
    private suspend fun importLocalFile(
        fileName: String,
        bytes: ByteArray,
        fallbackTitle: String,
        parentId: String?,
    ): Document {
        val draft = parseDocFile(fileName, bytes.toString(Charsets.UTF_8))
        val doc = repo.create(
            workspaceId = workspaceId,
            title = draft.title.ifBlank { fallbackTitle },
            icon = draft.icon,
            parentId = parentId,
        )
        return applyBody(doc, draft.content)
    }

    /**
     * Office and PDF: the server creates the document, the editor fills it.
     *
     * The payload is parked in the state rather than applied here — see
     * [DocPendingImport] for why the phone does not parse the HTML itself.
     */
    private suspend fun importThroughServer(fileName: String, bytes: ByteArray, mime: String?): Document {
        val result = repo.import(workspaceId, bytes, fileName, mime)
        val payload = JsonObject().apply {
            addProperty("html", result.html)
            result.page?.let { add("page", it) }
            result.pdf?.let { add("pdf", it) }
        }
        _state.update {
            it.copy(
                pendingImport = DocPendingImport(result.document.id, payload.toString()),
                templates = it.templates.copy(notice = importNotice(result)),
            )
        }
        return result.document
    }

    /**
     * What the reader has to be told about a finished conversion.
     *
     * Both cases mean the document is a faithful import of the *text* and a
     * reduction of the file, and the only way to know that is to be told (#2755,
     * #2821). The reason travels with the count because a bare number says
     * something was lost without saying whether re-inserting the figure as PNG
     * would help.
     */
    private fun importNotice(result: DocumentImportResult): UiText? = when {
        result.imagesDropped > 0 && result.imagesDroppedReason.isNotBlank() -> UiText.Res(
            R.string.docs_import_images_dropped_why,
            listOf(result.imagesDropped, result.imagesDroppedReason),
        )

        result.imagesDropped > 0 -> UiText.Res(
            R.string.docs_import_images_dropped,
            listOf(result.imagesDropped),
        )

        result.sectionsDiffer -> UiText.Res(R.string.docs_import_sections_differ)

        else -> null
    }

    /** Writes a freshly created document's body, keeping the version stamp in
     *  step so the first edit after it does not answer with a conflict. */
    private suspend fun applyBody(doc: Document, content: JsonElement): Document =
        when (val saved = repo.saveContent(doc.id, content, doc.updatedAt)) {
            is DocContentSave.Saved -> doc.copy(updatedAt = saved.result.updatedAt)

            // Nobody else can be editing a document created a moment ago; if the
            // stamp is somehow stale the document still exists with its body
            // unwritten, and saying so beats pretending it worked.
            is DocContentSave.Conflict -> throw java.io.IOException("document body write conflicted")
        }

    /** Re-reads the list so the new tile exists, then opens it. */
    private fun refreshThenOpen(doc: Document) {
        launchCatching {
            refresh()
            open(doc)
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
