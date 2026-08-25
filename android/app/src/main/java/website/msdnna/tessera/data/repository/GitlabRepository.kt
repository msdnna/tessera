package website.msdnna.tessera.data.repository

import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.model.GitlabConnectRequest
import website.msdnna.tessera.data.model.GitlabConnection
import website.msdnna.tessera.data.model.GitlabIntegration
import website.msdnna.tessera.data.model.GitlabIntegrationRequest
import website.msdnna.tessera.data.model.GitlabIntegrationsResponse
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

    /** All GL bindings of a workspace + capability flags (service token / is_admin). */
    suspend fun integrations(workspaceId: String): GitlabIntegrationsResponse =
        api.gitlabIntegrations(workspaceId)

    suspend fun createIntegration(workspaceId: String, req: GitlabIntegrationRequest): GitlabIntegration =
        api.gitlabCreateIntegration(workspaceId, req)

    suspend fun updateIntegration(workspaceId: String, id: String, req: GitlabIntegrationRequest): GitlabIntegration =
        api.gitlabUpdateIntegration(workspaceId, id, req)

    suspend fun deleteIntegration(workspaceId: String, id: String) =
        api.gitlabDeleteIntegration(workspaceId, id)

    /** Manual sync; [full] = true asks for a full sweep (`?mode=full`) instead of the
     *  default incremental pull. */
    suspend fun sync(workspaceId: String, integrationId: String, full: Boolean = false): GitlabSyncResult =
        api.gitlabSyncIntegration(workspaceId, integrationId, if (full) "full" else null)

    /** Sync-journal: recent runs, the actions of one run, and retrying a failed push. */
    suspend fun syncRuns(workspaceId: String): List<website.msdnna.tessera.data.model.GitlabSyncRun> =
        api.gitlabSyncRuns(workspaceId).orEmpty()
    suspend fun syncActions(workspaceId: String, runId: String): List<website.msdnna.tessera.data.model.GitlabSyncAction> =
        api.gitlabSyncActions(workspaceId, runId).orEmpty()
    suspend fun retryWriteback(workspaceId: String, runId: String, actionId: String) =
        api.gitlabRetryWriteback(workspaceId, runId, actionId)

    /** Write-back conflicts of a workspace ([] when no integration / none open). */
    suspend fun conflicts(workspaceId: String): List<website.msdnna.tessera.data.model.GitlabConflict> =
        runCatching { api.gitlabConflicts(workspaceId).orEmpty() }.getOrDefault(emptyList())

    /** Resolve a conflict: ours | theirs | manual (manual carries merged [value]). */
    suspend fun resolveConflict(taskId: String, conflictId: String, resolution: String, value: Map<String, String>?) =
        api.resolveGitlabConflict(
            taskId, conflictId,
            website.msdnna.tessera.data.model.ResolveConflictRequest(resolution, value),
        )

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

    /** The board's columns for the rule editor: the write-back binding trigger matches
     *  by stable id, правило статуса — по имени, а подпись собирается из `name_key`
     *  (#2800), поэтому колонка едет целиком, а не парой полей. */
    suspend fun columns(boardId: String): List<website.msdnna.tessera.data.model.BoardColumn> =
        api.columns(boardId).orEmpty()

    /** A project's tag-prefix display names (for the GitLab rule editor). */
    suspend fun tagPrefixes(projectId: String): List<website.msdnna.tessera.data.model.TagPrefix> =
        api.tagPrefixes(projectId).orEmpty()

    suspend fun setTagPrefixes(
        projectId: String,
        prefixes: List<website.msdnna.tessera.data.model.TagPrefixEntry>,
    ): List<website.msdnna.tessera.data.model.TagPrefix> =
        api.setTagPrefixes(projectId, website.msdnna.tessera.data.model.SetTagPrefixesRequest(prefixes)).orEmpty()
}
