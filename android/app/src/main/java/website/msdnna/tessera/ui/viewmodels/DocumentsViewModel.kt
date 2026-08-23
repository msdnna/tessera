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
import website.msdnna.tessera.util.DocTreeRow
import website.msdnna.tessera.util.documentTree
import website.msdnna.tessera.util.errorMessage
import website.msdnna.tessera.util.parseDocBlocks

data class DocumentsUiState(
    val loading: Boolean = true,
    val error: UiText? = null,
    val rows: List<DocTreeRow> = emptyList(),
    val openId: String? = null,
    // The body is a second request, so the reader opens before it arrives.
    val opening: Boolean = false,
    val open: Document? = null,
    val blocks: List<DocBlock> = emptyList(),
)

/**
 * Documents module, read-only (#2735): a tree of the workspace's documents and
 * a reader for one of them. Editing stays on the web client — see #2718.
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
            val rows = documentTree(repo.list(workspaceId))
            _state.update { it.copy(loading = false, rows = rows) }
        }
    }

    /**
     * Opens a document. The tile's own row is shown immediately (title, so the
     * reader is never blank) while the body loads; a failure closes the reader
     * rather than leaving it stuck on a spinner.
     */
    fun open(doc: Document) {
        _state.update { it.copy(openId = doc.id, opening = true, open = doc, blocks = emptyList(), error = null) }
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

    fun clearError() = _state.update { it.copy(error = null) }

    fun reload() = load(workspaceId)

    private fun launchCatching(block: suspend () -> Unit) {
        viewModelScope.launch {
            val result = runCatching { block() }
            result.exceptionOrNull()?.let { e ->
                _state.update { it.copy(loading = false, error = errorMessage(e)) }
            }
        }
    }
}
