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
import website.msdnna.tessera.data.model.SearchResults
import website.msdnna.tessera.data.repository.SearchRepository
import website.msdnna.tessera.ui.UiText
import website.msdnna.tessera.util.errorMessage

data class SearchUiState(
    val query: String = "",
    val loading: Boolean = false,
    val error: UiText? = null,
    val results: SearchResults? = null,
)

/**
 * Debounced workspace search (web `SearchBar`): 220 ms debounce + a sequence
 * guard so only the latest in-flight query's results land. Blank queries clear
 * the results without a request.
 */
class SearchViewModel(
    private val repo: SearchRepository = SearchRepository(),
) : ViewModel() {
    private val _state = MutableStateFlow(SearchUiState())
    val state: StateFlow<SearchUiState> = _state.asStateFlow()

    private var workspaceId: String = ""
    private var debounce: Job? = null
    private var seq = 0

    fun bind(workspaceId: String) {
        if (this.workspaceId != workspaceId) {
            this.workspaceId = workspaceId
            reset()
        }
    }

    fun onQueryChange(query: String) {
        _state.update { it.copy(query = query) }
        debounce?.cancel()
        val trimmed = query.trim()
        if (trimmed.isEmpty() || workspaceId.isBlank()) {
            _state.update { it.copy(loading = false, results = null, error = null) }
            return
        }
        val mine = ++seq
        _state.update { it.copy(loading = true) }
        debounce = viewModelScope.launch {
            delay(DEBOUNCE_MS)
            val result = runCatching { repo.search(workspaceId, trimmed) }
            if (mine != seq) return@launch // a newer query superseded this one
            result
                .onSuccess { res -> _state.update { it.copy(loading = false, results = res, error = null) } }
                .onFailure { e -> _state.update { it.copy(loading = false, error = errorMessage(e)) } }
        }
    }

    fun reset() {
        debounce?.cancel()
        seq++
        _state.update { SearchUiState() }
    }

    private companion object {
        const val DEBOUNCE_MS = 220L
    }
}
