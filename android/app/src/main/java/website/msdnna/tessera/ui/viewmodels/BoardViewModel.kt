package website.msdnna.tessera.ui.viewmodels

import android.os.SystemClock
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.google.gson.Gson
import java.util.Calendar
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.model.BoardColumn
import website.msdnna.tessera.data.model.BoardView
import website.msdnna.tessera.data.model.BoardViewConfig
import website.msdnna.tessera.data.model.BoardViewFilters
import website.msdnna.tessera.data.model.Member
import website.msdnna.tessera.data.model.SortLevel
import website.msdnna.tessera.data.model.Tag
import website.msdnna.tessera.data.model.Task
import website.msdnna.tessera.data.realtime.RealtimeClient
import website.msdnna.tessera.data.realtime.RealtimeEvent
import website.msdnna.tessera.data.repository.BoardRepository
import website.msdnna.tessera.util.errorMessage
import website.msdnna.tessera.util.isoDateKey

enum class BoardViewMode { Kanban, List, Calendar }

enum class DueFilter { All, Overdue, Today, Week, Has, None }

/** Sort fields available in the multi-level sort editor (web parity). */
enum class SortField(val key: String, val label: String) {
    Priority("priority", "Приоритет"),
    Due("due", "Срок"),
    Title("title", "Название"),
    Number("number", "Номер"),
    ;

    companion object {
        fun fromKey(key: String): SortField? = entries.firstOrNull { it.key == key }
    }
}

/** Client-side board filter (web KanbanBoard filters). Empty = show everything. */
data class BoardFilter(
    val query: String = "",
    val priorities: Set<Int> = emptySet(),
    val tagIds: Set<String> = emptySet(),
    val assigneeIds: Set<String> = emptySet(),
    val due: DueFilter = DueFilter.All,
) {
    val isActive: Boolean
        get() = query.isNotBlank() || priorities.isNotEmpty() || tagIds.isNotEmpty() ||
            assigneeIds.isNotEmpty() || due != DueFilter.All
}

private val TagPalette = listOf(
    "#7c5cff", "#2f80ed", "#0eb0a9", "#18a058", "#f0a020", "#e0533d", "#eb2f96",
)

