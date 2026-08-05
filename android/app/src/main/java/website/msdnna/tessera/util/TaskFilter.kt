package website.msdnna.tessera.util

import java.text.Collator
import java.util.Calendar
import java.util.Locale
import website.msdnna.tessera.data.model.Task

/*
 * Composer-bar filtering for the board — port of `frontend/src/utils/taskFilter.js`
 * and the person predicates of `frontend/src/utils/boardFilters.js`.
 *
 * Filtering is entirely client-side: the board loads its top-level tasks and every
 * subtask, and this module decides what stays visible.
 *
 * Key rule (task #2602): a parent card stays on the board when IT matches OR when at
 * least one of its subtasks matches — otherwise a subtask assigned to you was invisible
 * under an "assignee = me" filter. When only a subtask matched, the parent's on-card
 * subtask list is narrowed to the matching children. The task modal is untouched and
 * always lists every subtask.
 *
 * Pure functions: no Compose, no network, no clock reads (the caller passes a
 * [FilterClock]) — so the whole module is unit-testable.
 */

private const val GL_PREFIX = "gl:"
private const val DAYS_IN_WEEK = 7

enum class DueFilter { All, Overdue, Today, Week, Has, None }

/** Client-side board filter (web KanbanBoard `filters`). Empty = show everything. */
data class BoardFilter(
    val query: String = "",
    val priorities: Set<Int> = emptySet(),
    val tagIds: Set<String> = emptySet(),
    val assigneeIds: Set<String> = emptySet(),
    /** Author facet (web `filters.authors`): Tessera user ids and `gl:<login>` values. */
    val authorIds: Set<String> = emptySet(),
    // Status = board column ids (timeline-only facet; lets you hide e.g. «done»).
    val statuses: Set<String> = emptySet(),
    // Milestone ids to show; "__none__" matches milestone-less tasks (deep-link from
    // the «Этапы» screen sets a single id). Empty = no milestone filter.
    val milestoneIds: Set<String> = emptySet(),
    val due: DueFilter = DueFilter.All,
) {
    val isActive: Boolean
        get() = query.isNotBlank() || priorities.isNotEmpty() || tagIds.isNotEmpty() ||
            assigneeIds.isNotEmpty() || authorIds.isNotEmpty() || statuses.isNotEmpty() ||
            milestoneIds.isNotEmpty() || due != DueFilter.All
}

/** One filter facet. [SUBTASK_FACETS] is the subset a subtask may lift its parent by. */
enum class TaskFacet { Query, Priorities, Assignees, Authors, Tags, Statuses, Milestones, Due }

/**
 * Facets a subtask may "lift" its parent by. [TaskFacet.Statuses] (the board column) is
 * deliberately excluded: a subtask can live in a different column, and letting it lift
 * the parent would draw the parent into a column it isn't in. [TaskFacet.Authors]
 * (task #2603) behaves like assignees — a subtask by the picked author lifts its parent.
 */
val SUBTASK_FACETS: Set<TaskFacet> = setOf(
    TaskFacet.Query,
    TaskFacet.Assignees,
    TaskFacet.Authors,
    TaskFacet.Tags,
    TaskFacet.Priorities,
    TaskFacet.Due,
    TaskFacet.Milestones,
)

/**
 * The `yyyy-MM-dd` boundaries the «Срок» facet compares against, injected so filtering
 * never reads the clock itself (web passes a `now` Date for the same reason).
 */
data class FilterClock(val today: String, val weekEnd: String) {
    companion object {
        /** Boundaries from the device clock: today and today + 7 days. */
        fun now(): FilterClock = FilterClock(dateKey(0), dateKey(DAYS_IN_WEEK))

        /** A local `yyyy-MM-dd` key for today + [days], matching due-date keys. */
        private fun dateKey(days: Int): String {
            val cal = Calendar.getInstance().apply { add(Calendar.DAY_OF_MONTH, days) }
            val y = cal.get(Calendar.YEAR)
            val m = (cal.get(Calendar.MONTH) + 1).toString().padStart(2, '0')
            val d = cal.get(Calendar.DAY_OF_MONTH).toString().padStart(2, '0')
            return "$y-$m-$d"
        }
    }
}

/** True for a `gl:<username>` filter value (as opposed to a Tessera user uuid). */
fun isGitlabValue(v: String): Boolean = v.startsWith(GL_PREFIX)

