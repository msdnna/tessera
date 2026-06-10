package website.msdnna.tessera.data.model

import com.google.gson.annotations.SerializedName

/**
 * Full task detail — mirrors the backend `taskDetail` returned by
 * `GET /tasks/:id`: the task itself plus its tags, assignees and direct
 * subtasks (with meta, for the subtasks tab).
 */
data class TaskDetail(
    @SerializedName("id") val id: String = "",
    @SerializedName("board_id") val boardId: String = "",
    @SerializedName("column_id") val columnId: String = "",
    @SerializedName("parent_id") val parentId: String? = null,
    @SerializedName("title") val title: String = "",
    @SerializedName("description") val description: String = "",
    @SerializedName("priority") val priority: Int = 0,
    @SerializedName("due_date") val dueDate: String? = null,
    @SerializedName("created_by") val createdBy: String? = null,
    @SerializedName("completed_at") val completedAt: String? = null,
    @SerializedName("number") val number: Long? = null,
    // Nullable: the backend serialises empty slices as JSON `null`, and Gson
    // ignores Kotlin defaults — so these arrive null when the task has none.
    // Always read them through the non-null accessors below.
    @SerializedName("tags") private val tagsRaw: List<Tag>? = null,
    @SerializedName("assignees") private val assigneesRaw: List<AssigneeUser>? = null,
    @SerializedName("subtasks") private val subtasksRaw: List<Task>? = null,
) {
    val isCompleted: Boolean get() = completedAt != null
    val tags: List<Tag> get() = tagsRaw.orEmpty()
    val assignees: List<AssigneeUser> get() = assigneesRaw.orEmpty()
    val subtasks: List<Task> get() = subtasksRaw.orEmpty()

    /** A lightweight [Task] view of this detail, for reusing card-level pickers. */
    fun asTask(): Task = Task(
        id = id, boardId = boardId, columnId = columnId, parentId = parentId,
        title = title, description = description, priority = priority, dueDate = dueDate,
        createdBy = createdBy, completedAt = completedAt, number = number,
        tagIds = tags.map { it.id }, assigneeIds = assignees.map { it.id },
    )
}

/** A task assignee — mirrors `ListTaskAssigneesRow`. */
data class AssigneeUser(
    @SerializedName("id") val id: String = "",
    @SerializedName("email") val email: String = "",
    @SerializedName("name") val name: String = "",
)

/** A task comment — mirrors `ListTaskCommentsRow`. */
data class Comment(
    @SerializedName("id") val id: String = "",
    @SerializedName("task_id") val taskId: String = "",
    @SerializedName("author_id") val authorId: String? = null,
    @SerializedName("body") val body: String = "",
    @SerializedName("created_at") val createdAt: String = "",
    @SerializedName("updated_at") val updatedAt: String = "",
    @SerializedName("author_name") val authorName: String? = null,
    @SerializedName("author_email") val authorEmail: String? = null,
)

/** A task relation (referenced by #N) — mirrors `ListTaskRelationsRow`. */
data class Relation(
    @SerializedName("id") val id: String = "",
    @SerializedName("task_id") val taskId: String = "",
    @SerializedName("related_task_id") val relatedTaskId: String = "",
    @SerializedName("kind") val kind: String = "relates",
    @SerializedName("related_number") val relatedNumber: Long? = null,
    @SerializedName("related_title") val relatedTitle: String = "",
    @SerializedName("related_board_id") val relatedBoardId: String = "",
    @SerializedName("related_completed_at") val relatedCompletedAt: String? = null,
    @SerializedName("related_archived_at") val relatedArchivedAt: String? = null,
)

/** A task attachment — mirrors `ListTaskAttachmentsRow`. */
data class Attachment(
    @SerializedName("id") val id: String = "",
    @SerializedName("task_id") val taskId: String = "",
    @SerializedName("uploader_id") val uploaderId: String? = null,
    @SerializedName("filename") val filename: String = "",
    @SerializedName("content_type") val contentType: String = "",
    @SerializedName("size") val size: Long = 0,
    @SerializedName("created_at") val createdAt: String = "",
    @SerializedName("uploader_name") val uploaderName: String? = null,
)

/** A task journal entry — mirrors the backend `taskEventResp`. `data` is opaque. */
data class TaskEvent(
    @SerializedName("id") val id: String = "",
    @SerializedName("kind") val kind: String = "",
    @SerializedName("created_at") val createdAt: String = "",
    @SerializedName("actor_name") val actorName: String? = null,
)

/** `{ "url": "/api/uploads/<uuid>.<ext>" }` from `POST /api/uploads`. */
data class UploadResponse(@SerializedName("url") val url: String = "")

// ── Request bodies ──────────────────────────────────────────────────────────

data class CreateCommentRequest(
    @SerializedName("body") val body: String,
    @SerializedName("mentions") val mentions: List<String> = emptyList(),
)

data class UpdateCommentRequest(@SerializedName("body") val body: String)

data class AddRelationRequest(
    @SerializedName("number") val number: Long,
    @SerializedName("kind") val kind: String = "relates",
)
