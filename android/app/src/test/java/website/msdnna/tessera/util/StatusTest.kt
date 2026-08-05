package website.msdnna.tessera.util

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test
import website.msdnna.tessera.data.model.BoardColumn
import website.msdnna.tessera.data.model.Task

/** Board-column helpers behind the status row and the card divergence marker. */
class StatusTest {
    private fun col(id: String, position: Double, name: String = id) =
        BoardColumn(id = id, name = name, position = position)

    private fun task(id: String, columnId: String, position: Double = 0.0, parentId: String? = null) =
        Task(id = id, columnId = columnId, position = position, parentId = parentId)

    // Deliberately out of order: the caller's list arrives unsorted.
    private val columns = listOf(col("c", 3.0), col("a", 1.0), col("b", 2.0))

    @Test
    fun `sorts columns by float position, not by index`() {
        assertEquals(listOf("a", "b", "c"), sortedColumns(columns).map { it.id })
    }

    @Test
    fun `columnById resolves, and yields null for blank or unknown ids`() {
        assertEquals("b", columnById(columns, "b")?.id)
        assertNull(columnById(columns, ""))
        assertNull(columnById(columns, null))
        assertNull(columnById(columns, "nope"))
    }

    @Test
    fun `nextColumn walks right and stops at the last column`() {
        assertEquals("b", nextColumn(columns, "a")?.id)
        assertEquals("c", nextColumn(columns, "b")?.id)
        assertNull(nextColumn(columns, "c"))
        assertNull(nextColumn(columns, "nope"))
        assertNull(nextColumn(emptyList(), "a"))
    }

    @Test
    fun `divergedColumn marks only a child in another column`() {
        assertEquals("b", divergedColumn("b", "a", columns)?.id)
        // Same column as the parent → no chip (it would sit on every row).
        assertNull(divergedColumn("a", "a", columns))
        // Unknown / missing column → render nothing rather than throw.
        assertNull(divergedColumn("gone", "a", columns))
        assertNull(divergedColumn(null, "a", columns))
        assertNull(divergedColumn("b", null, columns))
    }

    @Test
    fun `doneTarget resolves the board done column, null when unset`() {
        assertEquals("c", doneTarget(columns, "c")?.id)
        assertNull(doneTarget(columns, null))
        assertNull(doneTarget(columns, "gone"))
    }

    @Test
    fun `siblingNeighbors keeps a subtask's place in the parent list`() {
        val subs = listOf(task("s1", "a"), task("s2", "a"), task("s3", "a"))
        // Middle: sits between its two neighbours.
        assertEquals(MoveNeighbors("s1", "s3"), siblingNeighbors(subs, "s2"))
        // First: nothing before it.
        assertEquals(MoveNeighbors(null, "s2"), siblingNeighbors(subs, "s1"))
        // Last: nothing after it.
        assertEquals(MoveNeighbors("s2", null), siblingNeighbors(subs, "s3"))
        // Unknown id → bare nulls (the backend picks the default slot).
        assertEquals(MoveNeighbors(), siblingNeighbors(subs, "nope"))
        assertEquals(MoveNeighbors(), siblingNeighbors(emptyList(), "s1"))
    }

    @Test
    fun `columnTail finds the highest position in the target column, self excluded`() {
        val tasks = listOf(
            task("t1", "a", position = 10.0),
            task("t2", "a", position = 30.0),
            task("t3", "a", position = 20.0),
            task("t4", "b", position = 99.0),
        )
        assertEquals("t2", columnTail(tasks, "a", selfId = null))
        // The moved task never becomes its own neighbour.
        assertEquals("t3", columnTail(tasks, "a", selfId = "t2"))
        assertEquals("t4", columnTail(tasks, "b", selfId = null))
        // Empty target column / unknown column → no anchor.
        assertNull(columnTail(tasks, "c", selfId = null))
        assertNull(columnTail(tasks, null, selfId = null))
        assertNull(columnTail(emptyList(), "a", selfId = null))
    }

    @Test
    fun `moveNeighbors appends a top-level task to the column tail`() {
        val board = listOf(task("t1", "a", position = 10.0), task("t2", "b", position = 20.0))
        // Never bare nulls on a populated board: positionBetween(nil, nil) = 65536
        // would drop the card near the top of the target column.
        assertEquals(
            MoveNeighbors(beforeId = "t2"),
            moveNeighbors("t1", parentId = null, targetColumnId = "b", siblings = emptyList(), topLevelTasks = board),
        )
    }

    @Test
    fun `moveNeighbors keeps a subtask in its sibling order`() {
        val subs = listOf(task("s1", "a", parentId = "p"), task("s2", "a", parentId = "p"))
        assertEquals(
            MoveNeighbors(beforeId = "s1"),
            moveNeighbors("s2", parentId = "p", targetColumnId = "b", siblings = subs, topLevelTasks = emptyList()),
        )
    }
}