/** The bare GitLab login behind a `gl:` value ("" for a Tessera uuid). */
fun gitlabLogin(v: String): String = if (isGitlabValue(v)) v.removePrefix(GL_PREFIX) else ""

/**
 * Assignee facet: a task matches when any selected person is among its Tessera assignees
 * or its GitLab assignee logins. An empty selection filters nothing.
 */
fun matchesAssignee(task: Task, selected: Set<String>): Boolean {
    if (selected.isEmpty()) return true
    return selected.any { a ->
        // `gl:` values carry the LOGIN, so they match gitlabAssigneeLogins — matching the
        // parallel `gitlabAssignees` (display names) silently found nothing whenever a
        // GitLab member's name differed from their username.
        if (isGitlabValue(a)) gitlabLogin(a) in task.gitlabAssigneeLogins else a in task.assigneeIds
    }
}

/**
 * Author facet. GitLab-synced tasks have `created_by IS NULL` — their author lives in
 * `gitlab_author` — so a Tessera person also matches through the GitLab login linked to
 * their account ([glLoginByUserId]: tessera user id → gl_username). A task with neither
 * author is filtered out once any author is selected.
 */
fun matchesAuthor(task: Task, selected: Set<String>, glLoginByUserId: Map<String, String> = emptyMap()): Boolean {
    if (selected.isEmpty()) return true
    val createdBy = task.createdBy
    val glAuthor = task.gitlabAuthor
    return selected.any { a ->
        if (isGitlabValue(a)) {
            glAuthor != null && glAuthor == gitlabLogin(a)
        } else {
            val login = glLoginByUserId[a]
            createdBy == a || (login != null && glAuthor == login)
        }
    }
}

/** Due-date predicate for the «Срок» filter, against [clock]'s boundaries. */
fun matchesDue(task: Task, mode: DueFilter, clock: FilterClock): Boolean {
    val due = isoDateKey(task.dueDate)
    return when (mode) {
        DueFilter.All -> true

        DueFilter.Has -> due.isNotEmpty()

        DueFilter.None -> due.isEmpty()

        DueFilter.Overdue -> due.isNotEmpty() && due < clock.today && !task.isCompleted

        DueFilter.Today -> due == clock.today

        // Inclusive of the seventh day, like web (`dueDay - today <= 7 days`).
        DueFilter.Week -> due.isNotEmpty() && due >= clock.today && due <= clock.weekEnd
    }
}

/**
 * Does one task pass every active facet? [facets] limits which facets are considered
 * (subtasks use [SUBTASK_FACETS]); null = all of them.
 */
fun matchesTask(
    task: Task,
    filter: BoardFilter,
    clock: FilterClock,
    facets: Set<TaskFacet>? = null,
    glLoginByUserId: Map<String, String> = emptyMap(),
): Boolean = (facets ?: TaskFacet.entries.toSet()).all { facet ->
    passesFacet(facet, task, filter, clock, glLoginByUserId)
}

/** One facet's verdict. An inactive (empty) facet passes everything. */
private fun passesFacet(
    facet: TaskFacet,
    task: Task,
    filter: BoardFilter,
    clock: FilterClock,
    glLoginByUserId: Map<String, String>,
): Boolean = when (facet) {
    TaskFacet.Priorities -> filter.priorities.isEmpty() || task.priority in filter.priorities

    TaskFacet.Assignees -> matchesAssignee(task, filter.assigneeIds)

    TaskFacet.Authors -> matchesAuthor(task, filter.authorIds, glLoginByUserId)

    TaskFacet.Tags -> filter.tagIds.isEmpty() || task.tagIds.any { it in filter.tagIds }

    TaskFacet.Statuses -> filter.statuses.isEmpty() || task.columnId in filter.statuses

    TaskFacet.Milestones -> filter.milestoneIds.isEmpty() ||
        (task.milestoneId ?: "__none__") in filter.milestoneIds

    TaskFacet.Due -> matchesDue(task, filter.due, clock)

    TaskFacet.Query -> filter.query.isBlank() || matchesQuery(task, filter.query.trim().lowercase())
}

private fun matchesQuery(task: Task, q: String): Boolean =
    task.title.lowercase().contains(q) || task.number?.let { "#$it".contains(q) } == true

/**
 * True when at least one facet a subtask may lift its parent by is active. With no such
 * facet the subtask pass is a no-op and can be skipped entirely.
 */
