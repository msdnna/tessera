package website.msdnna.tessera.ui.viewmodels

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import website.msdnna.tessera.data.model.Document
import website.msdnna.tessera.data.repository.DocumentRepository
import website.msdnna.tessera.ui.UiText
import website.msdnna.tessera.util.DocBlock
import website.msdnna.tessera.util.DocCrumb
import website.msdnna.tessera.util.docChildCount
import website.msdnna.tessera.util.docTiles
import website.msdnna.tessera.util.errorMessage
import website.msdnna.tessera.util.parseDocBlocks
import website.msdnna.tessera.util.pruneCrumbs

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
    /** Children of the open document — the reader's «показать вложенные». */
    val openChildCount: Int = 0,
)

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
                error = null,
                openChildCount = docChildCount(it.docs, doc.id),
            )
        }
        viewModelScope.launch {
            val result = runCatching { repo.get(doc.id) }
            result.fold(
                onSuccess = { full ->
                    // Ignore a body that arrives after the reader moved on.
                    if (_state.value.openId != doc.id) return@fold
                    _state.update {
                        it.copy(opening = false, open = full, blocks = parseDocBlocks(full.content))
                    }
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

    fun close() = _state.update { it.copy(openId = null, open = null, blocks = emptyList(), opening = false) }

    /** Walks into a container: the grid shows its children, the reader closes. */
    fun drillInto(doc: Document) = _state.update {
        it.copy(openId = null, open = null, blocks = emptyList(), opening = false)
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
