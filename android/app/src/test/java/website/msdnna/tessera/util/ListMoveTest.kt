package website.msdnna.tessera.util

import org.junit.Assert.assertEquals
import org.junit.Assert.assertSame
import org.junit.Test

/** Pure reorder behind the composer's sort-chip drag (see [moveItem]). */
class ListMoveTest {
    private val base = listOf("a", "b", "c", "d")

    @Test
    fun `moves an element forward, shifting the ones it passes`() {
        assertEquals(listOf("b", "c", "a", "d"), base.moveItem(0, 2))
    }

    @Test
    fun `moves an element backward`() {
        assertEquals(listOf("a", "d", "b", "c"), base.moveItem(3, 1))
    }

    @Test
    fun `moves to the last index`() {
        assertEquals(listOf("b", "c", "d", "a"), base.moveItem(0, 3))
    }

    @Test
    fun `moving onto itself is a no-op`() {
        assertSame(base, base.moveItem(2, 2))
    }

    @Test
    fun `moving between neighbours swaps them`() {
        assertEquals(listOf("b", "a", "c", "d"), base.moveItem(1, 0))
    }

    /** A drop resolved against a stale index (a level removed mid-gesture) must not
     *  throw — the drag simply does nothing. */
    @Test
    fun `out-of-range indices leave the list untouched`() {
        assertSame(base, base.moveItem(0, 9))
        assertSame(base, base.moveItem(-1, 1))
        assertSame(base, base.moveItem(4, 0))
        assertSame(emptyList<String>(), emptyList<String>().moveItem(0, 0))
    }
}
