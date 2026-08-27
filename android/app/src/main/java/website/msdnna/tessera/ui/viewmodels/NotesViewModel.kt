package website.msdnna.tessera.ui.viewmodels

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import website.msdnna.tessera.data.model.Note
import website.msdnna.tessera.data.repository.NoteRepository
import website.msdnna.tessera.ui.UiText
import website.msdnna.tessera.util.errorMessage

data class NotesUiState(
    val loading: Boolean = true,
    val error: UiText? = null,
    val list: List<Note> = emptyList(),
    val selectedId: String? = null,
    // True when the right pane is a brand-new (unsaved) note.
    val composingNew: Boolean = false,
) {
    val selected: Note? get() = list.find { it.id == selectedId }
}

/** Two-pane notes (web `NotesView`): a list + an editor, saving on demand. */
class NotesViewModel(
    private val repo: NoteRepository = NoteRepository(),
) : ViewModel() {
    private val _state = MutableStateFlow(NotesUiState())
    val state: StateFlow<NotesUiState> = _state.asStateFlow()

    private var workspaceId: String = ""

    fun load(workspaceId: String, preselectId: String? = null) {
        this.workspaceId = workspaceId
        _state.update { it.copy(loading = true, error = null, selectedId = null, composingNew = false) }
        launchCatching {
            val list = repo.list(workspaceId)
            val select = preselectId?.takeIf { id -> list.any { it.id == id } }
            _state.update { it.copy(loading = false, list = list, selectedId = select) }
        }
    }

    fun select(note: Note) = _state.update { it.copy(selectedId = note.id, composingNew = false) }

    fun newNote() = _state.update { it.copy(selectedId = null, composingNew = true) }

    fun closeEditor() = _state.update { it.copy(selectedId = null, composingNew = false) }

    /** Saves the open note: updates the selected one, else creates a new note. */
    fun save(title: String, body: String) = launchCatching {
        val current = _state.value.selected
        val saved = if (current != null) {
            repo.update(current.id, title, body)
        } else {
            repo.create(workspaceId, title, body)
        }
        val list = repo.list(workspaceId)
        _state.update { it.copy(list = list, selectedId = saved.id, composingNew = false) }
    }

    fun delete(id: String) = launchCatching {
        repo.delete(id)
        val list = repo.list(workspaceId)
        _state.update { it.copy(list = list, selectedId = null, composingNew = false) }
    }

    fun clearError() = _state.update { it.copy(error = null) }

    private fun launchCatching(block: suspend () -> Unit) {
        viewModelScope.launch {
            val result = runCatching { block() }
            result.exceptionOrNull()?.let { e ->
                _state.update { it.copy(loading = false, error = errorMessage(e)) }
            }
        }
    }
}