data class BoardUiState(
    val loading: Boolean = true,
    val error: String? = null,
    val columns: List<BoardColumn> = emptyList(),
    val tasks: List<Task> = emptyList(),
    val subtasks: List<Task> = emptyList(),
    val tags: Map<String, Tag> = emptyMap(),
    val tagList: List<Tag> = emptyList(),
    val members: List<Member> = emptyList(),
    val viewMode: BoardViewMode = BoardViewMode.Kanban,
    val groupByTag: Boolean = false,
    /** When grouping by tag, only tags carrying this namespace prefix ("S: ",
     *  "effort::") become columns. Empty = every tag is a column. */
    val tagPrefix: String = "",
    val subtasksExpanded: Boolean = false,
    val doneColumnId: String? = null,
    val filter: BoardFilter = BoardFilter(),
    /** Ordered multi-level sort. Empty = manual (position) order. */
    val sortLevels: List<SortLevel> = emptyList(),
    /** Saved server-side views for this board (per-user). */
    val savedViews: List<BoardView> = emptyList(),
    /** Name of the last applied/saved view, or null. */
    val currentViewName: String? = null,
    /** Archived cards for the Archive modal (null = not yet loaded). */
    val archived: List<Task>? = null,
    /** True while a pull-to-refresh is in flight (no full-screen spinner). */
    val refreshing: Boolean = false,
) {
    val membersMap: Map<String, Member> get() = members.associateBy { it.userId }

    /** Full column list, drag-position source — NOT filtered (DnD needs every card). */
    fun tasksIn(columnId: String): List<Task> =
        tasks.filter { it.columnId == columnId }.sortedBy { it.position }

    /** Display list for a column: the active filter + sort applied. */
    fun visibleTasksIn(columnId: String): List<Task> = applyFilterSort(tasksIn(columnId))

    /** Applies the active [filter] then the multi-level [sortLevels]. Used by every view. */
    fun applyFilterSort(list: List<Task>): List<Task> {
        val today = dateKey(0)
        val weekEnd = dateKey(DAYS_IN_WEEK)
        val filtered = list.filter { t ->
            val q = filter.query.trim().lowercase()
            val matchesQuery = q.isEmpty() || t.title.lowercase().contains(q) ||
                t.number?.let { "#$it".contains(q) } == true
            val matchesPriority = filter.priorities.isEmpty() || t.priority in filter.priorities
            val matchesTags = filter.tagIds.isEmpty() || t.tagIds.any { it in filter.tagIds }
            val matchesAssignees = filter.assigneeIds.isEmpty() || t.assigneeIds.any { it in filter.assigneeIds }
            val due = isoDateKey(t.dueDate)
            val matchesDue = when (filter.due) {
                DueFilter.All -> true
                DueFilter.Has -> due.isNotEmpty()
                DueFilter.None -> due.isEmpty()
                DueFilter.Overdue -> due.isNotEmpty() && due < today && !t.isCompleted
                DueFilter.Today -> due == today
                DueFilter.Week -> due.isNotEmpty() && due >= today && due < weekEnd
            }
            matchesQuery && matchesPriority && matchesTags && matchesAssignees && matchesDue
        }
        if (sortLevels.isEmpty()) return filtered.sortedBy { it.position }
        val comparator = sortLevels
            .map { levelComparator(it) }
            .reduce { acc, next -> acc.then(next) }
        return filtered.sortedWith(comparator.thenBy { it.position })
    }

    /** One sort level → a [Comparator] (web `cmpLevel`: due-less tasks always last). */
    private fun levelComparator(level: SortLevel): Comparator<Task> {
        val d = if (level.dir == "desc") -1 else 1
        return Comparator { a, b ->
            when (level.field) {
                "due" -> {
                    val av = isoDateKey(a.dueDate).ifEmpty { null }
                    val bv = isoDateKey(b.dueDate).ifEmpty { null }
                    when {
                        av == null && bv == null -> 0
                        av == null -> 1
                        bv == null -> -1
                        else -> d * av.compareTo(bv)
                    }
                }

                "priority" -> d * a.priority.compareTo(b.priority)

                "title" -> d * a.title.compareTo(b.title, ignoreCase = true)

                "number" -> d * (a.number ?: 0L).compareTo(b.number ?: 0L)

                else -> 0
            }
        }
    }

    fun subtasksOf(parentId: String): List<Task> =
        subtasks.filter { it.parentId == parentId }.sortedBy { it.position }

    fun subtaskCount(parentId: String): Int = subtasks.count { it.parentId == parentId }

    val sortedColumns: List<BoardColumn> get() = columns.sortedBy { it.position }
}

/** Loads a board (columns + cards + subtasks + tags + members) and performs all
 *  card mutations. One instance per opened board. */
