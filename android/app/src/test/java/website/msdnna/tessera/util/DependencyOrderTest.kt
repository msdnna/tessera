package website.msdnna.tessera.util

import org.junit.Assert.assertEquals
import org.junit.Assert.assertSame
import org.junit.Assert.assertTrue
import org.junit.Test
import website.msdnna.tessera.data.model.Task

/** Mirrors web `frontend/tests/dependencyOrder.spec.js` — the "Авто" Gantt row order. */
class DependencyOrderTest {

    private fun tasks(vararg ids: String) = ids.map { Task(id = it, title = it) }

    private fun ids(out: List<Task>) = out.map { it.id }

    @Test
    fun `places a dependent right after its blocker, before the next root`() {
        // A blocks Б; В sat right after A in the incoming order → expect A, Б, В.
        val out = topoByDeps(tasks("A", "Б", "В"), listOf("A" to "Б"))
        assertEquals(listOf("A", "Б", "В"), ids(out))
    }

    @Test
    fun `returns the list unchanged when there are no deps`() {
        val list = tasks("A", "Б", "В")
        assertSame(list, topoByDeps(list, emptyList()))
    }

    @Test
    fun `walks a chain depth-first`() {
        val out = topoByDeps(tasks("A", "B", "C"), listOf("A" to "B", "B" to "C"))
        assertEquals(listOf("A", "B", "C"), ids(out))
    }

    @Test
    fun `orders siblings of a node by incoming order`() {
        // A blocks both C and B; incoming order has B before C → A, B, C.
        val out = topoByDeps(tasks("A", "B", "C"), listOf("A" to "C", "A" to "B"))
        assertEquals(listOf("A", "B", "C"), ids(out))
    }

    @Test
    fun `keeps every task exactly once even with a cycle`() {
        val out = topoByDeps(tasks("A", "B", "C"), listOf("A" to "B", "B" to "A"))
        assertEquals(3, out.size)
        assertEquals(listOf("A", "B", "C"), ids(out).sorted())
    }

    @Test
    fun `ignores edges pointing at absent tasks`() {
        val out = topoByDeps(tasks("A", "B"), listOf("A" to "ghost"))
        assertEquals(listOf("A", "B"), ids(out))
    }

    @Test
    fun `emits a shared dependent only once, after ALL of its blockers (diamond)`() {
        // A→B, A→C, B→D, C→D. D must appear once — and below C, not between B and C:
        // the Gantt draws finish→start arrows between rows, so a dependent placed above
        // one of its blockers gets an arrow pointing back up the chart.
        val out = topoByDeps(
            tasks("A", "B", "C", "D"),
            listOf("A" to "B", "A" to "C", "B" to "D", "C" to "D"),
        )
        assertEquals(listOf("A", "B", "C", "D"), ids(out))
    }

    @Test
    fun `holds a task with two blockers until the second one is emitted`() {
        // The minimal shape of the same rule: A and B both block C.
        val out = topoByDeps(tasks("A", "B", "C"), listOf("A" to "C", "B" to "C"))
        assertEquals(listOf("A", "B", "C"), ids(out))
    }

    @Test
    fun `never places a task above one of its blockers`() {
        // Two roots feeding a joined tail, with the incoming order deliberately
        // interleaved so a first-blocker-wins walk would emit Г before В.
        val out = topoByDeps(
            tasks("A", "Б", "В", "Г"),
            listOf("A" to "Г", "В" to "Г", "Б" to "В"),
        )
        val at = { id: String -> ids(out).indexOf(id) }
        assertTrue(at("Г") > at("A"))
        assertTrue(at("Г") > at("В"))
        assertTrue(at("В") > at("Б"))
    }
}
