package website.msdnna.tessera.ui.viewmodels

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.model.Board
import website.msdnna.tessera.data.model.Project
import website.msdnna.tessera.data.model.ProjectGroup
import website.msdnna.tessera.data.model.Workspace
import website.msdnna.tessera.data.repository.WorkspaceRepository
import website.msdnna.tessera.util.errorMessage

data class WorkspaceUiState(
    val loading: Boolean = true,
    val error: String? = null,
    val workspaces: List<Workspace> = emptyList(),
    val currentId: String = "",
    val groups: List<ProjectGroup> = emptyList(),
    val projects: List<Project> = emptyList(),
    val boardsByProject: Map<String, List<Board>> = emptyMap(),
    /** Lazily-loaded project milestones («Этапы») for the sidebar stages tree. */
    val milestonesByProject: Map<String, List<website.msdnna.tessera.data.model.Milestone>> = emptyMap(),
    val expandedProjects: Set<String> = emptySet(),
    val expandedGroups: Set<String> = emptySet(),
    /** Project ids whose stages tree shows closed milestones (persisted). */
    val showClosedStages: Set<String> = emptySet(),
) {
    val current: Workspace? get() = workspaces.find { it.id == currentId }

    fun childGroups(parentId: String?): List<ProjectGroup> =
        groups.filter { (it.parentId) == parentId }.sortedBy { it.position }

    fun projectsInGroup(groupId: String?): List<Project> =
        projects.filter { (it.groupId) == groupId }.sortedBy { it.position }
}

/** Owns the sidebar tree: workspaces, the current selection, groups/projects
 *  and lazily-loaded boards. CRUD mutations refresh the affected slice. */
