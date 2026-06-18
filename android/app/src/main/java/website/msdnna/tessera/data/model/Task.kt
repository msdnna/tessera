package website.msdnna.tessera.data.model

import com.google.gson.annotations.SerializedName

/**
 * A recurrence rule on a task: repeat every [interval] [freq]-units. The backend
 * also stores a day/month anchor for monthly/yearly rules, but the client only
 * ever sends {freq, interval} and lets the server manage the anchor.
 */
data class Recurrence(
    @SerializedName("freq") val freq: String = "",
    @SerializedName("interval") val interval: Int = 1,
    // Weekly: weekdays to repeat on (0=Sun … 6=Sat). Custom: explicit dates
    // ("yyyy-MM-dd"). Both omitted (null) when not applicable.
    @SerializedName("weekdays") val weekdays: List<Int>? = null,
    @SerializedName("dates") val dates: List<String>? = null,
    @SerializedName("trigger") val trigger: String? = null, // complete|column|schedule
    @SerializedName("trigger_column") val triggerColumn: String? = null,
    @SerializedName("target_column") val targetColumn: String? = null,
    @SerializedName("create_new") val createNew: Boolean = false,
    @SerializedName("once") val once: Boolean = false,
    @SerializedName("skip_weekends") val skipWeekends: Boolean = false,
)

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
    @SerializedName("recurrence") val recurrence: Recurrence? = null,
    // Per-task due-notification overrides (null = inherit the user default).
    @SerializedName("due_lead_minutes") val dueLeadMinutes: Int? = null,
    @SerializedName("due_repeat_minutes") val dueRepeatMinutes: Int? = null,
    @SerializedName("due_notify_enabled") val dueNotifyEnabled: Boolean? = null,
    @SerializedName("tag_ids") val tagIds: List<String> = emptyList(),
    @SerializedName("assignee_ids") val assigneeIds: List<String> = emptyList(),
    // GitLab provenance (present when the card is mirrored from a GitLab issue).
    @SerializedName("gitlab_iid") val gitlabIid: Long? = null,
    @SerializedName("gitlab_url") val gitlabUrl: String? = null,
    @SerializedName("gitlab_author") val gitlabAuthor: String? = null,
    @SerializedName("gitlab_author_name") val gitlabAuthorName: String? = null,
    @SerializedName("gitlab_author_avatar_url") val gitlabAuthorAvatarUrl: String? = null,
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
    @SerializedName("recurrence") val recurrence: Recurrence? = null,
)

/** Per-task due-notification overrides; null fields inherit the user default. */
data class DueNotifyRequest(
    @SerializedName("lead_minutes") val leadMinutes: Int? = null,
    @SerializedName("repeat_minutes") val repeatMinutes: Int? = null,
    @SerializedName("enabled") val enabled: Boolean? = null,
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

/** A project's friendly display name for a tag prefix (mirrors backend `db.TagPrefix`).
 *  `prefix` is the canonical key (trimmed + lowercased) the backend stores. */
data class TagPrefix(
    @SerializedName("project_id") val projectId: String = "",
    @SerializedName("workspace_id") val workspaceId: String = "",
    @SerializedName("prefix") val prefix: String = "",
    @SerializedName("label") val label: String = "",
)

/** PUT body: the complete desired set of prefix display names (replace-all). */
data class SetTagPrefixesRequest(
    @SerializedName("prefixes") val prefixes: List<TagPrefixEntry>,
)

data class TagPrefixEntry(
    @SerializedName("prefix") val prefix: String,
    @SerializedName("label") val label: String,
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
