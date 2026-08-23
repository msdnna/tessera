package website.msdnna.tessera.ui.viewmodels

import android.os.SystemClock
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.google.gson.Gson
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
import website.msdnna.tessera.data.model.Recurrence
import website.msdnna.tessera.data.model.SortLevel
import website.msdnna.tessera.data.model.Tag
import website.msdnna.tessera.data.model.Task
import website.msdnna.tessera.data.realtime.RealtimeClient
import website.msdnna.tessera.data.realtime.RealtimeEvent
import website.msdnna.tessera.data.repository.BoardRepository
import website.msdnna.tessera.ui.UiText
import website.msdnna.tessera.util.BoardFilter
import website.msdnna.tessera.util.DueFilter
import website.msdnna.tessera.util.FilterClock
import website.msdnna.tessera.util.errorMessage
import website.msdnna.tessera.util.filterBoardTasks
import website.msdnna.tessera.util.isoDateKey
import website.msdnna.tessera.util.moveItem

enum class BoardViewMode { Kanban, List, Calendar, Matrix, Timeline, Gantt }

/** Sort fields available in the multi-level sort editor (web parity). */
enum class SortField(val key: String, val label: String) {
    Priority("priority", "Приоритет"),
    Due("due", "Срок"),
    Status("status", "Статус"),
    Title("title", "Название"),
    Number("number", "Номер"),
    ;

    companion object {
        fun fromKey(key: String): SortField? = entries.firstOrNull { it.key == key }
    }
}

private val TagPalette = listOf(
    "#7c5cff", "#2f80ed", "#0eb0a9", "#18a058", "#f0a020", "#e0533d", "#eb2f96",
)

/** A transient board-activity toast (mirrors the web BoardActivityToasts): who did
 *  what on the currently-open board. Not persisted — pure live activity. */
data class BoardActivity(
    val key: Long,
    val taskId: String,
    val number: Long?,
    val title: String,
    val verb: String, // created | moved | completed | reopened
    val actorName: String,
    val self: Boolean,
)

