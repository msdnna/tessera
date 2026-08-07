package website.msdnna.tessera.util

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import website.msdnna.tessera.data.model.Task

/**
 * Composer-bar filtering: the subtask-lifts-parent rule and the person/due predicates.
 * Cases mirror `frontend/tests/ut-taskFilter.spec.js` so both clients stay in step.
 */
class TaskFilterTest {
    // A fixed clock: "today" is 2026-08-03, the week window ends 2026-08-10.
    private val clock = FilterClock(today = "2026-08-03", weekEnd = "2026-08-10")

    // One parent in column c1 with two children; only the first is assigned to u1.
    private val parent = Task(id = "p1", title = "Родитель", number = 10, columnId = "c1")
    private val subMine = Task(
        id = "s1",
        parentId = "p1",
        title = "Моя подзадача",
        number = 11,
        columnId = "c1",
        priority = 3,
        assigneeIds = listOf("u1"),
        tagIds = listOf("tagA"),
        dueDate = "2026-08-03T00:00:00+03:00",
    )
    private val subOther = Task(
        id = "s2",
        parentId = "p1",
        title = "Чужая подзадача",
        number = 12,
        columnId = "c2",
        assigneeIds = listOf("u2"),
    )
    private val subsByParent = mapOf("p1" to listOf(subMine, subOther))

    private fun run(
        filter: BoardFilter,
        tasks: List<Task> = listOf(parent),
        subs: Map<String, List<Task>> = subsByParent,
        glLoginByUserId: Map<String, String> = emptyMap(),
    ) = filterBoardTasks(tasks, subs, filter, clock, glLoginByUserId)

    // ── подзадача поднимает родителя ──

    @Test
    fun `assignee filter keeps the parent and only the matching subtask`() {
        val r = run(BoardFilter(assigneeIds = setOf("u1")))
        assertEquals(setOf("p1"), r.visibleIds)
        assertEquals(listOf("s1"), r.subtasksByParent["p1"]?.map { it.id })
        assertEquals(setOf("p1"), r.narrowedParents)
    }

    @Test
    fun `a subtask title or number lifts the parent`() {
        assertEquals(setOf("p1"), run(BoardFilter(query = "моя подзадача")).visibleIds)
        assertEquals(setOf("p1"), run(BoardFilter(query = "#11")).visibleIds)
    }

    @Test
    fun `tags, priority and due of a subtask lift the parent too`() {
        assertEquals(setOf("p1"), run(BoardFilter(tagIds = setOf("tagA"))).visibleIds)
        assertEquals(setOf("p1"), run(BoardFilter(priorities = setOf(3))).visibleIds)
        assertEquals(setOf("p1"), run(BoardFilter(due = DueFilter.Today)).visibleIds)
    }

    @Test
    fun `a gl-login assignee on a subtask lifts the parent`() {
        val gl = subMine.copy(assigneeIds = emptyList(), gitlabAssigneeLogins = listOf("msdnna"))
        val r = run(BoardFilter(assigneeIds = setOf("gl:msdnna")), subs = mapOf("p1" to listOf(gl, subOther)))
        assertEquals(setOf("p1"), r.visibleIds)
        assertEquals(listOf("s1"), r.subtasksByParent["p1"]?.map { it.id })
    }

    @Test
    fun `neither the parent nor any child matched — the parent is gone`() {
        assertTrue(run(BoardFilter(assigneeIds = setOf("u9"))).visibleIds.isEmpty())
    }

    // ── автор (#2603) ──

    @Test
    fun `author facet matches a tessera author and lifts a parent by its child`() {
        val mine = subMine.copy(createdBy = "u1")
        val r = run(BoardFilter(authorIds = setOf("u1")), subs = mapOf("p1" to listOf(mine, subOther)))
        assertEquals(setOf("p1"), r.visibleIds)
        assertEquals(listOf("s1"), r.subtasksByParent["p1"]?.map { it.id })
    }

    @Test
    fun `a gitlab-synced task matches its tessera author through the linked login`() {
        val synced = parent.copy(createdBy = null, gitlabAuthor = "msdnna")
        // Picking the Tessera person finds the synced card only via the link map.
        assertTrue(
            matchesAuthor(synced, setOf("u1"), mapOf("u1" to "msdnna")),
        )
        assertFalse(matchesAuthor(synced, setOf("u1")))
        // …and the bare `gl:` value always matches.
        assertTrue(matchesAuthor(synced, setOf("gl:msdnna")))
    }

    @Test
    fun `a task with no author at all is filtered out once an author is picked`() {
        val orphan = parent.copy(createdBy = null, gitlabAuthor = null)
        assertFalse(matchesAuthor(orphan, setOf("u1", "gl:msdnna")))
        assertTrue(matchesAuthor(orphan, emptySet()))
    }

    // ── колонка (statuses) не пробрасывается ──

    @Test
    fun `a subtask in another column does NOT drag its parent into that filter`() {
        // s2 lives in c2, the parent in c1: filtering by column c2 must not show it.
        assertTrue(run(BoardFilter(statuses = setOf("c2"))).visibleIds.isEmpty())
    }

