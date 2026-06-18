package website.msdnna.tessera.data.repository

import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.model.GitlabConnectRequest
import website.msdnna.tessera.data.model.GitlabConnection
import website.msdnna.tessera.data.model.GitlabIntegration
import website.msdnna.tessera.data.model.GitlabSetIntegrationRequest
import website.msdnna.tessera.data.model.GitlabSyncResult

/** A workspace board flattened for a picker: `id` + a `Project / Board` label.
 *  [projectId] lets the integration editor resolve the target project for the
 *  project-scoped tag-prefix display names. */
data class BoardOption(val id: String, val label: String, val projectId: String)

/** GitLab integration: per-user connection, per-workspace config + manual sync,
 *  plus the board/column lookups the config editor needs. */
class GitlabRepository {
    private val api get() = AppContainer.api()

    suspend fun connection(): GitlabConnection = api.gitlabConnection()
    suspend fun connect(baseUrl: String, token: String): GitlabConnection =
        api.gitlabConnect(GitlabConnectRequest(baseUrl, token))
    suspend fun disconnect() = api.gitlabDisconnect()

    suspend fun integration(workspaceId: String): GitlabIntegration = api.gitlabIntegration(workspaceId)
    suspend fun setIntegration(workspaceId: String, req: GitlabSetIntegrationRequest): GitlabIntegration =
        api.gitlabSetIntegration(workspaceId, req)
    suspend fun sync(workspaceId: String): GitlabSyncResult = api.gitlabSync(workspaceId)

    /** Every board in the workspace, labelled `Project / Board`, for the picker. */
    suspend fun workspaceBoards(workspaceId: String): List<BoardOption> {
        val out = mutableListOf<BoardOption>()
        for (project in api.projects(workspaceId).orEmpty()) {
            for (board in api.boards(project.id).orEmpty()) {
                out.add(BoardOption(board.id, "${project.name} / ${board.name}", project.id))
            }
        }
        return out
    }

    /** The column names of a board (for the status / default-column pickers). */
    suspend fun columnNames(boardId: String): List<String> =
        api.columns(boardId).orEmpty().map { it.name }

    /** A project's tag-prefix display names (for the GitLab rule editor). */
    suspend fun tagPrefixes(projectId: String): List<website.msdnna.tessera.data.model.TagPrefix> =
        api.tagPrefixes(projectId).orEmpty()

    suspend fun setTagPrefixes(
        projectId: String,
        prefixes: List<website.msdnna.tessera.data.model.TagPrefixEntry>,
    ): List<website.msdnna.tessera.data.model.TagPrefix> =
        api.setTagPrefixes(projectId, website.msdnna.tessera.data.model.SetTagPrefixesRequest(prefixes)).orEmpty()
}
