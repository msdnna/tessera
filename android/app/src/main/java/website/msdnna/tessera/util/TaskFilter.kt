package website.msdnna.tessera.util

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
 * That rule holds at EVERY depth (#2932). Subtasks are a tree and the board loads all of
 * it into one flat parent id → children map (`BoardViewModel.subtasks` is every task with
 * a parent_id, not just the first level — [countWithSubtasks] walks the same map through
 * all of them), but the pass here used to read direct children only. A grandchild
 * assigned to you therefore took its whole branch off the board — no card for its
 * parent, so no way to reach it at all, which is the very thing #2602 was about, one
 * level further down. Web had the same hole and was fixed first
 * (`frontend/src/utils/taskFilter.js`); this is the port.
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
data class FilterClock(
    val today: String,
    val weekEnd: String,
    /**
     * The calendar the boundaries were read in — carried, not re-derived, so the due
     * dates compared against them ([matchesDue]) are bucketed by the same zone. A
     * clock built for Europe/Moscow and due keys read off the device would be two
     * calendars in one comparison.
     */
    val fmt: DateFormatPrefs = DateFormatPrefs.Default,
) {
    companion object {
        /** Boundaries from the clock in the user's zone: today, and today + 7 days —
         *  the *exclusive* end of the week window (see [matchesDue]). */
        fun now(fmt: DateFormatPrefs = DateFormatPrefs.Default): FilterClock =
            FilterClock(dayKeyIn(fmt.zone, 0), dayKeyIn(fmt.zone, DAYS_IN_WEEK), fmt)
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
    // dueDayKey, не isoDateKey: границы [clock] — дни календаря пользователя, и
    // сравнивать их надо с тем же днём, который стоит в подписи карточки.
    val due = dueDayKey(task.dueDate, clock.fmt)
    return when (mode) {
        DueFilter.All -> true

        DueFilter.Has -> due.isNotEmpty()

        DueFilter.None -> due.isEmpty()

        DueFilter.Overdue -> due.isNotEmpty() && due < clock.today && !task.isCompleted

        DueFilter.Today -> due == clock.today

        // Half-open [today, today+7) — seven days counting today, not eight. The same
        // window the server counts the home «На неделе» tile in (`due_date < @week_end`
        // in WorkspaceTaskSummary), the home screen filters on (HomeViewModel) and web's
        // «Срок» facet uses (`dueDay < weekEnd` in utils/taskFilter.js). It has to be the
        // same one twice over: the tile's number is the caption of the list this filter
        // draws, and a saved board view carries the facet as the bare string "week"
        // between web and the phone — an eighth day here is a card the other side never
        // shows.
        DueFilter.Week -> due.isNotEmpty() && due >= clock.today && due < clock.weekEnd
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
    val walk = SubtaskWalk(subtasksByParent, filter, clock, glLoginByUserId)
    val visible = LinkedHashSet<String>()
    for (t in tasks) {
        if (walk.keepCard(t, subFacets)) visible += t.id
    }
    return FilteredBoard(visible, walk.out, walk.narrowed)
}

/**
 * Does this top-level card stay on the board? Same two branches as a subtask
 * ([keepSubtree]) — matched by itself → its whole tree stays as it is; matched only
 * through something below → the child list is narrowed to the surviving branches.
 */
private fun SubtaskWalk.keepCard(t: Task, subFacets: Boolean): Boolean {
    if (matchesTask(t, filter, clock, glLoginByUserId = glLoginByUserId)) {
        emitSubtree(t.id, mutableSetOf(t.id))
        return true
    }
    val subs = byParent[t.id].orEmpty()
    if (!subFacets || subs.isEmpty()) return false
    // The card itself failed. It may still be lifted by something below it — but only if
    // it passes the facets a subtask cannot stand in for (its board column), otherwise
    // the card would appear in a column it isn't in.
    if (!matchesTask(t, filter, clock, setOf(TaskFacet.Statuses), glLoginByUserId)) return false
    val seen = mutableSetOf(t.id)
    val hits = subs.filter { keepSubtree(it, seen) }
    if (hits.isEmpty()) return false
    out[t.id] = hits
    if (hits.size < subs.size) narrowed += t.id
    return true
}

/**
 * One filtering pass over the subtask tree: everything [keepSubtree] reads besides the
 * node it is looking at, plus what it writes. Bundled so the recursion carries two
 * arguments instead of eight.
 */
private class SubtaskWalk(
    val byParent: Map<String, List<Task>>,
    val filter: BoardFilter,
    val clock: FilterClock,
    val glLoginByUserId: Map<String, String>,
    val out: MutableMap<String, List<Task>> = mutableMapOf(),
    val narrowed: MutableSet<String> = mutableSetOf(),
)

/**
 * Copy the branch under [id] into [SubtaskWalk.out] untouched, at every depth — what a
 * task that matched by itself takes with it: its children are not the reason it is
 * visible, so none of them is hidden.
 *
 * [seen] is every id already placed on this walk. The child map is flat and comes from
 * the server, so a parent chain that looped back on itself would otherwise recurse
 * forever — [countWithSubtasks] guards the same map for the same reason.
 */
private fun SubtaskWalk.emitSubtree(id: String, seen: MutableSet<String>) {
    val subs = byParent[id].orEmpty()
    if (subs.isEmpty()) return
    out[id] = subs
    for (s in subs) {
        if (s.id.isEmpty() || !seen.add(s.id)) continue
        emitSubtree(s.id, seen)
    }
}

/**
 * Does this subtask — or anything under it — survive the filter? Writes the child lists
 * it keeps into [SubtaskWalk.out] and records every level it had to narrow in
 * [SubtaskWalk.narrowed], then answers whether its parent may count it as a hit.
 *
 * Same two branches as a top-level card: matched by itself → the whole branch below
 * stays; matched only through a descendant → the child list is narrowed to the surviving
 * ones, so what is drawn is the path down to the match and nothing else. Nothing is
 * written when the answer is false, so a dead branch leaves no trace in the result.
 */
private fun SubtaskWalk.keepSubtree(node: Task, seen: MutableSet<String>): Boolean {
    if (node.id.isEmpty() || !seen.add(node.id)) return false
    if (matchesTask(node, filter, clock, SUBTASK_FACETS, glLoginByUserId)) {
        emitSubtree(node.id, seen)
        return true
    }
    val subs = byParent[node.id].orEmpty()
    if (subs.isEmpty()) return false
    val hits = subs.filter { keepSubtree(it, seen) }
    if (hits.isEmpty()) return false
    out[node.id] = hits
    if (hits.size < subs.size) narrowed += node.id
    return true
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
    // Collator, not a raw String compare: the web sorts with its own collator, where
    // Latin precedes Cyrillic, case/ё are folded and digits compare by value — a
    // code-unit sort disagrees on all three ([UserTextOrder]).
    return byLogin.values.sortedWith { a, b -> UserTextOrder.compare(a.name, b.name) }
}
