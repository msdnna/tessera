package website.msdnna.tessera.data.model

import com.google.gson.annotations.SerializedName

/** `{ tasks: [...], notes: [...], has_more: {...} }` from `GET /workspaces/:id/search?q=`. */
data class SearchResults(
    @SerializedName("tasks") private val tasksRaw: List<SearchTask>? = null,
    @SerializedName("notes") private val notesRaw: List<SearchNote>? = null,
    @SerializedName("has_more") private val hasMoreRaw: SearchHasMore? = null,
) {
    val tasks: List<SearchTask> get() = tasksRaw.orEmpty()
    val notes: List<SearchNote> get() = notesRaw.orEmpty()
    val isEmpty: Boolean get() = tasks.isEmpty() && notes.isEmpty()

    /** Absent on a server older than the flag — which truncates just the same,
     *  so the safe reading of "no answer" is "nothing was cut off" rather than a
     *  warning under every list. */
    val hasMore: SearchHasMore get() = hasMoreRaw ?: SearchHasMore()
}

/** Which kinds the server had to cut short at its page size. */
data class SearchHasMore(
    @SerializedName("tasks") val tasks: Boolean = false,
    @SerializedName("notes") val notes: Boolean = false,
    @SerializedName("documents") val documents: Boolean = false,
)

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
