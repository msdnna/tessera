package website.msdnna.tessera.e2e

import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performImeAction
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextInput
import com.google.common.truth.Truth.assertThat
import org.junit.Rule
import org.junit.Test
import org.junit.rules.RuleChain
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import website.msdnna.tessera.ui.TestTags

/**
 * The board's own create path: a card from a column footer and a column from the
 * end-of-board tile, both driven through the real inline fields against the real
 * backend.
 *
 * Every write is asserted twice — the server has it, and the board shows it. The
 * two halves catch different defects: the board renders optimistically, so a
 * request that failed still paints a card (UI-only assertion passes, data is
 * gone), while a write that persists but never reaches the board state leaves the
 * user staring at a stale screen (server-only assertion passes).
 */
@RunWith(RobolectricTestRunner::class)
class BoardCrudE2eTest {
    private val e2e = E2eRule()
    private val compose = createComposeRule()

    @get:Rule
    val rules: RuleChain = RuleChain.outerRule(e2e).around(compose)

    @Test
    fun `the seeded board renders every column and a card it did not create`() {
        // Created over the API before the screen mounts: proves the board renders
        // what the backend has, independently of the app's own create path.
        val outsider = E2eBackend.createTask(e2e.fixture, "card from the API")

        compose.setBoardContent(e2e.fixture)

        e2e.fixture.columns.forEach { compose.awaitTag(TestTags.boardColumn(it.id)) }
        compose.awaitTag(TestTags.taskCard(outsider.id))
    }

    @Test
    fun `a card created from the column footer reaches the backend and the board`() {
        compose.setBoardContent(e2e.fixture)
        val column = e2e.fixture.firstColumn
        val title = "card from the footer ${System.nanoTime()}"

        compose.onNodeWithTag(TestTags.columnAddTask(column.id)).performClick()
        compose.awaitTag(TestTags.columnTaskInput(column.id))
        compose.onNodeWithTag(TestTags.columnTaskInput(column.id)).performTextInput(title)
        // The field commits on the IME action (or on focus loss) — it has no
        // button, mirroring the web's inline inputs.
        compose.onNodeWithTag(TestTags.columnTaskInput(column.id)).performImeAction()

        val created = compose.awaitServer("the new card to appear on the board") {
            E2eBackend.tasks(e2e.fixture).firstOrNull { it.title == title }
        }
        assertThat(created.columnId).isEqualTo(column.id)
        // The id only exists server-side, so this also proves the card on screen is
        // the persisted one and not just the optimistic placeholder.
        compose.awaitTag(TestTags.taskCard(created.id))
    }

    @Test
    fun `a column created from the end-of-board tile reaches the backend and the board`() {
        compose.setBoardContent(e2e.fixture)
        val name = "column ${System.nanoTime()}"

        // The tile sits past the last column of a board wider than the screen. It
        // is composed (the lane row is a plain scrolling Row, not lazy), but a tap
        // outside the window is silently dropped rather than rejected — hence the
        // explicit scroll before the click.
        compose.onNodeWithTag(TestTags.BOARD_ADD_COLUMN).performScrollTo().performClick()
        compose.awaitTag(TestTags.BOARD_COLUMN_INPUT)
        compose.onNodeWithTag(TestTags.BOARD_COLUMN_INPUT).performTextInput(name)
        compose.onNodeWithTag(TestTags.BOARD_COLUMN_INPUT).performImeAction()

        val created = compose.awaitServer("the new column to appear on the board") {
            E2eBackend.columns(e2e.fixture).firstOrNull { it.name == name }
        }
        // A fifth lane, i.e. appended rather than replacing one of the defaults.
        assertThat(E2eBackend.columns(e2e.fixture)).hasSize(e2e.fixture.columns.size + 1)
        compose.awaitTag(TestTags.boardColumn(created.id))
    }
}
