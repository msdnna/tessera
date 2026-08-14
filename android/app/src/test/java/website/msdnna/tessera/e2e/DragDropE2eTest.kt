package website.msdnna.tessera.e2e

import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.test.junit4.createComposeRule
import com.google.common.truth.Truth.assertThat
import org.junit.Rule
import org.junit.Test
import org.junit.rules.RuleChain
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import website.msdnna.tessera.data.model.Task
import website.msdnna.tessera.ui.TestTags

/**
 * The board's drag-and-drop: reordering inside a column, nesting one card under
 * another, and moving a card to the next column.
 *
 * Nothing here is optimistic — the board never reorders locally, it asks the
 * backend and redraws from the answer (`DND_ARCHITECTURE.md` §1). So every spec
 * asserts against the server's own view of the board: that is the only place the
 * new order actually exists.
 *
 * Geometry is read from the live layout rather than assumed, and every drag is
 * aimed so that the dragged card collapsing (`dragCollapse`) cannot move the
 * target out from under the finger — see the note on each spec.
 */
@RunWith(RobolectricTestRunner::class)
class DragDropE2eTest {
    private val e2e = E2eRule()
    private val compose = createComposeRule()

    @get:Rule
    val rules: RuleChain = RuleChain.outerRule(e2e).around(compose)

    /** Board order as the server has it — the reference every spec checks. */
    private fun serverOrder(): List<String> =
        E2eBackend.tasks(e2e.fixture).sortedBy { it.position }.map { it.title }

    private fun seedCards(vararg titles: String): List<Task> =
        titles.map { E2eBackend.createTask(e2e.fixture, it) }

    @Test
    fun `a card dragged below the others is reordered on the server`() {
        seedCards("drag-a", "drag-b", "drag-c")
        compose.setBoardContent(e2e.fixture)
        assertThat(serverOrder()).containsExactly("drag-a", "drag-b", "drag-c").inOrder()

        // Aimed at the column's own footer tile — past every card, but still inside
        // the column body that a drop is resolved against. Anywhere higher is a
        // false target: the 8dp gaps between cards are too thin to hit reliably,
        // and a point that misses one lands on a card body, which the board reads
        // as nesting rather than as reordering.
        val ids = serverIds()
        val lastCard = compose.windowRect(TestTags.taskCard(ids[2]))
        val footer = compose.windowRect(TestTags.columnAddTask(e2e.fixture.firstColumn.id))
        val target = Offset(lastCard.center.x, footer.center.y - compose.collapseShift(ids[0]))
        compose.dragCardTo(ids[0], target)

        compose.awaitServer("drag-a to be reordered to the end") {
            serverOrder().takeIf { it.last() == "drag-a" }
        }
        assertThat(serverOrder()).containsExactly("drag-b", "drag-c", "drag-a").inOrder()
    }

    @Test
    fun `a card dragged onto another card becomes its subtask`() {
        seedCards("nest-parent", "nest-child")
        compose.setBoardContent(e2e.fixture)

        // The child is dragged UPWARDS onto the card above it on purpose: a
        // collapsing card only reflows what sits below it, so the target card does
        // not move while the drag is in flight and the finger lands where aimed.
        val parentId = serverIds()[0]
        val childId = serverIds()[1]
        compose.dragCardTo(childId, compose.windowRect(TestTags.taskCard(parentId)).center)

        val nested = compose.awaitServer("nest-child to become a subtask") {
            E2eBackend.task(e2e.fixture, childId).takeIf { it.parentId != null }
        }
        assertThat(nested.parentId).isEqualTo(parentId)
        // A subtask is no longer a card of its own on the board: the endpoint the
        // board reads returns top-level tasks only.
        assertThat(serverOrder()).containsExactly("nest-parent")
    }

    @Test
    fun `a card dragged into the next column changes column on the server`() {
        val card = seedCards("cross-column").single()
        val target = e2e.fixture.columns[1]
        compose.setBoardContent(e2e.fixture)

        // Columns are laid out a fixed margin short of the screen width so the next
        // one peeks in at the right edge (`BoardViews.kt` peekWidth) — that peeking
        // strip is the only part of it a finger can reach. The x is clamped inside
        // the window: a pointer pushed past the right edge leaves the board instead
        // of landing in the column.
        //
        // The y comes from the target column's own footer rather than from the
        // dragged card's row, because a drop is resolved against the column's body
        // (the node carrying `dropColumn`), which wraps its content — an empty
        // column is barely taller than its header, so the height the dragged card
        // sits at is already past its bottom edge.
        val strip = compose.windowRect(TestTags.boardColumn(target.id))
        val footer = compose.windowRect(TestTags.columnAddTask(target.id))
        val x = minOf(strip.left + strip.width * 0.05f, compose.windowWidthPx() - 2f)
        compose.dragCardTo(card.id, Offset(x, footer.center.y))

        val moved = compose.awaitServer("cross-column to land in the second column") {
            E2eBackend.task(e2e.fixture, card.id).takeIf { it.columnId != e2e.fixture.firstColumn.id }
        }
        assertThat(moved.columnId).isEqualTo(target.id)
        compose.awaitTag(TestTags.taskCard(card.id))
    }

    /** Ids of the board's top-level cards in server order. */
    private fun serverIds(): List<String> =
        E2eBackend.tasks(e2e.fixture).sortedBy { it.position }.map { it.id }
}
