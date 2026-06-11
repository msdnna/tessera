package website.msdnna.tessera.data.model

import com.google.gson.annotations.SerializedName

/**
 * A board card (top-level task) or subtask, with tag/assignee ids aggregated —
 * mirrors the backend `ListBoardTasksWithMeta` / `ListSubtasksWithMeta` rows.
 */
data class Task(
    @SerializedName("id") val id: String = "",
    @SerializedName("board_id") val boardId: String = "",
    @SerializedName("column_id") val columnId: String = "",
    @SerializedName("parent_id") val parentId: String? = null,
    @SerializedName("title") val title: String = "",
    @SerializedName("description") val description: String = "",
    @SerializedName("priority") val priority: Int = 0,
    @SerializedName("due_date") val dueDate: String? = null,
    @SerializedName("position") val position: Double = 0.0,
    @SerializedName("created_by") val createdBy: String? = null,
    @SerializedName("completed_at") val completedAt: String? = null,
    @SerializedName("archived_at") val archivedAt: String? = null,
    @SerializedName("number") val number: Long? = null,
    @SerializedName("tag_ids") val tagIds: List<String> = emptyList(),
    @SerializedName("assignee_ids") val assigneeIds: List<String> = emptyList(),
    // GitLab provenance (present when the card is mirrored from a GitLab issue).
    @SerializedName("gitlab_iid") val gitlabIid: Long? = null,
    @SerializedName("gitlab_url") val gitlabUrl: String? = null,
    @SerializedName("gitlab_author") val gitlabAuthor: String? = null,
    @SerializedName("gitlab_author_name") val gitlabAuthorName: String? = null,
    // External GitLab assignees (no Tessera account) — display names only.
    @SerializedName("gitlab_assignees") val gitlabAssignees: List<String> = emptyList(),
) {
    val isCompleted: Boolean get() = completedAt != null
}

data class CreateTaskRequest(
    @SerializedName("column_id") val columnId: String,
    @SerializedName("title") val title: String,
    @SerializedName("parent_id") val parentId: String? = null,
    @SerializedName("description") val description: String = "",
    @SerializedName("priority") val priority: Int = 0,
    @SerializedName("due_date") val dueDate: String? = null,
)

data class MoveTaskRequest(
    @SerializedName("column_id") val columnId: String,
    @SerializedName("before_id") val beforeId: String? = null,
    @SerializedName("after_id") val afterId: String? = null,
)

/** Full task update — the backend requires all fields (title is mandatory). */
data class UpdateTaskRequest(
    @SerializedName("title") val title: String,
    @SerializedName("description") val description: String = "",
    @SerializedName("priority") val priority: Int = 0,
    @SerializedName("due_date") val dueDate: String? = null,
    @SerializedName("completed") val completed: Boolean = false,
)

data class SetParentRequest(@SerializedName("parent_id") val parentId: String?)
data class AddTagRequest(@SerializedName("tag_id") val tagId: String)
data class AddAssigneeRequest(@SerializedName("user_id") val userId: String)

/** Move a task (and its subtasks) to another board; column optional (defaults to first). */
data class TransferTaskRequest(
    @SerializedName("board_id") val boardId: String,
    @SerializedName("column_id") val columnId: String? = null,
)

/** Invite an existing user to a workspace by email. role = member|admin. */
data class AddMemberRequest(
    @SerializedName("email") val email: String,
    @SerializedName("role") val role: String = "member",
)

/** A workspace member — mirrors the backend `ListMembersRow`. */
data class Member(
    @SerializedName("user_id") val userId: String = "",
    @SerializedName("role") val role: String = "",
    @SerializedName("email") val email: String = "",
    @SerializedName("name") val name: String = "",
)

data class CreateTagRequest(
    @SerializedName("name") val name: String,
    @SerializedName("color") val color: String,
)

data class UpdateColumnRequest(
    @SerializedName("name") val name: String,
    @SerializedName("color") val color: String = "",
)

data class CreateColumnRequest(@SerializedName("name") val name: String)

data class SetDoneColumnRequest(@SerializedName("column_id") val columnId: String?)

data class ColumnMoveRequest(
    @SerializedName("before_id") val beforeId: String?,
    @SerializedName("after_id") val afterId: String?,
)
