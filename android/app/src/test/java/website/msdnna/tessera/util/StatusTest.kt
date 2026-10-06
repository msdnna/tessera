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
    fun `moveNeighbors names no neighbour for a top-level card`() {
        // The regression: the tail was picked from the board's loaded cards, which
        // under a sprint scope are a slice of the column. Anything named here is one
        // card out of that slice, and beforeId alone lands the card at prev + 65536 —
        // the next card's slot on an untouched ladder. Only the server sees the whole
        // column, so the modal names nothing and lets it append.
        val board = listOf(task("t1", "a", position = 10.0), task("t2", "b", position = 20.0))
        assertEquals(MoveNeighbors(), moveNeighbors("t1", parentId = null, siblings = emptyList()))
        assertEquals(MoveNeighbors(), moveNeighbors("t1", parentId = null, siblings = board))
    }

    @Test
    fun `moveNeighbors keeps a subtask in its sibling order`() {
        val subs = listOf(task("s1", "a", parentId = "p"), task("s2", "a", parentId = "p"))
        assertEquals(
            MoveNeighbors(beforeId = "s1"),
            moveNeighbors("s2", parentId = "p", siblings = subs),
        )
    }
}