    @Test
    fun `the parent's own column still applies when a child lifts it`() {
        assertEquals(
            setOf("p1"),
            run(BoardFilter(statuses = setOf("c1"), assigneeIds = setOf("u1"))).visibleIds,
        )
        assertTrue(run(BoardFilter(statuses = setOf("c2"), assigneeIds = setOf("u1"))).visibleIds.isEmpty())
    }

    // ── родитель совпал сам ──

    @Test
    fun `a parent that matched itself keeps every child and is not narrowed`() {
        val r = run(BoardFilter(query = "родитель"))
        assertEquals(listOf("s1", "s2"), r.subtasksByParent["p1"]?.map { it.id })
        assertTrue(r.narrowedParents.isEmpty())
    }

    @Test
    fun `all children matched — no narrowing`() {
        val r = run(
            BoardFilter(assigneeIds = setOf("u1")),
            subs = mapOf("p1" to listOf(subMine, subOther.copy(assigneeIds = listOf("u1")))),
        )
        assertEquals(2, r.subtasksByParent["p1"]?.size)
        assertTrue(r.narrowedParents.isEmpty())
    }

    // ── matchesDue ──

    @Test
    fun `matchesDue none and has`() {
        val none = Task(id = "t")
        val dated = Task(id = "t", dueDate = "2026-08-03T09:00:00+03:00")
        assertTrue(matchesDue(none, DueFilter.None, clock))
        assertFalse(matchesDue(dated, DueFilter.None, clock))
        assertTrue(matchesDue(dated, DueFilter.Has, clock))
        assertFalse(matchesDue(none, DueFilter.Has, clock))
    }

    @Test
    fun `matchesDue today, week and overdue`() {
        fun t(due: String, completed: String? = null) = Task(id = "t", dueDate = due, completedAt = completed)
        assertTrue(matchesDue(t("2026-08-03T09:00:00"), DueFilter.Today, clock))
        assertTrue(matchesDue(t("2026-08-06T09:00:00"), DueFilter.Week, clock))
        // The seventh day is inside the window (web `dueDay - today <= 7 days`).
        assertTrue(matchesDue(t("2026-08-10T09:00:00"), DueFilter.Week, clock))
        assertFalse(matchesDue(t("2026-08-20T09:00:00"), DueFilter.Week, clock))
        assertTrue(matchesDue(t("2026-08-01T09:00:00"), DueFilter.Overdue, clock))
        // A closed task is never «просроченная».
        assertFalse(matchesDue(t("2026-08-01", "2026-08-02T10:00:00"), DueFilter.Overdue, clock))
    }

    // ── matchesTask / hasSubtaskFacets ──

    @Test
    fun `facets narrow which filters are checked`() {
        val flt = BoardFilter(statuses = setOf("c9"), assigneeIds = setOf("u1"))
        assertFalse(matchesTask(subMine, flt, clock)) // wrong column
        assertTrue(matchesTask(subMine, flt, clock, facets = setOf(TaskFacet.Assignees)))
    }

    @Test
    fun `without a subtask-liftable facet the child pass is skipped`() {
        assertFalse(hasSubtaskFacets(BoardFilter()))
        assertFalse(hasSubtaskFacets(BoardFilter(statuses = setOf("c1"))))
        assertFalse(hasSubtaskFacets(BoardFilter(query = "  ")))
        assertTrue(hasSubtaskFacets(BoardFilter(query = "x")))
        assertTrue(hasSubtaskFacets(BoardFilter(tagIds = setOf("tagA"))))
        assertTrue(hasSubtaskFacets(BoardFilter(authorIds = setOf("u1"))))
    }

    @Test
    fun `a gl-assignee value matches the login, not the display name`() {
        val t = Task(id = "t", gitlabAssignees = listOf("Иван Петров"), gitlabAssigneeLogins = listOf("ivan"))
        assertTrue(matchesAssignee(t, setOf("gl:ivan")))
        assertFalse(matchesAssignee(t, setOf("gl:Иван Петров")))
    }

    // ── boardGitlabAuthors ──

    @Test
    fun `boardGitlabAuthors dedups by login and sorts by display name`() {
        val tasks = listOf(
            Task(id = "1", gitlabAuthor = "zoe", gitlabAuthorName = "Зоя"),
            Task(id = "2", gitlabAuthor = "abe", gitlabAuthorName = "Абрам", gitlabAuthorAvatarUrl = "u"),
            Task(id = "3", gitlabAuthor = "zoe", gitlabAuthorName = "Зоя"),
            Task(id = "4", gitlabAuthor = null),
            // No display name → the login stands in.
            Task(id = "5", gitlabAuthor = "bob"),
        )
        val authors = boardGitlabAuthors(tasks)
        // Latin before Cyrillic, like web `localeCompare(…, 'ru')`.
        assertEquals(listOf("bob", "Абрам", "Зоя"), authors.map { it.name })
        assertEquals("u", authors.first { it.username == "abe" }.avatarUrl)
    }
}
