package website.msdnna.tessera.util

import org.junit.Assert.assertEquals
import org.junit.Test
import website.msdnna.tessera.data.model.DeleteCounts

/**
 * The counts in the delete confirmations (#3014). Cases mirror
 * `frontend/tests/cx-project-slug-taken.spec.js` (countsPhrase): same order,
 * same zero rule, so both clients name the same thing.
 */
class DeleteCountsTest {
    private val word = mapOf(CountKind.PROJECTS to "p", CountKind.BOARDS to "b", CountKind.TASKS to "t")
    private fun phrase(c: DeleteCounts?) = deleteCountsPhrase(
        c,
        part = { kind, n -> "$n${word.getValue(kind)}" },
        join = { list, last -> "$list & $last" },
    )

    @Test
    fun `names projects, boards and tasks in that order`() {
        assertEquals("4p, 23b & 612t", phrase(DeleteCounts(projects = 4, boards = 23, tasks = 612)))
        assertEquals("3b & 148t", phrase(DeleteCounts(boards = 3, tasks = 148)))
    }

    @Test
    fun `leaves zero parts out`() {
        assertEquals("2b", phrase(DeleteCounts(boards = 2)))
        assertEquals("1p & 5t", phrase(DeleteCounts(projects = 1, tasks = 5)))
    }

    @Test
    fun `is empty when there is nothing to name`() {
        assertEquals("", phrase(DeleteCounts()))
        assertEquals("", phrase(null))
    }
}
