package website.msdnna.tessera.data.repository

import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.model.Tag
import website.msdnna.tessera.data.model.WorkspaceSummary
import website.msdnna.tessera.data.model.WorkspaceTask

/** Powers the Home / "Моя работа" dashboard: summary counts + the task list. */
class HomeRepository {
    private val api get() = AppContainer.api()

    suspend fun summary(workspaceId: String): WorkspaceSummary = api.workspaceSummary(workspaceId)
    suspend fun tasks(workspaceId: String): List<WorkspaceTask> = api.workspaceTasks(workspaceId).orEmpty()
    suspend fun tags(workspaceId: String): List<Tag> = api.workspaceTags(workspaceId).orEmpty()
}