class WorkspaceViewModel(
    private val repo: WorkspaceRepository = WorkspaceRepository(),
) : ViewModel() {
    private val prefs get() = AppContainer.prefs

    private val _state = MutableStateFlow(WorkspaceUiState())
    val state: StateFlow<WorkspaceUiState> = _state.asStateFlow()

    init {
        load()
    }

    fun load() = launchCatching {
        val list = repo.listWorkspaces()
        val saved = prefs.currentWorkspaceId.first()
        // Restore the persisted sidebar expand state before selecting the workspace
        // so the tree opens to where it was left and the right boards preload.
        val expandedGroups = prefs.expandedGroups.first()
        val expandedProjects = prefs.expandedProjects.first()
        val showClosedStages = prefs.showClosedStages.first()
        val current = list.firstOrNull { it.id == saved }?.id ?: list.firstOrNull()?.id ?: ""
        _state.update {
            it.copy(
                loading = false,
                workspaces = list,
                currentId = current,
                expandedGroups = expandedGroups,
                expandedProjects = expandedProjects,
                showClosedStages = showClosedStages,
            )
        }
        if (current.isNotBlank()) selectWorkspace(current)
    }

    fun addWorkspace(name: String) = launchCatching {
        val ws = repo.createWorkspace(name)
        val list = repo.listWorkspaces()
        _state.update { it.copy(workspaces = list) }
        selectWorkspace(ws.id)
    }

    /** Owner-only workspace deletion (server re-checks). If the deleted workspace
     *  was active, switches to the first remaining one and calls [onGone] so the
     *  host can leave for Home. Refuses to delete the only workspace. */
    fun removeWorkspace(id: String, onGone: () -> Unit) = launchCatching {
        if (_state.value.workspaces.size <= 1) {
            _state.update { it.copy(error = "Нельзя удалить единственное пространство") }
            return@launchCatching
        }
        val wasCurrent = _state.value.currentId == id
        repo.deleteWorkspace(id)
        val list = repo.listWorkspaces()
        _state.update { it.copy(workspaces = list) }
        if (wasCurrent) {
            val next = list.firstOrNull()?.id ?: ""
            if (next.isNotBlank()) selectWorkspace(next)
            onGone()
        }
    }

    fun selectWorkspace(id: String) = launchCatching {
        prefs.setCurrentWorkspaceId(id)
        _state.update { it.copy(currentId = id, boardsByProject = emptyMap(), milestonesByProject = emptyMap()) }
        val groups = repo.groups(id)
        val projects = repo.projects(id)
        _state.update { it.copy(groups = groups, projects = projects) }
        // Preload children for any project restored as expanded (lazy: boards always,
        // milestones when its tree shows stages).
        _state.value.expandedProjects
            .filter { pid -> projects.any { it.id == pid } }
            .forEach { ensureProjectChildren(it) }
    }

    fun refresh() = launchCatching {
        val id = _state.value.currentId
        if (id.isBlank()) return@launchCatching
        val groups = repo.groups(id)
        val projects = repo.projects(id)
        _state.update { it.copy(groups = groups, projects = projects) }
    }

    fun toggleProject(projectId: String) {
        val expanded = _state.value.expandedProjects
        if (projectId in expanded) {
            _state.update { it.copy(expandedProjects = expanded - projectId) }
        } else {
            _state.update { it.copy(expandedProjects = expanded + projectId) }
            ensureProjectChildren(projectId)
        }
        persistExpansion()
    }

    /** Lazily load a project's boards (always — needed as a milestone's open target)
     *  and its milestones (only when the tree shows stages). */
    private fun ensureProjectChildren(projectId: String) {
        if (_state.value.boardsByProject[projectId] == null) loadBoards(projectId)
        val project = _state.value.projects.find { it.id == projectId }
        if (project != null && project.treeMode != "boards" && _state.value.milestonesByProject[projectId] == null) {
            loadMilestones(projectId)
        }
    }

    fun loadMilestones(projectId: String) = launchCatching {
        val ms = repo.milestones(projectId)
        _state.update { it.copy(milestonesByProject = it.milestonesByProject + (projectId to ms)) }
    }

    fun toggleGroup(groupId: String) {
        val expanded = _state.value.expandedGroups
        _state.update {
            it.copy(expandedGroups = if (groupId in expanded) expanded - groupId else expanded + groupId)
        }
        persistExpansion()
    }

    /** Force a group open (no toggle) — used when inline-creating a child. */
    fun ensureGroupExpanded(groupId: String) {
        if (groupId !in _state.value.expandedGroups) {
            _state.update { it.copy(expandedGroups = it.expandedGroups + groupId) }
            persistExpansion()
        }
    }

    /** Force a project open (no toggle), loading its boards lazily. */
    fun ensureProjectExpanded(projectId: String) {
        if (projectId !in _state.value.expandedProjects) {
            _state.update { it.copy(expandedProjects = it.expandedProjects + projectId) }
            persistExpansion()
        }
        ensureProjectChildren(projectId)
    }

    /** Persist the current expand state so the tree restores on next launch. */
    private fun persistExpansion() {
        val groups = _state.value.expandedGroups
        val projects = _state.value.expandedProjects
        viewModelScope.launch {
            prefs.setExpandedGroups(groups)
            prefs.setExpandedProjects(projects)
        }
    }

    fun loadBoards(projectId: String) = launchCatching {
        val boards = repo.boards(projectId)
        _state.update { it.copy(boardsByProject = it.boardsByProject + (projectId to boards)) }
    }

    fun addProject(name: String, groupId: String? = null) = launchCatching {
        repo.createProject(_state.value.currentId, name, groupId)
        refreshSilently()
    }

    fun addGroup(name: String, parentId: String? = null) = launchCatching {
        repo.createGroup(_state.value.currentId, name, parentId)
        refreshSilently()
    }

    fun renameProject(project: Project, name: String) = launchCatching {
        repo.updateProject(project, name = name)
        refreshSilently()
    }

    fun setProjectColor(project: Project, color: String) = launchCatching {
        repo.updateProject(project, color = color)
        refreshSilently()
    }

    fun setProjectIcon(project: Project, icon: String) = launchCatching {
        repo.updateProject(project, icon = icon)
        refreshSilently()
    }

    /** Whether the colour paints the badge box ("badge") or the glyph ("icon"). */
    fun setProjectIconMode(project: Project, mode: String) = launchCatching {
        repo.updateProject(project, iconMode = mode)
        refreshSilently()
    }

    /** What the sidebar tree shows under this project: "boards" | "milestones" | "both". */
    fun setProjectTreeMode(project: Project, mode: String) = launchCatching {
        repo.updateProject(project, treeMode = mode)
        refreshSilently()
        // If the project is open and now shows stages, load them lazily.
        if (project.id in _state.value.expandedProjects && mode != "boards" &&
            _state.value.milestonesByProject[project.id] == null
        ) {
            loadMilestones(project.id)
        }
    }

    /** Toggle whether this project's stages tree includes closed milestones (persisted). */
    fun toggleShowClosedStages(projectId: String) {
        val cur = _state.value.showClosedStages
        val next = if (projectId in cur) cur - projectId else cur + projectId
        _state.update { it.copy(showClosedStages = next) }
        viewModelScope.launch { prefs.setShowClosedStages(next) }
    }

    fun deleteProject(projectId: String) = launchCatching {
        repo.deleteProject(projectId)
        refreshSilently()
    }

    /** Transfers [projectId] to [targetWorkspaceId], then refreshes the tree. [onDone]
     *  receives the number of assignees stripped (non-members of the target). */
    fun transferProject(projectId: String, targetWorkspaceId: String, onDone: (Int) -> Unit) = launchCatching {
        val stripped = repo.transferProject(projectId, targetWorkspaceId)
        refreshSilently()
        onDone(stripped)
    }

    /** Two-level estimation config; null clears it (inherit). */
    fun setProjectEstimation(projectId: String, config: website.msdnna.tessera.data.model.EstimationConfig?) = launchCatching {
        repo.setProjectEstimation(projectId, config)
        refreshSilently()
    }

    fun setWorkspaceEstimation(workspaceId: String, config: website.msdnna.tessera.data.model.EstimationConfig?) = launchCatching {
        repo.setWorkspaceEstimation(workspaceId, config)
        refreshSilently()
    }

    fun renameGroup(group: ProjectGroup, name: String) = launchCatching {
        repo.updateGroup(group, name = name)
        refreshSilently()
    }

    fun setGroupColor(group: ProjectGroup, color: String) = launchCatching {
        repo.updateGroup(group, color = color)
        refreshSilently()
    }

    fun setGroupIcon(group: ProjectGroup, icon: String) = launchCatching {
        repo.updateGroup(group, icon = icon)
        refreshSilently()
    }

    /** Whether the colour paints the badge box ("badge") or the glyph ("icon"). */
    fun setGroupIconMode(group: ProjectGroup, mode: String) = launchCatching {
        repo.updateGroup(group, iconMode = mode)
        refreshSilently()
    }

    fun deleteGroup(groupId: String) = launchCatching {
        repo.deleteGroup(groupId)
        refreshSilently()
    }

    /** Reparents/reorders a group via the move endpoint, then refreshes the tree. */
    fun moveGroup(groupId: String, parentId: String?, beforeId: String?, afterId: String?) = launchCatching {
        repo.moveGroup(groupId, parentId, beforeId, afterId)
        refreshSilently()
    }

    /** Re-groups/reorders a project via the move endpoint, then refreshes the tree. */
    fun moveProject(projectId: String, groupId: String?, beforeId: String?, afterId: String?) = launchCatching {
        repo.moveProject(projectId, groupId, beforeId, afterId)
        refreshSilently()
    }

    fun addBoard(projectId: String, name: String) = launchCatching {
        repo.createBoard(projectId, name)
        loadBoards(projectId)
    }

    fun renameBoard(projectId: String, boardId: String, name: String) = launchCatching {
        repo.renameBoard(boardId, name)
        loadBoards(projectId)
    }

    /** Updates a board's name + icon/colour/mode, then refreshes the sidebar tree.
     *  [onUpdated] receives the fresh board so the caller can update local state. */
    fun updateBoard(
        projectId: String,
        boardId: String,
        name: String,
        icon: String?,
        color: String?,
        iconMode: String?,
        onUpdated: (Board) -> Unit = {},
    ) = launchCatching {
        val updated = repo.updateBoardMeta(boardId, name, icon, color, iconMode)
        loadBoards(projectId)
        onUpdated(updated)
    }

    fun deleteBoard(projectId: String, boardId: String) = launchCatching {
        repo.deleteBoard(boardId)
        loadBoards(projectId)
    }

    fun clearError() = _state.update { it.copy(error = null) }

    private suspend fun refreshSilently() {
        val id = _state.value.currentId
        if (id.isBlank()) return
        val groups = repo.groups(id)
        val projects = repo.projects(id)
        _state.update { it.copy(groups = groups, projects = projects) }
    }

    private fun launchCatching(block: suspend () -> Unit) {
        viewModelScope.launch {
            val result = runCatching { block() }
            result.exceptionOrNull()?.let { e ->
                _state.update { it.copy(loading = false, error = errorMessage(e)) }
            }
        }
    }
}
