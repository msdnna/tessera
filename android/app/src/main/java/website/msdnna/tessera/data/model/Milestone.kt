package website.msdnna.tessera.data.model

import com.google.gson.annotations.SerializedName

/**
 * A milestone («Этап») — a project-scoped planning unit, the Android mirror of the
 * web milestones feature. A task points at ≤1 milestone via `milestone_id`. When a
 * `gl_global_id` is present the milestone is GitLab-sourced (read-only locally; the
 * next pull re-asserts GitLab's values). `gl_url`/`gl_global_id` are populated by the
 * project-scoped list; the bare create/update responses omit them (treated null).
 */
data class Milestone(
    @SerializedName("id") val id: String = "",
    @SerializedName("project_id") val projectId: String = "",
    @SerializedName("title") val title: String = "",
    @SerializedName("description") val description: String = "",
    @SerializedName("start_date") val startDate: String? = null,
    @SerializedName("due_date") val dueDate: String? = null,
    @SerializedName("state") val state: String = "active", // active | closed
    @SerializedName("position") val position: Double = 0.0,
    @SerializedName("gl_url") val glUrl: String? = null,
    @SerializedName("gl_global_id") val glGlobalId: String? = null,
) {
    val isClosed: Boolean get() = state == "closed"

    /** GitLab-sourced milestones are read-only in Tessera (managed by the sync). */
    val isLinked: Boolean get() = !glGlobalId.isNullOrBlank()
}

/**
 * A workspace-aggregate milestone row for the «Этапы» screen — every milestone
 * across the workspace's projects with task rollups. Mirrors the backend
 * `ListWorkspaceMilestonesRow`.
 */
data class WorkspaceMilestone(
    @SerializedName("id") val id: String = "",
    @SerializedName("project_id") val projectId: String = "",
    @SerializedName("title") val title: String = "",
    @SerializedName("description") val description: String = "",
    @SerializedName("start_date") val startDate: String? = null,
    @SerializedName("due_date") val dueDate: String? = null,
    @SerializedName("state") val state: String = "active",
    @SerializedName("position") val position: Double = 0.0,
    @SerializedName("gl_url") val glUrl: String? = null,
    @SerializedName("gl_global_id") val glGlobalId: String? = null,
    @SerializedName("project_name") val projectName: String = "",
    @SerializedName("project_slug") val projectSlug: String = "",
    @SerializedName("board_slug") val boardSlug: String? = null,
    @SerializedName("task_count") val taskCount: Long = 0,
    @SerializedName("done_count") val doneCount: Long = 0,
    @SerializedName("estimate_sum") val estimateSum: Double = 0.0,
) {
    val isClosed: Boolean get() = state == "closed"
    val isLinked: Boolean get() = !glGlobalId.isNullOrBlank()

    /** Bare milestone view (for date-range formatting / pickers). */
    fun asMilestone(): Milestone = Milestone(
        id = id, projectId = projectId, title = title, description = description,
        startDate = startDate, dueDate = dueDate, state = state, position = position,
        glUrl = glUrl, glGlobalId = glGlobalId,
    )
}

/** Create/update payload (dates are RFC3339 UTC-midnight; state defaults active). */
data class MilestoneRequest(
    @SerializedName("title") val title: String,
    @SerializedName("description") val description: String = "",
    @SerializedName("start_date") val startDate: String? = null,
    @SerializedName("due_date") val dueDate: String? = null,
    @SerializedName("state") val state: String = "active",
)

/** Assign a milestone to a task (null clears it). */
data class SetTaskMilestoneRequest(@SerializedName("milestone_id") val milestoneId: String?)

/** Response of POST /milestones/:id/gitlab — the freshly-linked GitLab milestone. */
data class MilestonePushResult(
    @SerializedName("id") val id: String = "",
    @SerializedName("gl_url") val glUrl: String? = null,
    @SerializedName("gl_linked") val glLinked: Boolean = false,
)