fun hasSubtaskFacets(filter: BoardFilter): Boolean = SUBTASK_FACETS.any { f ->
    when (f) {
        TaskFacet.Query -> filter.query.isNotBlank()
        TaskFacet.Assignees -> filter.assigneeIds.isNotEmpty()
        TaskFacet.Authors -> filter.authorIds.isNotEmpty()
        TaskFacet.Tags -> filter.tagIds.isNotEmpty()
        TaskFacet.Priorities -> filter.priorities.isNotEmpty()
        TaskFacet.Due -> filter.due != DueFilter.All
        TaskFacet.Milestones -> filter.milestoneIds.isNotEmpty()
        TaskFacet.Statuses -> false
    }
}

/**
 * Outcome of filtering a board.
 *
 * @property visibleIds ids of the top-level tasks that stay on the board
 * @property subtasksByParent per-parent child lists, narrowed for parents that only
 *   survived because a child matched
 * @property narrowedParents parents whose child list was narrowed (the card shows an
 *   «N из M» hint and locks subtask drag-reorder)
 */
data class FilteredBoard(
    val visibleIds: Set<String> = emptySet(),
    val subtasksByParent: Map<String, List<Task>> = emptyMap(),
    val narrowedParents: Set<String> = emptySet(),
)

/** Filter a board's tasks and subtasks together. */
fun filterBoardTasks(
    tasks: List<Task>,
    subtasksByParent: Map<String, List<Task>>,
    filter: BoardFilter,
    clock: FilterClock,
    glLoginByUserId: Map<String, String> = emptyMap(),
): FilteredBoard {
    val subFacets = hasSubtaskFacets(filter)
    val visible = LinkedHashSet<String>()
    val outSubs = mutableMapOf<String, List<Task>>()
    val narrowed = mutableSetOf<String>()

    for (t in tasks) {
        val subs = subtasksByParent[t.id].orEmpty()
        if (matchesTask(t, filter, clock, glLoginByUserId = glLoginByUserId)) {
            visible += t.id
            if (subs.isNotEmpty()) outSubs[t.id] = subs
        } else {
            val hits = if (subFacets) liftingChildren(t, subs, filter, clock, glLoginByUserId) else emptyList()
            if (hits.isNotEmpty()) {
                visible += t.id
                outSubs[t.id] = hits
                if (hits.size < subs.size) narrowed += t.id
            }
        }
    }
    return FilteredBoard(visible, outSubs, narrowed)
}

/**
 * The children that keep a non-matching [parent] on the board, or empty when none do.
 * The parent must still pass the facets a subtask cannot stand in for (its board column),
 * otherwise the card would appear in a column it isn't in.
 */
private fun liftingChildren(
    parent: Task,
    subs: List<Task>,
    filter: BoardFilter,
    clock: FilterClock,
    glLoginByUserId: Map<String, String>,
): List<Task> {
    if (subs.isEmpty()) return emptyList()
    if (!matchesTask(parent, filter, clock, setOf(TaskFacet.Statuses), glLoginByUserId)) return emptyList()
    return subs.filter { matchesTask(it, filter, clock, SUBTASK_FACETS, glLoginByUserId) }
}

/** A GitLab author seen on the board, for the author-filter menu. */
data class GitlabAuthor(val username: String, val name: String, val avatarUrl: String)

/**
 * Distinct GitLab authors present on the board: an issue can be opened by someone outside
 * the project's member roster, who would otherwise never appear as a filter option.
 * Sorted by display name.
 */
fun boardGitlabAuthors(tasks: List<Task>): List<GitlabAuthor> {
    val byLogin = LinkedHashMap<String, GitlabAuthor>()
    for (t in tasks) {
        val login = t.gitlabAuthor
        if (login.isNullOrBlank() || login in byLogin) continue
        byLogin[login] = GitlabAuthor(login, t.gitlabAuthorName?.ifBlank { login } ?: login, t.gitlabAuthorAvatarUrl.orEmpty())
    }
    // Collator, not a raw String compare: web sorts with localeCompare(…, 'ru'), where
    // Latin precedes Cyrillic and case/ё are folded — a code-unit sort disagrees.
    val collator = Collator.getInstance(Locale("ru"))
    return byLogin.values.sortedWith { a, b -> collator.compare(a.name, b.name) }
}
