package website.msdnna.tessera.smoke

import androidx.activity.ComponentActivity
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import com.google.common.truth.Truth.assertThat
import org.junit.Rule
import org.junit.Test
import org.junit.rules.RuleChain
import website.msdnna.tessera.e2e.E2eBackend
import website.msdnna.tessera.e2e.E2eRule
import website.msdnna.tessera.e2e.awaitServer
import website.msdnna.tessera.e2e.columnEndTarget
import website.msdnna.tessera.e2e.dragCardTo
import website.msdnna.tessera.e2e.setBoardContent

/**
 * A card dragged with a real finger on a real touch screen.
 *
 * Of everything the instrumented tier covers, this is the one that most earns its
 * minutes. The board arms dragging with `detectDragGesturesAfterLongPress`
 * (`DND_ARCHITECTURE.md` §3), so the gesture depends on event timing and on the
 * platform's touch slop — precisely what the JVM tier replaces with an injected
 * clock and a simulated pointer. A drag that works under Robolectric and not under
 * a thumb would look green all the way to the store.
 */
class DragDropSmokeTest {
    private val e2e = E2eRule()
    private val compose = createAndroidComposeRule<ComponentActivity>()

    @get:Rule
    val rules: RuleChain = RuleChain.outerRule(e2e).around(compose)

    @Test
    fun aCardDraggedBelowTheOthersIsReorderedOnTheServer() {
        listOf("smoke-drag-a", "smoke-drag-b").forEach { E2eBackend.createTask(e2e.fixture, it) }
        compose.setBoardContent(e2e.fixture)
        assertThat(serverOrder()).containsExactly("smoke-drag-a", "smoke-drag-b").inOrder()

        val ids = serverIds()
        compose.dragCardTo(ids[0], compose.columnEndTarget(e2e.fixture.firstColumn.id))

        // The board never reorders locally — it asks the backend and redraws from
        // the answer (`DND_ARCHITECTURE.md` §1), so the server is the only place
        // the new order exists.
        compose.awaitServer("smoke-drag-a to be reordered to the end") {
            serverOrder().takeIf { it.last() == "smoke-drag-a" }
        }
        assertThat(serverOrder()).containsExactly("smoke-drag-b", "smoke-drag-a").inOrder()
    }

    /** Board order as the server has it. */
    private fun serverOrder(): List<String> =
        E2eBackend.tasks(e2e.fixture).sortedBy { it.position }.map { it.title }

    /** Ids of the board's top-level cards in server order. */
    private fun serverIds(): List<String> =
        E2eBackend.tasks(e2e.fixture).sortedBy { it.position }.map { it.id }
}