class BoardViewModel(
    private val repo: BoardRepository = BoardRepository(),
) : ViewModel() {
    private val _state = MutableStateFlow(BoardUiState())
    val state: StateFlow<BoardUiState> = _state.asStateFlow()

    private val gson = Gson()
    private var boardId: String = ""
    private var workspaceId: String = ""
    private var projectId: String = "" // the board's project — scopes tags

    // Realtime: a live socket reloads the board on workspace-scoped events. A
    // suppress window after our own mutations avoids a redundant echo reload;
    // [dragging] pauses reloads so a drag in progress isn't yanked out from under.
    private var realtime: RealtimeClient? = null
    private var reloadJob: Job? = null
    private var suppressUntil = 0L

    @Volatile var dragging = false

    fun load(boardId: String, workspaceId: String) {
        this.boardId = boardId
        this.workspaceId = workspaceId
        ensureRealtime()
        _state.update { it.copy(loading = true, error = null) }
        launchCatching {
            val cfg = parseView(runCatching { AppContainer.prefs.boardViewJson(boardId) }.getOrDefault(""))
            val board = runCatching { repo.board(boardId) }.getOrNull()
            projectId = board?.projectId ?: ""
            val columns = repo.columns(boardId)
            val tasks = repo.tasks(boardId)
            val subtasks = repo.subtasks(boardId)
            val tags = if (projectId.isNotBlank()) runCatching { repo.tags(projectId) }.getOrDefault(emptyList()) else emptyList()
            val members = if (workspaceId.isNotBlank()) runCatching { repo.members(workspaceId) }.getOrDefault(emptyList()) else emptyList()
            _state.update {
                val base = it.copy(
                    loading = false,
                    doneColumnId = board?.doneColumnId,
                    columns = columns,
                    tasks = tasks,
                    subtasks = subtasks,
                    tags = tags.associateBy { t -> t.id },
                    tagList = tags,
                    members = members,
                )
                if (cfg != null) base.applyConfig(cfg) else base
            }
            loadViews()
        }
    }

    /** Loads this board's saved server-side views (tolerant of failure). */
    private suspend fun loadViews() {
        val views = runCatching { repo.views(boardId) }.getOrDefault(emptyList())
        _state.update { it.copy(savedViews = views) }
    }

    /**
     * Reload after a modal edit. Silent (no `loading` flag) on purpose: flipping
     * `loading` swaps the kanban for the full-screen loader, which tears down the
     * columns Row and resets its horizontal scroll to the first column. The silent
     * path refreshes data in place so the open/selected column stays put.
     */
    fun reload() {
        if (boardId.isBlank()) return
        viewModelScope.launch { runCatching { silentReload() } }
    }

    /** Pull-to-refresh: reloads board data without the full-screen spinner. */
    fun pullRefresh() {
        if (boardId.isBlank()) return
        _state.update { it.copy(refreshing = true) }
        viewModelScope.launch {
            runCatching { silentReload() }
            _state.update { it.copy(refreshing = false) }
        }
    }

    // ── realtime ─────────────────────────────────────────────────────────────

    private fun ensureRealtime() {
        if (realtime != null) return
        realtime = RealtimeClient(::onRealtimeEvent).also { it.connect() }
    }

    /** Workspace-scoped board events reload the board; our own echoes and
     *  in-progress drags are filtered out. Debounced to coalesce bursts. */
    private fun onRealtimeEvent(ev: RealtimeEvent) {
        if (ev.scope != workspaceId) return
        if (!ev.type.startsWith("task") && !ev.type.startsWith("column") && !ev.type.startsWith("board")) return
        if (dragging || SystemClock.elapsedRealtime() < suppressUntil) return
        reloadJob?.cancel()
        reloadJob = viewModelScope.launch {
            delay(REALTIME_DEBOUNCE_MS)
            if (!dragging) silentReload()
        }
    }

    /** Refreshes board data without the loading spinner (for live updates). */
    private suspend fun silentReload() {
        if (boardId.isBlank()) return
        val board = runCatching { repo.board(boardId) }.getOrNull()
        projectId = board?.projectId ?: projectId
        val columns = repo.columns(boardId)
        val tasks = repo.tasks(boardId)
        val subtasks = repo.subtasks(boardId)
        val tags = if (projectId.isNotBlank()) runCatching { repo.tags(projectId) }.getOrDefault(emptyList()) else emptyList()
        _state.update {
            it.copy(
                doneColumnId = board?.doneColumnId,
                columns = columns,
                tasks = tasks,
                subtasks = subtasks,
                tags = tags.associateBy { t -> t.id },
                tagList = tags,
            )
        }
    }

    /** Marks a window during which incoming echoes of our own change are ignored. */
    private fun markLocalChange() {
        suppressUntil = SystemClock.elapsedRealtime() + SUPPRESS_MS
    }

    override fun onCleared() {
        realtime?.close()
        realtime = null
    }

    fun renameColumn(column: BoardColumn, name: String) = launchCatching {
        repo.renameColumn(column, name)
        reloadColumns()
    }

    fun setColumnColor(column: BoardColumn, color: String) = launchCatching {
        repo.setColumnColor(column, color)
        reloadColumns()
    }

    /** Toggles which column auto-completes tasks (passing the current done id clears it). */
    fun setDoneColumn(columnId: String?) = launchCatching {
        repo.setDoneColumn(boardId, columnId)
        val board = runCatching { repo.board(boardId) }.getOrNull()
        _state.update { it.copy(doneColumnId = board?.doneColumnId) }
        markLocalChange()
    }

    fun createColumn(name: String) = launchCatching {
        repo.createColumn(boardId, name)
        reloadColumns()
    }

    fun deleteColumn(columnId: String) = launchCatching {
        repo.deleteColumn(columnId)
        reloadColumns()
    }

    fun moveColumn(columnId: String, beforeId: String?, afterId: String?) = launchCatching {
        repo.moveColumn(columnId, beforeId, afterId)
        reloadColumns()
    }

    private suspend fun reloadColumns() {
        _state.update { it.copy(columns = repo.columns(boardId), tasks = repo.tasks(boardId), subtasks = repo.subtasks(boardId)) }
        markLocalChange()
    }

    fun setViewMode(mode: BoardViewMode) {
        _state.update { it.copy(viewMode = mode) }
        persistView()
    }

    /** Sets the grouping: status, all-tags, or a tag namespace (e.g. "S: "). */
    fun setGrouping(byTag: Boolean, prefix: String = "") {
        _state.update { it.copy(groupByTag = byTag, tagPrefix = if (byTag) prefix else "") }
        persistView()
    }

    fun toggleSubtasksExpanded() {
        _state.update { it.copy(subtasksExpanded = !it.subtasksExpanded) }
        persistView()
    }
    fun clearError() = _state.update { it.copy(error = null) }

    // ── filters / sort / saved views ─────────────────────────────────────────

    fun setFilter(filter: BoardFilter) {
        _state.update { it.copy(filter = filter) }
        persistView()
    }

    fun clearFilter() {
        _state.update { it.copy(filter = BoardFilter()) }
        persistView()
    }

    /** Appends a sort level for [field] if not already present (default ascending). */
    fun addSortLevel(field: SortField) {
        _state.update {
            if (it.sortLevels.any { l -> l.field == field.key }) it
            else it.copy(sortLevels = it.sortLevels + SortLevel(field.key, "asc"))
        }
        persistView()
    }

    /** Flips a sort level between ascending and descending. */
    fun toggleSortDir(index: Int) {
        _state.update {
            val levels = it.sortLevels.toMutableList()
            levels.getOrNull(index)?.let { l ->
                levels[index] = l.copy(dir = if (l.dir == "desc") "asc" else "desc")
            }
            it.copy(sortLevels = levels)
        }
        persistView()
    }

    fun removeSortLevel(index: Int) {
        _state.update {
            it.copy(sortLevels = it.sortLevels.filterIndexed { i, _ -> i != index })
        }
        persistView()
    }

    fun clearSort() {
        _state.update { it.copy(sortLevels = emptyList()) }
        persistView()
    }

    // ── saved views (server-side) ────────────────────────────────────────────

    /** Saves (upserts) the current toolbar state as a named server-side view. */
    fun saveView(name: String) {
        val trimmed = name.trim()
        if (trimmed.isBlank() || boardId.isBlank()) return
        launchCatching {
            repo.saveView(boardId, trimmed, configFromState(_state.value))
            _state.update { it.copy(currentViewName = trimmed) }
            loadViews()
        }
    }

    /** Applies a saved view's config to the toolbar and persists it locally. */
    fun applyView(view: BoardView) {
        _state.update { it.applyConfig(view.config).copy(currentViewName = view.name) }
        persistView()
    }

    fun deleteView(view: BoardView) = launchCatching {
        repo.deleteView(view.id)
        _state.update {
            it.copy(currentViewName = if (it.currentViewName == view.name) null else it.currentViewName)
        }
        loadViews()
    }

    /** Serialises the current view (filter/sort/group/layout) to the per-board DataStore key. */
    private fun persistView() {
        if (boardId.isBlank()) return
        val json = gson.toJson(configFromState(_state.value))
        viewModelScope.launch { runCatching { AppContainer.prefs.setBoardViewJson(boardId, json) } }
    }

    private fun parseView(json: String): BoardViewConfig? =
        if (json.isBlank()) null else runCatching { gson.fromJson(json, BoardViewConfig::class.java) }.getOrNull()

    // ── tag management (TagManager modal) ──────────────────────────────────────

    fun updateTag(tagId: String, name: String, color: String) = launchCatching {
        repo.updateTag(tagId, name, color)
        refreshTagsAndTasks()
    }

    fun deleteTag(tagId: String) = launchCatching {
        repo.deleteTag(tagId)
        refreshTagsAndTasks()
    }

    fun createTagStandalone(name: String, color: String) = launchCatching {
        repo.createTag(projectId, name, color)
        refreshTagsAndTasks()
    }

    private suspend fun refreshTagsAndTasks() {
        val tags = repo.tags(projectId)
        _state.update { it.copy(tags = tags.associateBy { t -> t.id }, tagList = tags, tasks = repo.tasks(boardId)) }
        markLocalChange()
    }

    // ── archive ────────────────────────────────────────────────────────────────

    fun loadArchived() = launchCatching {
        _state.update { it.copy(archived = repo.archived(boardId)) }
    }

    fun restoreFromArchive(taskId: String) = launchCatching {
        repo.restoreTask(taskId)
        _state.update { it.copy(archived = repo.archived(boardId)) }
        refreshTasks()
    }

    fun deleteFromArchive(taskId: String) = launchCatching {
        repo.deleteTask(taskId)
        _state.update { it.copy(archived = repo.archived(boardId)) }
    }

    fun createTask(columnId: String, title: String, parentId: String? = null) = launchCatching {
        repo.createTask(boardId, columnId, title, parentId)
        refreshTasks()
    }

    /** Moves a card to an explicit slot (DnD) — or to a column's end (menu). */
    fun moveTask(taskId: String, columnId: String, beforeId: String?, afterId: String?) = launchCatching {
        repo.moveTask(taskId, columnId, beforeId, afterId)
        refreshTasks()
    }

    /** Resolves a drag drop: a subtask is detached to top-level first, then the
     *  task is positioned in the target column between before/after. */
    fun dropTask(task: Task, columnId: String, beforeId: String?, afterId: String?) = launchCatching {
        if (task.parentId != null) repo.setParent(task.id, null)
        repo.moveTask(task.id, columnId, beforeId, afterId)
        refreshTasks()
    }

    /** Drag-drop a task onto/into a card to make it that card's subtask,
     *  optionally positioned between sibling before/after (else appended). */
    fun nestTask(task: Task, parentId: String, beforeId: String? = null, afterId: String? = null) {
        if (task.id == parentId) return
        // Drop onto the body of the task's current parent with no slot = no-op.
        if (task.parentId == parentId && beforeId == null && afterId == null) return
        launchCatching {
            if (task.parentId != parentId) repo.setParent(task.id, parentId)
            if (beforeId != null || afterId != null) {
                val parentColumn = (state.value.tasks + state.value.subtasks)
                    .firstOrNull { it.id == parentId }?.columnId
                if (parentColumn != null) repo.moveTask(task.id, parentColumn, beforeId, afterId)
            }
            refreshTasks()
        }
    }

    fun moveToColumnEnd(task: Task, targetColumnId: String) {
        if (task.columnId == targetColumnId) return
        launchCatching {
            val last = _state.value.tasksIn(targetColumnId).lastOrNull { it.id != task.id }
            repo.moveTask(task.id, targetColumnId, afterId = last?.id)
            refreshTasks()
        }
    }

    fun setParent(taskId: String, parentId: String?) = launchCatching {
        repo.setParent(taskId, parentId)
        refreshTasks()
    }

    fun toggleDone(task: Task) = launchCatching {
        repo.updateTask(task, completed = !task.isCompleted)
        refreshTasks()
    }

    fun setPriority(task: Task, priority: Int) = launchCatching {
        repo.updateTask(task, priority = priority)
        refreshTasks()
    }

    fun setDue(task: Task, dueIso: String?) = launchCatching {
        repo.updateTask(task, dueDate = dueIso)
        refreshTasks()
    }

    fun renameTask(task: Task, title: String) = launchCatching {
        repo.updateTask(task, title = title)
        refreshTasks()
    }

    fun toggleTag(task: Task, tagId: String) = launchCatching {
        if (tagId in task.tagIds) repo.removeTag(task.id, tagId) else repo.addTag(task.id, tagId)
        refreshTasks()
    }

    fun createTagAndAdd(task: Task, name: String) = launchCatching {
        val color = TagPalette.random()
        val tag = repo.createTag(projectId, name, color)
        repo.addTag(task.id, tag.id)
        val tags = repo.tags(projectId)
        _state.update { it.copy(tags = tags.associateBy { t -> t.id }, tagList = tags) }
        refreshTasks()
    }

    fun toggleAssignee(task: Task, userId: String) = launchCatching {
        if (userId in task.assigneeIds) repo.removeAssignee(task.id, userId) else repo.addAssignee(task.id, userId)
        refreshTasks()
    }

    fun archive(taskId: String) = launchCatching {
        repo.archiveTask(taskId)
        refreshTasks()
    }

    fun delete(taskId: String) = launchCatching {
        repo.deleteTask(taskId)
        refreshTasks()
    }

    private suspend fun refreshTasks() {
        val tasks = repo.tasks(boardId)
        val subtasks = repo.subtasks(boardId)
        _state.update { it.copy(tasks = tasks, subtasks = subtasks) }
        markLocalChange()
    }

    private fun launchCatching(block: suspend () -> Unit) {
        viewModelScope.launch {
            val result = runCatching { block() }
            result.exceptionOrNull()?.let { e ->
                _state.update { it.copy(loading = false, error = errorMessage(e)) }
            }
        }
    }

    private companion object {
        const val REALTIME_DEBOUNCE_MS = 300L
        const val SUPPRESS_MS = 1500L
    }
}

