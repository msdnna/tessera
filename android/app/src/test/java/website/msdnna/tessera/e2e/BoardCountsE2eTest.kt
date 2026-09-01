package website.msdnna.tessera.e2e

import androidx.compose.ui.test.junit4.createComposeRule
import org.junit.Rule
import org.junit.Test
import org.junit.rules.RuleChain
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import website.msdnna.tessera.ui.TestTags

/**
 * The column header «2 (4)» (#2853) and the composer bar's «показано: 2 (4)»
 * (#2854), against a real board with a real subtask tree.
 *
 * The unit tests pin the arithmetic; what only a live board can show is that the
 * numbers are fed from the board's *own* data — the flat, all-levels subtask list
 * the backend returns — rather than from whatever the screen happens to have
 * composed. Hence a tree deeper than one level: a counter wired to the cards' own
 * child lists would report 3 here and still look plausible.
 */
@RunWith(RobolectricTestRunner::class)
class BoardCountsE2eTest {
    private val e2e = E2eRule()
    private val compose = createComposeRule()

    @get:Rule
    val rules: RuleChain = RuleChain.outerRule(e2e).around(compose)

    @Test
    fun `the column header and the composer bar fold in every nesting level`() {
        val fixture = e2e.fixture
        // parent → child → grandchild, plus a flat card beside it: 2 cards, 4 tasks.
        val parent = E2eBackend.createTask(fixture, "counted parent ${System.nanoTime()}")
        val child = E2eBackend.createTask(fixture, "counted child", parentId = parent.id)
        E2eBackend.createTask(fixture, "counted grandchild", parentId = child.id)
        E2eBackend.createTask(fixture, "counted flat card ${System.nanoTime()}")

        compose.setBoardContent(fixture)

        // Waiting on the *text*, not on the node: the header composes as soon as the
        // columns arrive, while the cards and their subtasks land with a later
        // response — asserting once the node exists would race the data in and read «0».
        compose.awaitTextOn(TestTags.columnCount(fixture.firstColumn.id), "2 (4)")
        // The bar counts the whole board, so it agrees with the single seeded column.
        compose.awaitTextOn(TestTags.BOARD_COMPOSER_COUNT, "2 (4)")
    }

    @Test
    fun `a column whose cards have no subtasks keeps the bare number`() {
        val fixture = e2e.fixture
        E2eBackend.createTask(fixture, "flat only ${System.nanoTime()}")

        compose.setBoardContent(fixture)

        // «1 (1)» would be noise, not information (web ColumnHeader parity).
        compose.awaitTextOn(TestTags.columnCount(fixture.firstColumn.id), "1")
    }
}
