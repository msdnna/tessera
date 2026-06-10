package website.msdnna.tessera.data.repository

import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.model.Note

/** CRUD over workspace notes (two-pane NotesView). */
class NoteRepository {
    private val api get() = AppContainer.api()

    suspend fun list(workspaceId: String): List<Note> = api.notes(workspaceId).orEmpty()
    suspend fun get(id: String): Note = api.note(id)
    suspend fun create(workspaceId: String, title: String, body: String): Note =
        api.createNote(workspaceId, website.msdnna.tessera.data.model.CreateNoteRequest(title = title, body = body))
    suspend fun update(id: String, title: String, body: String): Note =
        api.updateNote(id, website.msdnna.tessera.data.model.UpdateNoteRequest(title = title, body = body))
    suspend fun delete(id: String) = api.deleteNote(id)
}
