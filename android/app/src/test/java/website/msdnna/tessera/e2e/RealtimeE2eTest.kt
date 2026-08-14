package website.msdnna.tessera.e2e

import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import org.junit.Rule
import org.junit.Test
import org.junit.rules.RuleChain
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import website.msdnna.tessera.ui.TestTags

/**
 * The board updates itself when the data changes somewhere else.
 *
 * Every change here is made straight against the backend while the screen is
 * already mounted, and nothing in the spec touches the app afterwards — so the
 * only thing that can bring the change onto the screen is the websocket the
 * board subscribes to (`BoardViewModel.ensureRealtime`). A spec that clicked
 * anything after the write would pass on the refresh that click triggers and say
 * nothing about realtime.
 *
 * The whole chain is real: Go's workspace-scoped fan-out hub, an OkHttp
 * websocket, the debounce, and the silent reload that follows.
 */
@RunWith(RobolectricTestRunner::class)
class RealtimeE2eTest {
    private val e2e = E2eRule()
    private val compose = createComposeRule()

    @get:Rule
    val rules: RuleChain = RuleChain.outerRule(e2e).around(compose)

    @Test
    fun `a card created elsewhere appears on the open board`() {
        compose.setBoardContent(e2e.fixture)

        val pushed = E2eBackend.createTask(e2e.fixture, "pushed over the socket")

        compose.awaitTag(TestTags.taskCard(pushed.id))
    }

    @Test
    fun `a card renamed elsewhere is redrawn with its new title`() {
        val card = E2eBackend.createTask(e2e.fixture, "before the rename")
        compose.setBoardContent(e2e.fixture)
        compose.awaitTag(TestTags.taskCard(card.id))

        val title = "renamed over the socket"
        E2eBackend.renameTask(e2e.fixture, card.id, title)

        // Asserted on the card's own subtree, not on the screen at large: the title
        // is the card's data, and a board-wide text match would also be satisfied by
        // a toast about the change rather than by the card actually being redrawn.
        compose.awaitTextIn(TestTags.taskCard(card.id), title)
    }

    @Test
    fun `a column created elsewhere appears on the open board`() {
        compose.setBoardContent(e2e.fixture)

        val added = E2eBackend.createColumn(e2e.fixture, "lane over the socket")

        compose.awaitTag(TestTags.boardColumn(added.id))
        // The new lane is a working column, not just a drawn one: its inline
        // create field opens, which is what the board wires up per column from the
        // reloaded state.
        //
        // The lane is scrolled into view first, and it is the lane that has to be
        // scrolled to — not the tile inside it. A fifth column lands past the right
        // edge of a board already wider than the screen, and a tap outside the
        // window is dropped silently rather than rejected, so the click would look
        // like it did nothing. Scrolling to the tile does not help: it sits inside
        // the column's vertical list, so the scroll is spent on that list while the
        // board's horizontal offset — the one actually hiding it — stays put.
        compose.onNodeWithTag(TestTags.boardColumn(added.id)).performScrollTo()
        compose.onNodeWithTag(TestTags.columnAddTask(added.id)).performClick()
        compose.awaitTag(TestTags.columnTaskInput(added.id))
    }
}
