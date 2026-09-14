package website.msdnna.tessera.data.model

import com.google.gson.annotations.SerializedName

/** `{ tasks: [...], notes: [...], documents: [...], has_more: {...} }` from
 *  `GET /workspaces/:id/search?q=`. */
data class SearchResults(
    @SerializedName("tasks") private val tasksRaw: List<SearchTask>? = null,
    @SerializedName("notes") private val notesRaw: List<SearchNote>? = null,
    @SerializedName("documents") private val documentsRaw: List<SearchDocument>? = null,
    @SerializedName("has_more") private val hasMoreRaw: SearchHasMore? = null,
) {
    val tasks: List<SearchTask> get() = tasksRaw.orEmpty()
    val notes: List<SearchNote> get() = notesRaw.orEmpty()
    val documents: List<SearchDocument> get() = documentsRaw.orEmpty()

    /** Пусто — значит пусто по ВСЕМ трём видам, а не по двум из них: сервер ищет
     *  и по заголовкам документов (`SearchDocuments`), и запрос, которому ответили
     *  только документы, читался как «Ничего не найдено» поверх непустого ответа. */
    val isEmpty: Boolean get() = tasks.isEmpty() && notes.isEmpty() && documents.isEmpty()

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

/** A document search hit — mirrors `SearchDocumentsRow`. The slug is what the web
 *  router navigates by; the phone opens documents by id, so it is kept only to
 *  stay shaped like the payload. */
data class SearchDocument(
    @SerializedName("id") val id: String = "",
    @SerializedName("title") val title: String = "",
    @SerializedName("slug") val slug: String = "",
)
