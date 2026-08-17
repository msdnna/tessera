package website.msdnna.tessera.data.model

import com.google.gson.JsonElement
import com.google.gson.annotations.SerializedName

/**
 * A workspace document (#2718). Mirrors the backend `Document`
 * (`GET /workspaces/:id/documents`, `GET /documents/:id`).
 *
 * One model for both calls, as on the server: the list query simply omits
 * `content`, so it arrives null and the body is fetched when a document is
 * opened. [content] is the raw ProseMirror tree — see [website.msdnna.tessera.util.parseDocBlocks].
 */
data class Document(
    @SerializedName("id") val id: String = "",
    @SerializedName("workspace_id") val workspaceId: String = "",
    @SerializedName("parent_id") val parentId: String? = null,
    @SerializedName("project_id") val projectId: String? = null,
    @SerializedName("author_id") val authorId: String? = null,
    @SerializedName("title") val title: String = "",
    @SerializedName("slug") val slug: String = "",
    @SerializedName("icon") val icon: String = "",
    @SerializedName("preview") val preview: String = "",
    @SerializedName("position") val position: Double = 0.0,
    @SerializedName("created_at") val createdAt: String = "",
    @SerializedName("updated_at") val updatedAt: String = "",
    @SerializedName("content") val content: JsonElement? = null,
)
