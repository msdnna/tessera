package website.msdnna.tessera.data.model

import com.google.gson.annotations.SerializedName

/**
 * A workspace note — mirrors the backend `Note` (`GET /workspaces/:id/notes`,
 * `GET /notes/:id`). Body is plain Markdown text (the web NotesView uses a
 * plain textarea, not the rich editor).
 */
data class Note(
    @SerializedName("id") val id: String = "",
    @SerializedName("workspace_id") val workspaceId: String = "",
    @SerializedName("project_id") val projectId: String? = null,
    @SerializedName("author_id") val authorId: String? = null,
    @SerializedName("title") val title: String = "",
    @SerializedName("body") val body: String = "",
    @SerializedName("created_at") val createdAt: String = "",
    @SerializedName("updated_at") val updatedAt: String = "",
    // «Входящие» tags (names) and mentioned member ids (#3040). Nullable: Gson
    // leaves a field the server did not send as null, not as the default. The
    // editor's update sends neither, so the server keeps them.
    @SerializedName("tags") val tags: List<String>? = null,
    @SerializedName("mentions") val mentions: List<String>? = null,
)

data class CreateNoteRequest(
    @SerializedName("title") val title: String,
    @SerializedName("body") val body: String = "",
    @SerializedName("project_id") val projectId: String? = null,
)

data class UpdateNoteRequest(
    @SerializedName("title") val title: String,
    @SerializedName("body") val body: String = "",
)
