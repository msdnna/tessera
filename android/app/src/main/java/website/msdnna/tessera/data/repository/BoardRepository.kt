package website.msdnna.tessera.data.repository

import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.model.AddAssigneeRequest
import website.msdnna.tessera.data.model.AddTagRequest
import website.msdnna.tessera.data.model.BoardColumn
import website.msdnna.tessera.data.model.CreateTagRequest
import website.msdnna.tessera.data.model.CreateTaskRequest
import website.msdnna.tessera.data.model.DueNotifyRequest
import website.msdnna.tessera.data.model.GitlabMember
import website.msdnna.tessera.data.model.Member
import website.msdnna.tessera.data.model.MoveTaskRequest
import website.msdnna.tessera.data.model.PinGitlabAssigneeRequest
import website.msdnna.tessera.data.model.Recurrence
import website.msdnna.tessera.data.model.SetEisenhowerRequest
import website.msdnna.tessera.data.model.SetParentRequest
import website.msdnna.tessera.data.model.Tag
import website.msdnna.tessera.data.model.Task
import website.msdnna.tessera.data.model.UpdateTaskRequest

/** Reads a board's columns + cards and performs all card mutations. */
class BoardRepository {
    private val api get() = AppContainer.api()

    suspend fun board(boardId: String): website.msdnna.tessera.data.model.Board = api.board(boardId)
    suspend fun boards(projectId: String): List<website.msdnna.tessera.data.model.Board> =
        api.boards(projectId).orEmpty()

    /** Resolves a task's board id (for opening a task when only its id is known). */
    suspend fun taskBoardId(taskId: String): String = api.task(taskId).boardId
    suspend fun columns(boardId: String): List<BoardColumn> = api.columns(boardId).orEmpty()
    suspend fun createColumn(boardId: String, name: String): BoardColumn =
        api.createColumn(boardId, website.msdnna.tessera.data.model.CreateColumnRequest(name))
    suspend fun renameColumn(column: BoardColumn, name: String): BoardColumn =
        api.updateColumn(column.id, website.msdnna.tessera.data.model.UpdateColumnRequest(name, column.color))
    suspend fun setColumnColor(column: BoardColumn, color: String): BoardColumn =
        api.updateColumn(column.id, website.msdnna.tessera.data.model.UpdateColumnRequest(column.name, color))
    suspend fun deleteColumn(columnId: String) = api.deleteColumn(columnId)
    suspend fun moveColumn(columnId: String, beforeId: String?, afterId: String?): BoardColumn =
        api.moveColumn(columnId, website.msdnna.tessera.data.model.ColumnMoveRequest(beforeId, afterId))
    suspend fun setDoneColumn(boardId: String, columnId: String?) =
        api.setDoneColumn(boardId, website.msdnna.tessera.data.model.SetDoneColumnRequest(columnId))
    suspend fun tasks(boardId: String, milestone: String? = null, archived: Boolean = false): List<Task> =
        api.boardTasks(boardId, milestone, if (archived) 1 else null).orEmpty()
    suspend fun subtasks(boardId: String): List<Task> = api.boardSubtasks(boardId).orEmpty()
    suspend fun dependencies(boardId: String): List<website.msdnna.tessera.data.model.BoardDependency> =
        api.boardDependencies(boardId).orEmpty()
    suspend fun tags(projectId: String): List<Tag> = api.tags(projectId).orEmpty()
    suspend fun milestones(projectId: String): List<website.msdnna.tessera.data.model.Milestone> =
        api.milestones(projectId).orEmpty()

    /** Assign (non-null) or clear (null) a task's milestone. */
    suspend fun setTaskMilestone(taskId: String, milestoneId: String?) {
        if (milestoneId == null) {
            api.clearTaskMilestone(taskId)
        } else {
            api.setTaskMilestone(taskId, website.msdnna.tessera.data.model.SetTaskMilestoneRequest(milestoneId))
        }
    }