data class BoardUiState(
    val loading: Boolean = true,
    val error: UiText? = null,
    val columns: List<BoardColumn> = emptyList(),
    val tasks: List<Task> = emptyList(),
    val subtasks: List<Task> = emptyList(),
    /** Whole-board blocking edges, drawn as arrows on the Gantt view. */
    val dependencies: List<website.msdnna.tessera.data.model.BoardDependency> = emptyList(),
    val tags: Map<String, Tag> = emptyMap(),
    val tagList: List<Tag> = emptyList(),
    /** Canonical tag-prefix → friendly label (project-scoped). Drives the grouped
     *  group/filter menus and tag pickers; empty when none are configured. */
    val prefixNames: Map<String, String> = emptyMap(),
    /** Canonical prefixes of GitLab label rules mapping to non-tag actions (status /
     *  priority / …) on this board's integrations — hidden from the ADD tag-picker. */
    val metaTagPrefixes: Set<String> = emptySet(),
    /** Resolved estimation unit config for this board's project (drives the
     *  estimate input/chip/aggregates). Falls back to the built-in default. */
    val estimation: website.msdnna.tessera.data.model.EstimationConfig =
        website.msdnna.tessera.util.Estimation.DEFAULT,
    val members: List<Member> = emptyList(),
    val gitlabMembers: List<website.msdnna.tessera.data.model.GitlabMember> = emptyList(),
    /** The backend's built-in quick actions, verbatim: a command added in Go shows
     *  up in the editor popup without an Android change. */
    val builtinCommands: List<website.msdnna.tessera.data.model.CommandDef> = emptyList(),
    /** The workspace's custom command dictionary — suggested, never executed. */
    val customCommands: List<website.msdnna.tessera.data.model.WorkspaceCommand> = emptyList(),
    /** The caller can edit the dictionary (owner/admin); gates the editor entry. */
    val canManageCommands: Boolean = false,
    /** This board's project milestones («Этапы»), for the card chip, the milestone
     *  grouping and the picker. Empty when the project has none. */
    val milestones: List<website.msdnna.tessera.data.model.Milestone> = emptyList(),
    val viewMode: BoardViewMode = BoardViewMode.Kanban,
    /** Swimlane/column grouping: "status" | "tag" | "assignee" | "none". Kanban only
     *  honours status/tag (columns); assignee/none are timeline/Gantt-only (mirrors web). */
    val groupMode: String = "status",
    /** When grouping by tag, only tags carrying this namespace prefix ("S: ",
     *  "effort::") become columns. Empty = every tag is a column. */
    val tagPrefix: String = "",
    val subtasksExpanded: Boolean = false,
    /** "Авто" (Gantt only): order rows by the blocking-dependency graph instead of
     *  the list order. Only takes effect via [autoActive] (no grouping/sort). */
    val autoSort: Boolean = false,
    /** Per-column collapse override (web colCollapse): laneId → true(collapsed)/
     *  false(expanded). Absence = follow the auto rule. */
    val colCollapse: Map<String, Boolean> = emptyMap(),
    /** Auto-collapse empty columns into a strip unless overridden (web autoCollapseEmpty). */
    val autoCollapseEmpty: Boolean = false,
    /** Card density: "compact" | "medium" | "large" (web cardSize). */
    val cardSize: String = "medium",
    /** Pills stacked vertically instead of inline-wrapped (web stackFields). */
    val stackFields: Boolean = false,
    /** Show empty priority/due/tags/assignee as add-affordances (web showEmpty). */
    val showEmpty: Boolean = true,
    /** Per-field visibility overrides (web fieldVis): fieldKey → true/false, missing = visible. */
    val fieldVis: Map<String, Boolean> = emptyMap(),
    /** Auto-persist the current named view on every change (web autosaveView). */
    val autosaveView: Boolean = false,
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
    /** Read-only archive scope: when true, [tasks] holds the board's ARCHIVED tasks
     *  (rendered through the normal views with all filters/grouping/sort, but no
     *  mutations — only restore). Web parity (archive-as-board). */
    val archivedMode: Boolean = false,
    /** Server-side sprint scope: <milestone uuid> shows one sprint, "backlog" shows
     *  milestone-less tasks, null shows all. For large GitLab imports (web parity). */
    val milestoneScope: String? = null,
    /** True while a pull-to-refresh is in flight (no full-screen spinner). */
    val refreshing: Boolean = false,
) {
    val membersMap: Map<String, Member> get() = members.associateBy { it.userId }

    /** Kanban grouping is binary (status vs tag columns); derived from [groupMode]. */
    val groupByTag: Boolean get() = groupMode == "tag"

    /** Kanban grouping by milestone («По этапам») — one column per milestone + «Без этапа». */
    val groupByMilestone: Boolean get() = groupMode == "milestone"

    val milestonesMap: Map<String, website.msdnna.tessera.data.model.Milestone>
        get() = milestones.associateBy { it.id }

    /** "Авто" is only honoured on the Gantt with no grouping/sort, so any manual
     *  grouping or sort transparently turns it off (web `autoActive` parity). */
    val autoActive: Boolean
        get() = viewMode == BoardViewMode.Gantt && autoSort && groupMode == "none" && sortLevels.isEmpty()

    /** Effective collapsed state of a lane (web isColCollapsed): an explicit
     *  [colCollapse] override wins, otherwise auto-collapse applies to empty lanes. */
    fun isLaneCollapsed(laneId: String, count: Int): Boolean =
        colCollapse[laneId] ?: (autoCollapseEmpty && count == 0)

    /** Card density: compact renders the title only (no pills). */
    val isCompactCard: Boolean get() = cardSize == "compact"

    /** A field's own visibility toggle (web fieldVis): missing key = visible. */
    fun fieldOn(key: String): Boolean = fieldVis[key] != false

    /** Whether [key] renders on a card at the current density AND its visibility
     *  toggle (web `show(k) = sizeAllows(k) && fv(k)`). compact = none, medium =
     *  the key-field subset, large = all. */
    fun cardShows(key: String): Boolean {
        val sizeAllows = when (cardSize) {
            "compact" -> false
            "medium" -> key in MEDIUM_CARD_FIELDS
            else -> true // large (and any unknown) = all fields
        }
        return sizeAllows && fieldOn(key)
    }

    /** `/`-popup rows for the comment editor: built-ins first (registry order = popup
     *  order), the custom dictionary after (web `commandItems`). */
    val commandRows: List<website.msdnna.tessera.util.CommandItem>
        get() = website.msdnna.tessera.util.commandItems(builtinCommands, customCommands)

    /** Tessera user id → their GitLab login, for the reverse direction: a GitLab-synced
     *  task has no `created_by`, so matching its author against a Tessera person goes
     *  through this map (see util `matchesAuthor`). */
    val glLoginByUserId: Map<String, String>
        get() = gitlabMembers.mapNotNull { g ->
            g.tesseraUserId?.takeIf { g.glUsername.isNotBlank() }?.let { it to g.glUsername }
        }.toMap()

    /** Whole-board filter pass (tasks + subtasks together, see util `filterBoardTasks`).
     *  Computed once per state instance — [applyFilterSort] runs per column/lane. */
    private val filteredBoard: website.msdnna.tessera.util.FilteredBoard by lazy {
        if (!filter.isActive) {
            website.msdnna.tessera.util.FilteredBoard(
                visibleIds = tasks.mapTo(mutableSetOf()) { it.id },
                subtasksByParent = subtasks.groupBy { it.parentId.orEmpty() },
            )
        } else {
            filterBoardTasks(
                tasks = tasks,
                subtasksByParent = subtasks.groupBy { it.parentId.orEmpty() },
                filter = filter,
                clock = FilterClock.now(),
                glLoginByUserId = glLoginByUserId,
            )
        }
    }

    /** Full column list, drag-position source — NOT filtered (DnD needs every card). */
    fun tasksIn(columnId: String): List<Task> =
        tasks.filter { it.columnId == columnId }.sortedBy { it.position }

    /** Display list for a column: the active filter + sort applied. */
    fun visibleTasksIn(columnId: String): List<Task> = applyFilterSort(tasksIn(columnId))

    /** Applies the active [filter] then the multi-level [sortLevels]. Used by every view. */
    fun applyFilterSort(list: List<Task>): List<Task> {
        // Every caller passes a subset of [tasks], so membership in the precomputed
        // visible set is the whole filter — including parents lifted by a child.
        val filtered = if (filter.isActive) list.filter { it.id in filteredBoard.visibleIds } else list
        if (sortLevels.isEmpty()) return filtered.sortedBy { it.position }
        val comparator = sortLevels
            .map { levelComparator(it) }
            .reduce { acc, next -> acc.then(next) }
        return filtered.sortedWith(comparator.thenBy { it.position })
    }

    /** One sort level → a [Comparator] (web `cmpLevel`: due-less tasks always last). */
    private fun levelComparator(level: SortLevel): Comparator<Task> {
        val d = if (level.dir == "desc") -1 else 1
        val colPos = columns.associate { it.id to it.position }
        return Comparator { a, b ->
            when (level.field) {
                "status" -> d * (colPos[a.columnId] ?: 0.0).compareTo(colPos[b.columnId] ?: 0.0)

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

    /** Subtasks as drawn on the card: narrowed to the matching children when the parent
     *  only stayed on the board because one of them matched (web parity). The task modal
     *  keeps using [subtasksOf] and always lists every subtask. */
    fun visibleSubtasksOf(parentId: String): List<Task> =
        if (!filter.isActive) {
            subtasksOf(parentId)
        } else {
            filteredBoard.subtasksByParent[parentId].orEmpty().sortedBy { it.position }
        }

    /** True when the filter hid part of this parent's children — the card shows an
     *  «N из M» hint and locks subtask drag-reorder. */
    fun isSubtasksNarrowed(parentId: String): Boolean =
        filter.isActive && parentId in filteredBoard.narrowedParents

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

    // Live board-activity toasts (separate from the bell): fed from realtime events.
    private val _activity = MutableStateFlow<List<BoardActivity>>(emptyList())
    val activity: StateFlow<List<BoardActivity>> = _activity.asStateFlow()

    // Set when this board's project is deleted/transferred elsewhere (realtime
    // project.deleted) — the host leaves for Home (web navigate-home-on-delete).
    private val _gone = MutableStateFlow(false)
    val gone: StateFlow<Boolean> = _gone.asStateFlow()
    private var activitySeq = 0L

    private val gson = Gson()
    private var boardId: String = ""
    private var workspaceId: String = ""
    private var projectId: String = "" // the board's project — scopes tags

    // Current user id, tracked to flag own actions in the activity feed.
    private var currentUserId: String = ""

    init {
        viewModelScope.launch {
            AppContainer.prefs.user.collect { currentUserId = it?.id ?: "" }
        }
    }

    // Realtime: a live socket reloads the board on workspace-scoped events. A
    // suppress window after our own mutations avoids a redundant echo reload;
    // [dragging] pauses reloads so a drag in progress isn't yanked out from under.
    private var realtime: RealtimeClient? = null
    private var reloadJob: Job? = null
    private var suppressUntil = 0L

    // Sliding-window burst guard for activity toasts (anti-flood on sync).
    private var activityWindowStart = 0L
    private var activityWindowCount = 0

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
            val tasks = scopedTasks()
            val subtasks = scopedSubtasks()
            val dependencies = runCatching { repo.dependencies(boardId) }.getOrDefault(emptyList())
            val tags = if (projectId.isNotBlank()) runCatching { repo.tags(projectId) }.getOrDefault(emptyList()) else emptyList()
            val milestones = if (projectId.isNotBlank()) runCatching { repo.milestones(projectId) }.getOrDefault(emptyList()) else emptyList()
            val prefixNames = loadPrefixNames()
            val estimation = loadEstimation()
            val members = if (workspaceId.isNotBlank()) runCatching { repo.members(workspaceId) }.getOrDefault(emptyList()) else emptyList()
            val gitlabMembers = if (workspaceId.isNotBlank()) repo.gitlabMembers(workspaceId) else emptyList()
            val metaTagPrefixes = loadMetaTagPrefixes()
            val registry = if (workspaceId.isNotBlank()) {
                repo.workspaceCommands(workspaceId)
            } else {
                website.msdnna.tessera.data.model.CommandRegistry()
            }
            _state.update {
                val base = it.copy(
                    loading = false,
                    doneColumnId = board?.doneColumnId,
                    columns = columns,
                    tasks = tasks,
                    subtasks = subtasks,
                    dependencies = dependencies,
                    tags = tags.associateBy { t -> t.id },
                    tagList = tags,
                    milestones = milestones,
                    prefixNames = prefixNames,
                    metaTagPrefixes = metaTagPrefixes,
                    estimation = estimation,
                    members = members,
                    gitlabMembers = gitlabMembers,
                    builtinCommands = registry.builtin.orEmpty(),
                    customCommands = registry.custom.orEmpty(),
                    canManageCommands = registry.canManage,
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
        // This board's project was deleted or transferred away elsewhere → its board
        // is gone; signal the host to leave for Home (web project.deleted handler).
        if (ev.type == "project.deleted" &&
            projectId.isNotBlank() &&
            ev.data?.get("id")?.asString == projectId
        ) {
            _gone.value = true
            return
        }
        if (!ev.type.startsWith("task") && !ev.type.startsWith("column") &&
            !ev.type.startsWith("board") && !ev.type.startsWith("milestone")
        ) {
            return
        }
        // Board-activity toast for create/move on THIS board (any actor) — shown
        // regardless of the reload suppression below so own actions confirm too.
        if (ev.type == "task.created" || ev.type == "task.moved") pushActivity(ev)
        if (dragging || SystemClock.elapsedRealtime() < suppressUntil) return
        reloadJob?.cancel()
        reloadJob = viewModelScope.launch {
            delay(REALTIME_DEBOUNCE_MS)
            if (!dragging) silentReload()
        }
    }

    /** Builds an activity toast from a create/move event on the current board. The
     *  move verb is refined by comparing the event's completion state to the card we
     *  still hold locally (reload is debounced), so crossing the done boundary reads
     *  as completed/reopened. */
    private fun pushActivity(ev: RealtimeEvent) {
        val data = ev.data ?: return
        val t = runCatching { gson.fromJson(data, Task::class.java) }.getOrNull() ?: return
        if (t.boardId != boardId) return
        // Anti-flood: during a burst (e.g. a GitLab sync touching many tasks) drop
        // the individual toasts — the board still reloads via the debounced path.
        val now = SystemClock.elapsedRealtime()
        if (now - activityWindowStart > ACTIVITY_BURST_WINDOW_MS) {
            activityWindowStart = now
            activityWindowCount = 0
        }
        activityWindowCount++
        if (activityWindowCount > ACTIVITY_BURST_MAX) return
        var verb = if (ev.type == "task.created") "created" else "moved"
        if (ev.type == "task.moved") {
            val prev = _state.value.tasks.firstOrNull { it.id == t.id }
            val wasDone = prev?.isCompleted == true
            if (t.isCompleted && !wasDone) verb = "completed"
            else if (!t.isCompleted && wasDone) verb = "reopened"
        }
        val actorId = ev.actor.ifBlank { t.createdBy ?: "" }
        val self = actorId.isNotBlank() && actorId == currentUserId
        val actorName = _state.value.membersMap[actorId]?.name ?: ""
        val entry = BoardActivity(
            key = ++activitySeq,
            taskId = t.id,
            number = t.number,
            title = t.title.ifBlank { "Задача" },
            verb = verb,
            actorName = actorName,
            self = self,
        )
        _activity.update { (it + entry).takeLast(3) }
        viewModelScope.launch {
            delay(ACTIVITY_TTL_MS)
            dismissActivity(entry.key)
        }
    }

    /** Dismiss a board-activity toast (auto after a timeout, or manual close). */
    fun dismissActivity(key: Long) {
        _activity.update { list -> list.filterNot { it.key == key } }
    }

    /** Refreshes board data without the loading spinner (for live updates). */
    private suspend fun silentReload() {
        if (boardId.isBlank()) return
        val board = runCatching { repo.board(boardId) }.getOrNull()
        projectId = board?.projectId ?: projectId
        val columns = repo.columns(boardId)
        val tasks = scopedTasks()
        val subtasks = scopedSubtasks()
        val dependencies = runCatching { repo.dependencies(boardId) }.getOrDefault(emptyList())
        val tags = if (projectId.isNotBlank()) runCatching { repo.tags(projectId) }.getOrDefault(emptyList()) else emptyList()
        val milestones = if (projectId.isNotBlank()) runCatching { repo.milestones(projectId) }.getOrDefault(emptyList()) else emptyList()
        val prefixNames = loadPrefixNames()
        // Estimation config is loaded once on full load() — it rarely changes and
        // would cost two extra list calls on every realtime echo here.
        _state.update {
            it.copy(
                doneColumnId = board?.doneColumnId,
                columns = columns,
                tasks = tasks,
                subtasks = subtasks,
                dependencies = dependencies,
                tags = tags.associateBy { t -> t.id },
                tagList = tags,
                milestones = milestones,
                prefixNames = prefixNames,
            )
        }
    }

    /** Canonical GitLab meta-label prefixes for this board's integrations, hidden from
     *  the ADD tag-picker (best-effort). Computed once on [load] — it rarely changes,
     *  so realtime echoes skip the extra integrations call (like the estimation config). */
    private suspend fun loadMetaTagPrefixes(): Set<String> {
        if (workspaceId.isBlank() || boardId.isBlank()) return emptySet()
        val integrations = repo.gitlabIntegrations(workspaceId).filter { it.boardId == boardId }
        if (integrations.isEmpty()) return emptySet()
        return integrations.flatMap { website.msdnna.tessera.util.metaPrefixesFromRules(it.labelRules.rules) }.toSet()
    }

    /** Loads the project's canonical prefix → label map (best-effort, empty on failure). */
    private suspend fun loadPrefixNames(): Map<String, String> =
        if (projectId.isBlank()) {
            emptyMap()
        } else {
            runCatching { repo.tagPrefixes(projectId).associate { it.prefix to it.label } }.getOrDefault(emptyMap())
        }

    /** Resolves the board's estimation unit config (best-effort, built-in default on failure). */
    private suspend fun loadEstimation(): website.msdnna.tessera.data.model.EstimationConfig =
        if (projectId.isBlank() || workspaceId.isBlank()) {
            website.msdnna.tessera.util.Estimation.DEFAULT
        } else {
            runCatching { repo.estimationConfig(workspaceId, projectId) }
                .getOrDefault(website.msdnna.tessera.util.Estimation.DEFAULT)
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

    /** Per-layout toolbar memory: each layout keeps its own group/sort/filter, swapped
     *  on layout change (mirrors web's per-layout toolbar state, session-scoped). */
    private val toolbarByLayout = mutableMapOf<String, BoardViewConfig>()

    fun setViewMode(mode: BoardViewMode) {
        _state.update { s ->
            // snapshot the leaving layout's toolbar, then restore the entered layout's
            // (or coerce grouping the new layout can't show — e.g. assignee on Kanban).
            toolbarByLayout[layoutKey(s.viewMode)] = configFromState(s)
            val saved = toolbarByLayout[layoutKey(mode)]
            if (saved != null) s.applyConfig(saved).copy(viewMode = mode)
            else s.copy(viewMode = mode).coerceGroupingFor(mode)
        }
        persistView()
    }

    /** Sets the grouping mode (status | tag | assignee | none); prefix only for tag. */
    fun setGrouping(mode: String, prefix: String = "") {
        _state.update { it.copy(groupMode = mode, tagPrefix = if (mode == "tag") prefix else "") }
        persistView()
    }

    fun toggleSubtasksExpanded() {
        _state.update { it.copy(subtasksExpanded = !it.subtasksExpanded) }
        persistView()
    }

    /** Toggles "Авто" dependency-graph ordering (Gantt). Turning it on resets the
     *  composer to the bare no-group/no-sort state it needs (web `toggleAuto`). */
    fun toggleAutoSort() {
        _state.update { s ->
            if (s.autoActive) {
                s.copy(autoSort = false)
            } else {
                s.copy(
                    autoSort = true,
                    groupMode = "none",
                    tagPrefix = "",
                    sortLevels = emptyList(),
                    filter = BoardFilter(),
                    currentViewName = null,
                )
            }
        }
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

    /** Reorders the sort levels (drag-reorder of the composer's sort chips, web
     *  `<draggable>` parity). The level order IS the sort precedence — primary
     *  first — so this must persist, or the order is lost on the next board open. */
    fun moveSortLevel(from: Int, to: Int) {
        _state.update { it.copy(sortLevels = it.sortLevels.moveItem(from, to)) }
        persistView()
    }

    fun clearSort() {
        _state.update { it.copy(sortLevels = emptyList()) }
        persistView()
    }

    /**
     * Composer clear-all (×): drops every filter facet, all sort levels **and** the
     * server-side sprint scope, leaving a board with nothing applied. The scope is
     * part of what the bar shows (its accent chip), so leaving it behind would make
     * the × look broken — the bar would still be filtering after a "clear".
     */
    fun clearComposer() {
        val hadScope = _state.value.milestoneScope != null
        _state.update { it.copy(filter = BoardFilter(), sortLevels = emptyList(), milestoneScope = null) }
        persistView()
        if (hadScope) launchCatching { refreshTasks() }
    }

    // ── column collapse (kanban) ──────────────────────────────────────────────

    /** Flips a lane's collapsed state via an explicit override, then persists.
     *  [currentlyCollapsed] is the effective state the UI is showing. */
    fun toggleColumnCollapse(laneId: String, currentlyCollapsed: Boolean) {
        _state.update { it.copy(colCollapse = it.colCollapse + (laneId to !currentlyCollapsed)) }
        persistView()
    }

    /** Toggles the auto-collapse-empty rule. Turning it ON drops explicit overrides
     *  on the now-empty lanes so the rule visibly takes effect (web watch parity). */
    fun setAutoCollapseEmpty(on: Boolean, emptyLaneIds: Set<String>) {
        _state.update { st ->
            val cleaned = if (on) st.colCollapse.filterKeys { it !in emptyLaneIds } else st.colCollapse
            st.copy(autoCollapseEmpty = on, colCollapse = cleaned)
        }
        persistView()
    }

    // ── card customization (kanban) ───────────────────────────────────────────

    fun setCardSize(size: String) {
        _state.update { it.copy(cardSize = size) }
        persistView()
    }

    fun setStackFields(on: Boolean) {
        _state.update { it.copy(stackFields = on) }
        persistView()
    }

    fun setShowEmpty(on: Boolean) {
        _state.update { it.copy(showEmpty = on) }
        persistView()
    }

    /** Flips a single field's on-card visibility (web fieldVis toggle). */
    fun setFieldVisible(key: String, on: Boolean) {
        _state.update { it.copy(fieldVis = it.fieldVis + (key to on)) }
        persistView()
    }

    fun setAutosaveView(on: Boolean) {
        _state.update { it.copy(autosaveView = on) }
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

    /**
     * Replaces the workspace's custom command dictionary (one PUT of the complete
     * desired state) and refreshes the `/`-popup rows. [onDone] closes the editor
     * only on success — a rejected key must not silently drop the other rows.
     */
    fun saveCustomCommands(
        commands: List<website.msdnna.tessera.data.model.WorkspaceCommand>,
        onDone: () -> Unit,
    ) = launchCatching {
        val saved = repo.setWorkspaceCommands(workspaceId, commands)
        _state.update { it.copy(customCommands = saved) }
        onDone()
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

    /** Enter the read-only archive scope: load the board's archived tasks into
     *  [tasks] so the normal views render them (with all filters/grouping/sort),
     *  but no mutations. Subtasks are archived with their parents → cleared. */
    fun enterArchive() = launchCatching {
        _state.update { it.copy(archivedMode = true) }
        val tasks = repo.tasks(boardId, archived = true)
        _state.update { it.copy(tasks = tasks, subtasks = emptyList()) }
    }

    /** Leave the archive scope and reload the live board — bare, with no milestone
     *  narrowing: the sprint the user came from was replaced by the archive scope
     *  (the bar showed the archive chip in its place), so restoring it behind their
     *  back would open a board they never asked for. Any «Этап» facet built *inside*
     *  the archive goes with it, for the same reason. Entering another sprint from
     *  the sidebar leaves the archive through here first, then applies its scope. */
    fun exitArchive() {
        if (!_state.value.archivedMode) return
        _state.update {
            it.copy(
                archivedMode = false,
                milestoneScope = null,
                filter = it.filter.copy(milestoneIds = emptySet()),
            )
        }
        persistView()
        load(boardId, workspaceId)
    }

    /** Reloads the archived-scope task list after a restore/delete. */
    private suspend fun reloadArchiveScope() {
        val tasks = repo.tasks(boardId, archived = true)
        _state.update { it.copy(tasks = tasks, archived = repo.archived(boardId)) }
    }

    fun restoreFromArchive(taskId: String) = launchCatching {
        repo.restoreTask(taskId)
        if (_state.value.archivedMode) {
            reloadArchiveScope()
        } else {
            _state.update { it.copy(archived = repo.archived(boardId)) }
            refreshTasks()
        }
    }

    fun deleteFromArchive(taskId: String) = launchCatching {
        repo.deleteTask(taskId)
        if (_state.value.archivedMode) {
            reloadArchiveScope()
        } else {
            _state.update { it.copy(archived = repo.archived(boardId)) }
        }
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

    /** Pins a task to an Eisenhower quadrant (0-3), or null to derive it. (Matrix view.) */
    fun setEisenhower(taskId: String, quadrant: Int?) = launchCatching {
        repo.setEisenhower(taskId, quadrant)
        refreshTasks()
    }

    /** Matrix quick-add: create in the board's first column, then pin the quadrant. */
    fun createTaskInQuadrant(title: String, quadrant: Int) = launchCatching {
        val columnId = _state.value.columns.firstOrNull()?.id ?: return@launchCatching
        val task = repo.createTask(boardId, columnId, title)
        repo.setEisenhower(task.id, quadrant)
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

    fun setDueAndRecurrence(task: Task, dueIso: String?, startIso: String?, recurrence: Recurrence?) = launchCatching {
        repo.updateTask(task, dueDate = dueIso, startDate = startIso, recurrence = recurrence)
        refreshTasks()
    }

    fun setDueNotify(task: Task, lead: Int?, repeat: Int?, enabled: Boolean?) = launchCatching {
        repo.setDueNotify(task.id, lead, repeat, enabled)
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
        val adding = userId !in task.assigneeIds
        if (adding) repo.addAssignee(task.id, userId) else repo.removeAssignee(task.id, userId)
        // Bump the cross-board MRU only on assign, so the picker surfaces the people
        // you actually use first (web tessera_recent_assignees).
        if (adding) runCatching { AppContainer.prefs.bumpRecentAssignee(userId) }
        refreshTasks()
    }

    // Assign/unassign a GitLab project member (may have no Tessera account). On
    // integration boards with push_assignees on, the backend mirrors it to the issue.
    fun toggleGitlabAssignee(task: Task, m: website.msdnna.tessera.data.model.GitlabMember) = launchCatching {
        if (m.glUsername in task.gitlabAssigneeLogins) repo.removeGitlabAssignee(task.id, m.glUsername)
        else repo.pinGitlabAssignee(task.id, m)
        refreshTasks()
    }

    /** Assign (non-null) or clear (null) a task's milestone. */
    fun setTaskMilestone(task: Task, milestoneId: String?) = launchCatching {
        repo.setTaskMilestone(task.id, milestoneId)
        refreshTasks()
    }

    /**
     * Adds a milestone to the client-side «Этап» facet. Building one's own milestone
     * filter supersedes the server-side sprint scope, so the scope is dropped and the
     * full board reloaded (web `onAddSelect` `fm.*`): the scope narrows the fetch, so
     * a facet for any *other* milestone on top of it would match nothing at all.
     */
    fun addMilestoneFilter(milestoneId: String) {
        val hadScope = _state.value.milestoneScope != null
        _state.update {
            it.copy(
                filter = it.filter.copy(milestoneIds = it.filter.milestoneIds + milestoneId),
                milestoneScope = null,
            )
        }
        persistView()
        if (hadScope) launchCatching { refreshTasks() }
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
        val tasks = scopedTasks()
        val subtasks = scopedSubtasks()
        _state.update { it.copy(tasks = tasks, subtasks = subtasks) }
        markLocalChange()
    }

    /** The board's task set for the scope currently in effect. The archive is a scope
     *  of its own: it lists the board's archived tasks whole, ignoring the sprint
     *  narrowing (which the archive endpoint does not take). Every reload path goes
     *  through here — otherwise any refresh while in the archive (a facet change, a
     *  realtime echo) silently swaps the archived cards for live ones. */
    private suspend fun scopedTasks(): List<Task> =
        if (_state.value.archivedMode) repo.tasks(boardId, archived = true)
        else repo.tasks(boardId, _state.value.milestoneScope)

    /** Subtasks are archived together with their parents, so the archive has none. */
    private suspend fun scopedSubtasks(): List<Task> =
        if (_state.value.archivedMode) emptyList() else repo.subtasks(boardId)

    /** Server-side sprint scope (web parity): reload the board showing only [scope]
     *  (<milestone uuid> | "backlog" | null = all). For large GitLab imports. */
    fun setMilestoneScope(scope: String?) {
        if (_state.value.milestoneScope == scope) return
        _state.update {
            // Entering a sprint from the «Этапы» screen supersedes a hand-built
            // «Этап» facet (web `watch(route.query.milestone)`): the facet was for
            // some other milestone, so keeping it would filter the freshly scoped
            // board down to nothing.
            it.copy(
                milestoneScope = scope,
                filter = if (scope != null) it.filter.copy(milestoneIds = emptySet()) else it.filter,
            )
        }
        persistView()
        launchCatching { refreshTasks() }
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
        const val ACTIVITY_TTL_MS = 6500L

        // Burst guard: more than MAX activity events within WINDOW (e.g. a GitLab
        // sync creating/moving many tasks) stops the per-event toast flood.
        const val ACTIVITY_BURST_WINDOW_MS = 4000L
        const val ACTIVITY_BURST_MAX = 5
    }
}

// ── view config ↔ board state (web schema; camelCase keys for cross-device) ──

/** The web layout key for a view mode (Kanban = "board"). */
private fun layoutKey(mode: BoardViewMode): String = when (mode) {
    BoardViewMode.List -> "list"
    BoardViewMode.Calendar -> "calendar"
    BoardViewMode.Matrix -> "matrix"
    BoardViewMode.Timeline -> "timeline"
    BoardViewMode.Gantt -> "gantt"
    BoardViewMode.Kanban -> "board"
}

/** Coerce a grouping the entered layout can't display (assignee/none → status off
 *  the timeline/Gantt) so e.g. Kanban never inherits a swimlane-only grouping. */
private fun BoardUiState.coerceGroupingFor(mode: BoardViewMode): BoardUiState {
    val timelineLike = mode == BoardViewMode.Timeline || mode == BoardViewMode.Gantt
    return if (!timelineLike && groupMode != "status" && groupMode != "tag" && groupMode != "milestone") {
        copy(groupMode = "status", tagPrefix = "")
    } else {
        this
    }
}

/** Fields rendered on a "medium"-density card (web SIZE_FIELDS.medium). "compact"
 *  renders none, "large" renders all. */
private val MEDIUM_CARD_FIELDS = setOf("number", "priority", "due", "tags", "assignee")

/** Snapshots the toolbar state into a [BoardViewConfig] (web-compatible). */
private fun configFromState(s: BoardUiState): BoardViewConfig = BoardViewConfig(
    layout = layoutKey(s.viewMode),
    groupMode = s.groupMode,
    tagPrefix = s.tagPrefix,
    sortLevels = s.sortLevels,
    subtasksExpanded = s.subtasksExpanded,
    autoSort = s.autoSort,
    colCollapse = s.colCollapse,
    autoCollapseEmpty = s.autoCollapseEmpty,
    cardSize = s.cardSize,
    stackFields = s.stackFields,
    showEmpty = s.showEmpty,
    fieldVis = s.fieldVis,
    autosaveView = s.autosaveView,
    filters = BoardViewFilters(
        priorities = s.filter.priorities.toList(),
        assignees = s.filter.assigneeIds.toList(),
        authors = s.filter.authorIds.toList(),
        tags = s.filter.tagIds.toList(),
        statuses = s.filter.statuses.toList(),
        milestones = s.filter.milestoneIds.toList(),
        due = dueToWeb(s.filter.due),
        q = s.filter.query,
    ),
)

/** Applies a [BoardViewConfig] onto the toolbar state (group/sort/filter/layout). */
private fun BoardUiState.applyConfig(c: BoardViewConfig): BoardUiState = copy(
    viewMode = when (c.layout) {
        "list" -> BoardViewMode.List
        "calendar" -> BoardViewMode.Calendar
        "matrix" -> BoardViewMode.Matrix
        "timeline" -> BoardViewMode.Timeline
        "gantt" -> BoardViewMode.Gantt
        else -> BoardViewMode.Kanban
    },
    groupMode = c.groupMode.ifBlank { "status" },
    tagPrefix = if (c.groupMode == "tag") c.tagPrefix else "",
    sortLevels = c.sortLevels,
    subtasksExpanded = c.subtasksExpanded,
    autoSort = c.autoSort,
    colCollapse = c.colCollapse,
    autoCollapseEmpty = c.autoCollapseEmpty,
    cardSize = c.cardSize.ifBlank { "medium" },
    stackFields = c.stackFields,
    showEmpty = c.showEmpty,
    fieldVis = c.fieldVis,
    autosaveView = c.autosaveView,
    filter = BoardFilter(
        query = c.filters.q,
        priorities = c.filters.priorities.toSet(),
        tagIds = c.filters.tags.toSet(),
        assigneeIds = c.filters.assignees.toSet(),
        authorIds = c.filters.authors.toSet(),
        statuses = c.filters.statuses.toSet(),
        milestoneIds = c.filters.milestones.toSet(),
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
