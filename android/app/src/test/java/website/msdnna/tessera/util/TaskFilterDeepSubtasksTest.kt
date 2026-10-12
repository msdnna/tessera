package website.msdnna.tessera.util

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import website.msdnna.tessera.data.model.Task

/**
 * Фильтры доски и ВЛОЖЕННЫЕ подзадачи (#2932) — порт
 * `frontend/tests/ut-taskFilter-deep-subtasks.spec.js`.
 *
 * Правило #2602 («родителя видно, когда совпал он сам ИЛИ хотя бы одна его подзадача»)
 * работало ровно на один уровень: [filterBoardTasks] читал только прямых детей, хотя
 * доска грузит всё дерево разом — `BoardViewModel.subtasks` это каждая задача с
 * parent_id, любого уровня. Внук, назначенный на читателя, уносил с доски всю ветку:
 * карточки-предка нет, значит нет и пути к нему.
 */
class TaskFilterDeepSubtasksTest {
    // Те же фиксированные сутки, что в TaskFilterTest: «сегодня» — 2026-08-03.
    private val clock = FilterClock(today = "2026-08-03", weekEnd = "2026-08-10")

    // Дерево в три уровня: карточка p1 → «середина» m1 → внуки g1 (мой) и g2 (чужой).
    // Ни p1, ни m1 сами по себе ни под один фильтр ниже не подходят — только внук.
    private val top = Task(id = "p1", title = "Родитель", number = 10, columnId = "c1")
    private val mid = Task(id = "m1", parentId = "p1", title = "Середина", number = 11, columnId = "c1")
    private val grandMine = Task(
        id = "g1",
        parentId = "m1",
        title = "Внук по делу",
        number = 12,
        columnId = "c1",
        priority = 3,
        assigneeIds = listOf("u1"),
        tagIds = listOf("tagA"),
        dueDate = "2026-08-03T09:00:00+03:00",
    )
    private val grandOther = Task(
        id = "g2",
        parentId = "m1",
        title = "Чужой внук",
        number = 13,
        columnId = "c1",
        assigneeIds = listOf("u2"),
    )

    private val deep = mapOf("p1" to listOf(mid), "m1" to listOf(grandMine, grandOther))

    private fun run(
        filter: BoardFilter,
        tasks: List<Task> = listOf(top),
        subs: Map<String, List<Task>> = deep,
    ) = filterBoardTasks(tasks, subs, filter, clock)

    private fun ids(list: List<Task>?) = list?.map { it.id }

    // ── внук поднимает всю ветку ──

    @Test
    fun `assignee filter lifts the card and keeps the path down to the grandchild`() {
        val r = run(BoardFilter(assigneeIds = setOf("u1")))
        assertEquals(setOf("p1"), r.visibleIds)
        assertEquals(listOf("m1"), ids(r.subtasksByParent["p1"]))
        assertEquals(listOf("g1"), ids(r.subtasksByParent["m1"]))
    }

    @Test
    fun `the grandchild list is narrowed and the middle parent is marked`() {
        val r = run(BoardFilter(assigneeIds = setOf("u1")))
        assertTrue("m1" in r.narrowedParents)
        // p1 отдал единственного ребёнка целиком — усечения на этом уровне нет.
        assertFalse("p1" in r.narrowedParents)
    }

    @Test
    fun `a grandchild title lifts the card`() {
        val r = run(BoardFilter(query = "внук по делу"))
        assertEquals(setOf("p1"), r.visibleIds)
        assertEquals(listOf("g1"), ids(r.subtasksByParent["m1"]))
    }

    @Test
    fun `a grandchild number lifts the card`() {
        assertEquals(setOf("p1"), run(BoardFilter(query = "#12")).visibleIds)
    }

    @Test
    fun `tag, priority and due of a grandchild lift the card too`() {
        val filters = listOf(
            BoardFilter(tagIds = setOf("tagA")),
            BoardFilter(priorities = setOf(3)),
            BoardFilter(due = DueFilter.Today),
        )
        for (flt in filters) {
            val r = run(flt)
            assertEquals(flt.toString(), setOf("p1"), r.visibleIds)
            assertEquals(flt.toString(), listOf("g1"), ids(r.subtasksByParent["m1"]))
        }
    }