private const val DAYS_IN_WEEK = 7

/** A local `yyyy-MM-dd` key for today + [days], matching due-date keys. */
private fun dateKey(days: Int): String {
    val cal = Calendar.getInstance().apply { add(Calendar.DAY_OF_MONTH, days) }
    val y = cal.get(Calendar.YEAR)
    val m = (cal.get(Calendar.MONTH) + 1).toString().padStart(2, '0')
    val d = cal.get(Calendar.DAY_OF_MONTH).toString().padStart(2, '0')
    return "$y-$m-$d"
}

// ── view config ↔ board state (web schema; camelCase keys for cross-device) ──

/** Snapshots the toolbar state into a [BoardViewConfig] (web-compatible). */
private fun configFromState(s: BoardUiState): BoardViewConfig = BoardViewConfig(
    layout = when (s.viewMode) {
        BoardViewMode.List -> "list"
        BoardViewMode.Calendar -> "calendar"
        BoardViewMode.Kanban -> "board"
    },
    groupMode = if (s.groupByTag) "tag" else "status",
    tagPrefix = s.tagPrefix,
    sortLevels = s.sortLevels,
    subtasksExpanded = s.subtasksExpanded,
    filters = BoardViewFilters(
        priorities = s.filter.priorities.toList(),
        assignees = s.filter.assigneeIds.toList(),
        tags = s.filter.tagIds.toList(),
        due = dueToWeb(s.filter.due),
        q = s.filter.query,
    ),
)

