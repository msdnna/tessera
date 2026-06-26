package website.msdnna.tessera.data.model

import com.google.gson.annotations.SerializedName

/**
 * Two-level task-estimation config (workspace default / project override).
 * Provider-neutral; null at both levels means the built-in default (time/8h/5d).
 * Mirrors the backend canonicalisation in handlers/estimation.go.
 */
data class EstimationConfig(
    @SerializedName("unit") val unit: String = "time", // time | points | custom
    @SerializedName("hours_per_day") val hoursPerDay: Double? = null,
    @SerializedName("days_per_week") val daysPerWeek: Double? = null,
    @SerializedName("points_scale") val pointsScale: String? = null, // fibonacci|tshirt|linear
    @SerializedName("custom_label") val customLabel: String? = null,
)

/** A workspace — the top of the hierarchy. Mirrors the sqlc `Workspace`. */
data class Workspace(
    @SerializedName("id") val id: String = "",
    @SerializedName("name") val name: String = "",
    @SerializedName("owner_id") val ownerId: String = "",
    @SerializedName("task_counter") val taskCounter: Long = 0,
    @SerializedName("estimation") val estimation: EstimationConfig? = null,
)

/** A project group; nests via [parentId]. Mirrors sqlc `ProjectGroup`. */
data class ProjectGroup(
    @SerializedName("id") val id: String = "",
    @SerializedName("workspace_id") val workspaceId: String = "",
    @SerializedName("parent_id") val parentId: String? = null,
    @SerializedName("name") val name: String = "",
    @SerializedName("icon") val icon: String = "",
    @SerializedName("color") val color: String = "",
    @SerializedName("icon_mode") val iconMode: String = "badge",
    @SerializedName("position") val position: Double = 0.0,
)

/** A project; belongs to a group ([groupId]) or the root. Mirrors sqlc `Project`. */
data class Project(
    @SerializedName("id") val id: String = "",
    @SerializedName("workspace_id") val workspaceId: String = "",
    @SerializedName("group_id") val groupId: String? = null,
    @SerializedName("name") val name: String = "",
    @SerializedName("icon") val icon: String = "",
    @SerializedName("color") val color: String = "",
    @SerializedName("icon_mode") val iconMode: String = "badge",
    @SerializedName("position") val position: Double = 0.0,
    @SerializedName("estimation") val estimation: EstimationConfig? = null,
)

/** A board within a project. Mirrors sqlc `Board`. */
data class Board(
    @SerializedName("id") val id: String = "",
    @SerializedName("project_id") val projectId: String = "",
    @SerializedName("name") val name: String = "",
    @SerializedName("position") val position: Double = 0.0,
    @SerializedName("done_column_id") val doneColumnId: String? = null,
)

/** A board column. Mirrors sqlc `BoardColumn`. */
data class BoardColumn(
    @SerializedName("id") val id: String = "",
    @SerializedName("board_id") val boardId: String = "",
    @SerializedName("name") val name: String = "",
    @SerializedName("color") val color: String = "",
    @SerializedName("position") val position: Double = 0.0,
)

/** A workspace tag. Mirrors sqlc `Tag`. */
data class Tag(
    @SerializedName("id") val id: String = "",
    @SerializedName("project_id") val projectId: String = "",
    @SerializedName("name") val name: String = "",
    @SerializedName("color") val color: String = "",
)

// ── Request bodies ──────────────────────────────────────────────────────────

data class NameRequest(@SerializedName("name") val name: String)

data class CreateGroupRequest(
    @SerializedName("name") val name: String,
    @SerializedName("parent_id") val parentId: String? = null,
)

data class CreateProjectRequest(
    @SerializedName("name") val name: String,
    @SerializedName("icon") val icon: String? = null,
    @SerializedName("color") val color: String? = null,
    @SerializedName("group_id") val groupId: String? = null,
)

data class UpdateProjectRequest(
    @SerializedName("name") val name: String,
    @SerializedName("color") val color: String = "",
    @SerializedName("icon") val icon: String = "",
    @SerializedName("group_id") val groupId: String? = null,
    @SerializedName("icon_mode") val iconMode: String = "badge",
)

data class UpdateGroupRequest(
    @SerializedName("name") val name: String,
    @SerializedName("color") val color: String = "",
    @SerializedName("icon") val icon: String = "",
    @SerializedName("icon_mode") val iconMode: String = "badge",
)

/** Re-parents and/or repositions a group among its siblings (server computes the
 *  midpoint position from before_id / after_id). */
data class MoveGroupRequest(
    @SerializedName("parent_id") val parentId: String?,
    @SerializedName("before_id") val beforeId: String?,
    @SerializedName("after_id") val afterId: String?,
)

/** Re-groups and/or repositions a project among its siblings. */
data class MoveProjectRequest(
    @SerializedName("group_id") val groupId: String?,
    @SerializedName("before_id") val beforeId: String?,
    @SerializedName("after_id") val afterId: String?,
)