    @Test
    fun `a great-grandchild works the same — the depth is not bounded`() {
        val plainGrand = grandMine.copy(
            assigneeIds = emptyList(),
            tagIds = emptyList(),
            dueDate = null,
            priority = 0,
        )
        val great = grandMine.copy(id = "gg1", parentId = "g1", number = 14)
        val r = run(
            BoardFilter(assigneeIds = setOf("u1")),
            subs = mapOf("p1" to listOf(mid), "m1" to listOf(plainGrand), "g1" to listOf(great)),
        )
        assertEquals(setOf("p1"), r.visibleIds)
        assertEquals(listOf("gg1"), ids(r.subtasksByParent["g1"]))
    }

    @Test
    fun `a column deep in the branch still does NOT drag the card into that filter`() {
        // statuses не входит в SUBTASK_FACETS: карточку нельзя затащить в колонку,
        // в которой она не стоит, — ни ребёнком, ни внуком.
        val r = run(
            BoardFilter(statuses = setOf("c2")),
            subs = mapOf("p1" to listOf(mid), "m1" to listOf(grandMine.copy(columnId = "c2"), grandOther)),
        )
        assertTrue(r.visibleIds.isEmpty())
    }

    @Test
    fun `the card's own column applies when a grandchild lifts it`() {
        assertEquals(
            setOf("p1"),
            run(BoardFilter(statuses = setOf("c1"), assigneeIds = setOf("u1"))).visibleIds,
        )
        assertTrue(run(BoardFilter(statuses = setOf("c2"), assigneeIds = setOf("u1"))).visibleIds.isEmpty())
    }

    // ── контроли ──

    @Test
    fun `nobody in the branch matched — no card and no trace`() {
        val r = run(BoardFilter(assigneeIds = setOf("u9")))
        assertTrue(r.visibleIds.isEmpty())
        assertNull(r.subtasksByParent["m1"])
    }

    @Test
    fun `a middle parent that matched itself keeps its whole subtree`() {
        val r = run(BoardFilter(query = "середина"))
        assertEquals(setOf("p1"), r.visibleIds)
        assertEquals(listOf("m1"), ids(r.subtasksByParent["p1"]))
        // m1 совпал сам, значит его дети не усекаются — как и у совпавшей карточки.
        assertEquals(listOf("g1", "g2"), ids(r.subtasksByParent["m1"]))
        assertTrue(r.narrowedParents.isEmpty())
    }

    @Test
    fun `the card itself matched — the tree is left alone`() {
        val r = run(BoardFilter(query = "родитель"))
        assertEquals(listOf("m1"), ids(r.subtasksByParent["p1"]))
        assertEquals(listOf("g1", "g2"), ids(r.subtasksByParent["m1"]))
        assertTrue(r.narrowedParents.isEmpty())
    }

    @Test
    fun `an empty composer returns the tree as it is`() {
        val r = run(BoardFilter())
        assertEquals(setOf("p1"), r.visibleIds)
        assertEquals(listOf("m1"), ids(r.subtasksByParent["p1"]))
        assertEquals(listOf("g1", "g2"), ids(r.subtasksByParent["m1"]))
        assertTrue(r.narrowedParents.isEmpty())
    }

    @Test
    fun `both grandchildren matched — nothing is narrowed`() {
        val r = run(
            BoardFilter(assigneeIds = setOf("u1")),
            subs = mapOf(
                "p1" to listOf(mid),
                "m1" to listOf(grandMine, grandOther.copy(assigneeIds = listOf("u1"))),
            ),
        )
        assertEquals(2, r.subtasksByParent["m1"]?.size)
        assertTrue(r.narrowedParents.isEmpty())
    }

    @Test
    fun `a parent chain looping back on itself does not hang the filter`() {
        // Карта приходит с сервера плоской (parent_id → дети); битая цепочка,
        // замкнутая на себя, не должна уводить обход в бесконечность — тот же повод,
        // по которому страхуется countWithSubtasks.
        val r = run(
            BoardFilter(assigneeIds = setOf("u1")),
            subs = mapOf("p1" to listOf(mid), "m1" to listOf(grandMine), "g1" to listOf(top)),
        )
        assertEquals(setOf("p1"), r.visibleIds)
    }
}
