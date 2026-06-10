package website.msdnna.tessera.data.model

import com.google.gson.annotations.SerializedName

/**
 * A workspace-wide task row with project/board/column context — mirrors the
 * backend `ListWorkspaceTasksRow` (`GET /workspaces/:id/tasks`). Powers the
 * Home / "Моя работа" dashboard list.
 */
data class WorkspaceTask(
    @SerializedName("id") val id: String = "",
    @SerializedName("board_id") val boardId: String = "",
    @SerializedName("column_id") val columnId: String = "",
    @SerializedName("parent_id") val parentId: String? = null,
    @SerializedName("title") val title: String = "",
    @SerializedName("priority") val priority: Int = 0,
    @SerializedName("due_date") val dueDate: String? = null,
    @SerializedName("completed_at") val completedAt: String? = null,
    @SerializedName("number") val number: Long? = null,
    @SerializedName("board_name") val boardName: String = "",
    @SerializedName("project_name") val projectName: String = "",
    @SerializedName("project_color") val projectColor: String = "",
    @SerializedName("column_name") val columnName: String = "",
    @SerializedName("column_color") val columnColor: String = "",
    @SerializedName("tag_ids") val tagIds: List<String> = emptyList(),
    @SerializedName("assignee_ids") val assigneeIds: List<String> = emptyList(),
) {
    val isCompleted: Boolean get() = completedAt != null
}

/** Headline counts for the dashboard cards — mirrors `WorkspaceSummary`. */
data class WorkspaceSummary(
    @SerializedName("total") val total: Int = 0,
    @SerializedName("completed") val completed: Int = 0,
    @SerializedName("active") val active: Int = 0,
    @SerializedName("assigned") val assigned: Int = 0,
    @SerializedName("overdue") val overdue: Int = 0,
    @SerializedName("due_today") val dueToday: Int = 0,
    @SerializedName("due_week") val dueWeek: Int = 0,
    @SerializedName("unassigned") val unassigned: Int = 0,
)