/** Applies a [BoardViewConfig] onto the toolbar state (group/sort/filter/layout). */
private fun BoardUiState.applyConfig(c: BoardViewConfig): BoardUiState = copy(
    viewMode = when (c.layout) {
        "list" -> BoardViewMode.List
        "calendar" -> BoardViewMode.Calendar
        else -> BoardViewMode.Kanban
    },
    groupByTag = c.groupMode == "tag",
    tagPrefix = if (c.groupMode == "tag") c.tagPrefix else "",
    sortLevels = c.sortLevels,
    subtasksExpanded = c.subtasksExpanded,
    filter = BoardFilter(
        query = c.filters.q,
        priorities = c.filters.priorities.toSet(),
        tagIds = c.filters.tags.toSet(),
        assigneeIds = c.filters.assignees.toSet(),
        due = dueFromWeb(c.filters.due),
    ),
)

private fun dueToWeb(due: DueFilter): String = when (due) {
    DueFilter.All -> ""
    DueFilter.Overdue -> "overdue"
    DueFilter.Today -> "today"
    DueFilter.Week -> "week"
    DueFilter.Has -> "has"
    DueFilter.None -> "none"
}

private fun dueFromWeb(due: String): DueFilter = when (due) {
    "overdue" -> DueFilter.Overdue
    "today" -> DueFilter.Today
    "week" -> DueFilter.Week
    "has" -> DueFilter.Has
    "none" -> DueFilter.None
    else -> DueFilter.All
}