    /** Inline milestone create from a task picker (returns the new milestone). */
    suspend fun createMilestone(projectId: String, title: String): website.msdnna.tessera.data.model.Milestone =
        api.createMilestone(projectId, website.msdnna.tessera.data.model.MilestoneRequest(title.trim()))
    suspend fun members(workspaceId: String): List<Member> = api.members(workspaceId).orEmpty()

    /** Resolved estimation config for a project: project override → workspace
     *  default → built-in. Best-effort (uses the list endpoints; the board has no
     *  single project/workspace getter). */
    suspend fun estimationConfig(
        workspaceId: String,
        projectId: String,
    ): website.msdnna.tessera.data.model.EstimationConfig {
        val ws = runCatching { api.workspaces().orEmpty().firstOrNull { it.id == workspaceId } }.getOrNull()
        val proj = runCatching { api.projects(workspaceId).orEmpty().firstOrNull { it.id == projectId } }.getOrNull()
        return website.msdnna.tessera.util.Estimation.resolve(proj, ws)
    }

    /** A project's prefix→label display names (canonical prefix keys). */
    suspend fun tagPrefixes(projectId: String): List<website.msdnna.tessera.data.model.TagPrefix> =
        api.tagPrefixes(projectId).orEmpty()

    /** Replace-all: send the complete merged set (blank labels are dropped server-side). */
    suspend fun setTagPrefixes(
        projectId: String,
        prefixes: List<website.msdnna.tessera.data.model.TagPrefixEntry>,
    ): List<website.msdnna.tessera.data.model.TagPrefix> =
        api.setTagPrefixes(projectId, website.msdnna.tessera.data.model.SetTagPrefixesRequest(prefixes)).orEmpty()

    /** Two-level estimation config; pass null to clear (inherit). */
    suspend fun setWorkspaceEstimation(
        workspaceId: String,
        config: website.msdnna.tessera.data.model.EstimationConfig?,
    ): website.msdnna.tessera.data.model.Workspace = api.setWorkspaceEstimation(workspaceId, config)

    suspend fun setProjectEstimation(
        projectId: String,
        config: website.msdnna.tessera.data.model.EstimationConfig?,
    ): website.msdnna.tessera.data.model.Project = api.setProjectEstimation(projectId, config)

    suspend fun createTask(boardId: String, columnId: String, title: String, parentId: String? = null): Task =
        api.createTask(boardId, CreateTaskRequest(columnId = columnId, title = title, parentId = parentId))

    suspend fun moveTask(taskId: String, columnId: String, beforeId: String? = null, afterId: String? = null): Task =
        api.moveTask(taskId, MoveTaskRequest(columnId, beforeId, afterId))

    suspend fun setParent(taskId: String, parentId: String?): Task =
        api.setTaskParent(taskId, SetParentRequest(parentId))

    /** Pin a task to an Eisenhower quadrant (0-3), or null to derive it automatically. */
    suspend fun setEisenhower(taskId: String, quadrant: Int?): Task =
        api.setTaskEisenhower(taskId, SetEisenhowerRequest(quadrant))

    /** Per-task due-notification overrides (null = inherit the user default). */
    suspend fun setDueNotify(taskId: String, lead: Int?, repeat: Int?, enabled: Boolean?): Task =
        api.setDueNotify(taskId, DueNotifyRequest(lead, repeat, enabled))

    /** Full update — pass the task plus the fields you're changing. Recurrence and
     *  start_date default to the task's current values so unrelated edits don't
     *  wipe them (the backend update is full-replace, like web). */
    suspend fun updateTask(
        task: Task,
        title: String = task.title,
        description: String = task.description,
        priority: Int = task.priority,
        dueDate: String? = task.dueDate,
        startDate: String? = task.startDate,
        estimate: Double? = task.estimate,
        completed: Boolean = task.isCompleted,
        recurrence: Recurrence? = task.recurrence,
    ): Task = api.updateTask(
        task.id,
        UpdateTaskRequest(title, description, priority, dueDate, startDate, estimate, completed, recurrence),
    )

