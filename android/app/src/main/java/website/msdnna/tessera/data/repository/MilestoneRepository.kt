package website.msdnna.tessera.data.repository

import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.model.Milestone
import website.msdnna.tessera.data.model.MilestonePushResult
import website.msdnna.tessera.data.model.MilestoneRequest
import website.msdnna.tessera.data.model.SetTaskMilestoneRequest
import website.msdnna.tessera.data.model.WorkspaceMilestone

/** Milestone («Этап») CRUD, task assignment and the opt-in GitLab push. */
class MilestoneRepository {
    private val api get() = AppContainer.api()

    suspend fun list(projectId: String): List<Milestone> = api.milestones(projectId).orEmpty()

    suspend fun workspaceList(workspaceId: String): List<WorkspaceMilestone> =
        api.workspaceMilestones(workspaceId).orEmpty()

    suspend fun create(
        projectId: String,
        title: String,
        startDate: String? = null,
        dueDate: String? = null,
    ): Milestone = api.createMilestone(projectId, MilestoneRequest(title.trim(), "", startDate, dueDate))

    suspend fun update(
        milestoneId: String,
        title: String,
        description: String,
        startDate: String?,
        dueDate: String?,
        state: String,
    ): Milestone = api.updateMilestone(
        milestoneId, MilestoneRequest(title.trim(), description, startDate, dueDate, state),
    )

    suspend fun delete(milestoneId: String) = api.deleteMilestone(milestoneId)

    /** Assign (non-null) or clear (null) a task's milestone. */
    suspend fun setTask(taskId: String, milestoneId: String?) {
        if (milestoneId == null) api.clearTaskMilestone(taskId)
        else api.setTaskMilestone(taskId, SetTaskMilestoneRequest(milestoneId))
    }

    suspend fun pushToGitlab(milestoneId: String): MilestonePushResult =
        api.pushMilestoneToGitlab(milestoneId)
}
