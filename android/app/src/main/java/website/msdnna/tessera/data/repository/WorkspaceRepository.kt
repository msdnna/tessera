package website.msdnna.tessera.data.repository

import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.model.Board
import website.msdnna.tessera.data.model.BoardColumn
import website.msdnna.tessera.data.model.CreateGroupRequest
import website.msdnna.tessera.data.model.CreateProjectRequest
import website.msdnna.tessera.data.model.NameRequest
import website.msdnna.tessera.data.model.Project
import website.msdnna.tessera.data.model.ProjectGroup
import website.msdnna.tessera.data.model.UpdateGroupRequest
import website.msdnna.tessera.data.model.UpdateProjectRequest
import website.msdnna.tessera.data.model.Workspace

/** Reads/writes the workspace → group → project → board hierarchy. */
class WorkspaceRepository {
    private val api get() = AppContainer.api()

    suspend fun listWorkspaces(): List<Workspace> = api.workspaces().orEmpty()
    suspend fun createWorkspace(name: String): Workspace =
        api.createWorkspace(website.msdnna.tessera.data.model.NameRequest(name))

    suspend fun groups(workspaceId: String): List<ProjectGroup> = api.groups(workspaceId).orEmpty()
    suspend fun projects(workspaceId: String): List<Project> = api.projects(workspaceId).orEmpty()

    suspend fun createGroup(workspaceId: String, name: String, parentId: String? = null): ProjectGroup =
        api.createGroup(workspaceId, CreateGroupRequest(name, parentId))

    suspend fun updateGroup(
        group: ProjectGroup,
        name: String = group.name,
        color: String = group.color,
        icon: String = group.icon,
    ): ProjectGroup = api.updateGroup(group.id, UpdateGroupRequest(name, color, icon))

    suspend fun moveGroup(groupId: String, parentId: String?, beforeId: String?, afterId: String?): ProjectGroup =
        api.moveGroup(groupId, website.msdnna.tessera.data.model.MoveGroupRequest(parentId, beforeId, afterId))

    suspend fun deleteGroup(groupId: String) = api.deleteGroup(groupId)

    suspend fun createProject(workspaceId: String, name: String, groupId: String? = null): Project =
        api.createProject(workspaceId, CreateProjectRequest(name = name, groupId = groupId))

    suspend fun updateProject(
        project: Project,
        name: String = project.name,
        color: String = project.color,
        icon: String = project.icon,
        groupId: String? = project.groupId,
    ): Project = api.updateProject(project.id, UpdateProjectRequest(name, color, icon, groupId))

    suspend fun moveProject(projectId: String, groupId: String?, beforeId: String?, afterId: String?): Project =
        api.moveProject(projectId, website.msdnna.tessera.data.model.MoveProjectRequest(groupId, beforeId, afterId))

    suspend fun deleteProject(projectId: String) = api.deleteProject(projectId)

    suspend fun boards(projectId: String): List<Board> = api.boards(projectId).orEmpty()
    suspend fun createBoard(projectId: String, name: String): Board = api.createBoard(projectId, NameRequest(name))
    suspend fun renameBoard(boardId: String, name: String): Board = api.updateBoard(boardId, NameRequest(name))
    suspend fun deleteBoard(boardId: String) = api.deleteBoard(boardId)

    suspend fun columns(boardId: String): List<BoardColumn> = api.columns(boardId).orEmpty()

    // ── members ──────────────────────────────────────────────────────────────
    suspend fun members(workspaceId: String): List<website.msdnna.tessera.data.model.Member> =
        api.members(workspaceId).orEmpty()
    suspend fun addMember(workspaceId: String, email: String, role: String) =
        api.addMember(workspaceId, website.msdnna.tessera.data.model.AddMemberRequest(email.trim(), role))
    suspend fun removeMember(workspaceId: String, userId: String) = api.removeMember(workspaceId, userId)
    suspend fun updateMemberRole(workspaceId: String, userId: String, role: String) =
        api.updateMemberRole(workspaceId, userId, website.msdnna.tessera.data.model.RoleUpdate(role))

    suspend fun invitations(workspaceId: String): List<website.msdnna.tessera.data.model.Invitation> =
        api.invitations(workspaceId).orEmpty()
    suspend fun createInvitation(workspaceId: String, email: String, role: String) =
        api.createInvitation(workspaceId, website.msdnna.tessera.data.model.InviteRequest(email.trim(), role))
    suspend fun deleteInvitation(workspaceId: String, invId: String) = api.deleteInvitation(workspaceId, invId)
}