    suspend fun archiveTask(taskId: String) = api.archiveTask(taskId)
    suspend fun deleteTask(taskId: String) = api.deleteTask(taskId)

    suspend fun addTag(taskId: String, tagId: String) = api.addTaskTag(taskId, AddTagRequest(tagId))
    suspend fun removeTag(taskId: String, tagId: String) = api.removeTaskTag(taskId, tagId)
    suspend fun addAssignee(taskId: String, userId: String) = api.addTaskAssignee(taskId, AddAssigneeRequest(userId))
    suspend fun removeAssignee(taskId: String, userId: String) = api.removeTaskAssignee(taskId, userId)
    suspend fun gitlabMembers(workspaceId: String): List<GitlabMember> =
        runCatching { api.gitlabMembers(workspaceId).orEmpty() }.getOrDefault(emptyList())

    /** GitLab integrations of a workspace — used to derive which label prefixes are
     *  meta (status/priority/…) and should be hidden from the tag-picker. */
    suspend fun gitlabIntegrations(workspaceId: String): List<website.msdnna.tessera.data.model.GitlabIntegration> =
        runCatching { api.gitlabIntegrations(workspaceId).integrations }.getOrDefault(emptyList())
    suspend fun pinGitlabAssignee(taskId: String, m: GitlabMember) =
        api.pinGitlabAssignee(taskId, PinGitlabAssigneeRequest(m.glUsername, m.glName, m.glAvatarUrl))
    suspend fun removeGitlabAssignee(taskId: String, username: String) = api.removeGitlabAssignee(taskId, username)

    /** The workspace's quick-action registry (built-ins + custom dictionary + the
     *  caller's right to edit it). Best-effort: without it the editor simply has
     *  no `/`-popup, which must not break the board load. */
    suspend fun workspaceCommands(workspaceId: String): website.msdnna.tessera.data.model.CommandRegistry =
        runCatching { api.workspaceCommands(workspaceId) }
            .getOrDefault(website.msdnna.tessera.data.model.CommandRegistry())

    /** Replaces the whole custom dictionary (complete desired state, like tag prefixes). */
    suspend fun setWorkspaceCommands(
        workspaceId: String,
        commands: List<website.msdnna.tessera.data.model.WorkspaceCommand>,
    ): List<website.msdnna.tessera.data.model.WorkspaceCommand> =
        api.setWorkspaceCommands(
            workspaceId,
            website.msdnna.tessera.data.model.SetWorkspaceCommandsRequest(commands),
        ).orEmpty()

    suspend fun createTag(projectId: String, name: String, color: String): Tag =
        api.createTag(projectId, CreateTagRequest(name, color))
    suspend fun updateTag(tagId: String, name: String, color: String): Tag =
        api.updateTag(tagId, CreateTagRequest(name, color))
    suspend fun deleteTag(tagId: String) = api.deleteTag(tagId)

    // ── archive ──────────────────────────────────────────────────────────────
    suspend fun archived(boardId: String): List<Task> = api.boardArchive(boardId).orEmpty()
    suspend fun restoreTask(taskId: String) = api.restoreTask(taskId)

    // ── saved board views (per-user, server-side) ─────────────────────────────
    suspend fun views(boardId: String): List<website.msdnna.tessera.data.model.BoardView> =
        api.boardViews(boardId).orEmpty()

    suspend fun saveView(
        boardId: String,
        name: String,
        config: website.msdnna.tessera.data.model.BoardViewConfig,
    ): website.msdnna.tessera.data.model.BoardView =
        api.saveBoardView(boardId, website.msdnna.tessera.data.model.SaveBoardViewRequest(name, config))

    suspend fun deleteView(viewId: String) = api.deleteBoardView(viewId)
}
