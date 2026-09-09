package website.msdnna.tessera.util

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import website.msdnna.tessera.data.model.Task

/**
 * Board counters (#2850/#2851). Cases mirror `frontend/tests/cx-board-counts.spec.js`
 * so both clients count the same way — the counting is where this goes wrong quietly:
 * the board's subtask map is flat and holds every nesting level, so a shallow count
 * under-reports, and a `parent_id` chain that loops back on itself would recurse
 * until the stack ends.
 */
class BoardCountTest {
    private fun task(id: String) = Task(id = id, title = id)

    @Test
    fun `counts only the cards when they have no children`() {
        assertEquals(BoardCount(2, 2), countWithSubtasks(listOf(task("a"), task("b")), emptyMap()))
    }

    @Test
    fun `is zero for an empty column`() {
        assertEquals(BoardCount(0, 0), countWithSubtasks(emptyList(), emptyMap()))
    }

    @Test
    fun `walks every nesting level, not just the direct children`() {
        // a → a1 → a11, plus a2; b → b1. Cards: 2, tree: 6.
        val subs = mapOf(
            "a" to listOf(task("a1"), task("a2")),
            "a1" to listOf(task("a11")),
            "b" to listOf(task("b1")),
        )
        assertEquals(BoardCount(2, 6), countWithSubtasks(listOf(task("a"), task("b")), subs))
    }

    @Test
    fun `ignores children of a parent that is not in this column`() {
        // The map is board-wide: another column's parent must not leak into this count.
        val subs = mapOf("a" to listOf(task("a1")), "z" to listOf(task("z1"), task("z2")))
        assertEquals(BoardCount(1, 2), countWithSubtasks(listOf(task("a")), subs))
    }

    @Test
    fun `honours a narrowed child list and keeps descending below it`() {
        // The caller hands in one merged map: the composer's narrowed list for the
        // parents it filtered (a keeps only a1), the raw board map for the levels
        // below (a1 → a11), which the filtered map never carries.
        val subs = mapOf("a" to listOf(task("a1")), "a1" to listOf(task("a11")))
        assertEquals(BoardCount(1, 3), countWithSubtasks(listOf(task("a")), subs))
    }

    @Test
    fun `terminates on a parent chain that loops back on itself`() {
        val subs = mapOf("a" to listOf(task("a1")), "a1" to listOf(task("a")))
        assertEquals(BoardCount(1, 2), countWithSubtasks(listOf(task("a")), subs))
    }

    @Test
    fun `counts a card carrying two column tags once`() {
        // Tag grouping puts such a card in two lanes; the composer counter is fed the
        // board-wide list, and the same card arriving twice must not double the total.
        val subs = mapOf("a" to listOf(task("a1")))
        assertEquals(BoardCount(1, 2), countWithSubtasks(listOf(task("a"), task("a")), subs))
    }

    @Test
    fun `hasSubtasks gates the bracketed half of the label`() {
        assertTrue(BoardCount(5, 12).hasSubtasks)
        // «5 (5)» would be noise, not information.
        assertFalse(BoardCount(5, 5).hasSubtasks)
    }
}
