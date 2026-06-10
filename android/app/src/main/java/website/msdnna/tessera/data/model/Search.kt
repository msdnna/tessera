package website.msdnna.tessera.data.model

import com.google.gson.annotations.SerializedName

/** `{ tasks: [...], notes: [...] }` from `GET /workspaces/:id/search?q=`. */
data class SearchResults(
    @SerializedName("tasks") private val tasksRaw: List<SearchTask>? = null,
    @SerializedName("notes") private val notesRaw: List<SearchNote>? = null,
) {
    val tasks: List<SearchTask> get() = tasksRaw.orEmpty()
    val notes: List<SearchNote> get() = notesRaw.orEmpty()
    val isEmpty: Boolean get() = tasks.isEmpty() && notes.isEmpty()
}

/** A task search hit — mirrors `SearchTasksRow`. */
data class SearchTask(
    @SerializedName("id") val id: String = "",
    @SerializedName("board_id") val boardId: String = "",
    @SerializedName("number") val number: Long? = null,
    @SerializedName("title") val title: String = "",
    @SerializedName("parent_id") val parentId: String? = null,
    @SerializedName("completed_at") val completedAt: String? = null,
) {
    val isCompleted: Boolean get() = completedAt != null
}

/** A note search hit — mirrors `SearchNotesRow`. */
data class SearchNote(
    @SerializedName("id") val id: String = "",
    @SerializedName("title") val title: String = "",
)
