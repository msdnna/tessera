package website.msdnna.tessera.smoke

import androidx.activity.ComponentActivity
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performImeAction
import androidx.compose.ui.test.performTextInput
import com.google.common.truth.Truth.assertThat
import org.junit.Rule
import org.junit.Test
import org.junit.rules.RuleChain
import website.msdnna.tessera.e2e.E2eBackend
import website.msdnna.tessera.e2e.E2eRule
import website.msdnna.tessera.e2e.awaitServer
import website.msdnna.tessera.e2e.awaitTag
import website.msdnna.tessera.e2e.openTaskModal
import website.msdnna.tessera.e2e.setBoardContent
import website.msdnna.tessera.ui.TestTags

/**
 * The board rendered and driven on a real device: it draws, a card opens on tap,
 * and the column footer creates one through the real soft keyboard.
 *
 * Hosted by a bare [ComponentActivity] (supplied by `ui-test-manifest`) rather
 * than by [website.msdnna.tessera.MainActivity], for the same reason the JVM tier
 * mounts the board directly: the app boots on Home and reaching a board means
 * walking the sidebar drawer, so routing every board spec through navigation would
 * make it fail for two unrelated reasons. Reaching the board from Home is its own
 * spec, and does not exist yet — stated plainly rather than implied.
 */
class BoardSmokeTest {
    private val e2e = E2eRule()
    private val compose = createAndroidComposeRule<ComponentActivity>()

    @get:Rule
    val rules: RuleChain = RuleChain.outerRule(e2e).around(compose)

    @Test
    fun theBoardRendersEveryColumnAndACardItDidNotCreate() {
        val outsider = E2eBackend.createTask(e2e.fixture, "smoke card from the API")

        compose.setBoardContent(e2e.fixture)

        e2e.fixture.columns.forEach { compose.awaitTag(TestTags.boardColumn(it.id)) }
        compose.awaitTag(TestTags.taskCard(outsider.id))
    }

    @Test
    fun tappingACardOpensItsModal() {
        val card = E2eBackend.createTask(e2e.fixture, "smoke card to open")

        compose.openTaskModal(e2e.fixture, card.id)

        compose.awaitTag(TestTags.TASK_MODAL)
    }

    @Test
    fun aCardCreatedFromTheColumnFooterReachesTheBackend() {
        compose.setBoardContent(e2e.fixture)
        val column = e2e.fixture.firstColumn
        val title = "smoke footer card ${System.nanoTime()}"

        compose.onNodeWithTag(TestTags.columnAddTask(column.id)).performClick()
        compose.awaitTag(TestTags.columnTaskInput(column.id))
        compose.onNodeWithTag(TestTags.columnTaskInput(column.id)).performTextInput(title)
        compose.onNodeWithTag(TestTags.columnTaskInput(column.id)).performImeAction()

        val created = compose.awaitServer("the new card to reach the backend") {
            E2eBackend.tasks(e2e.fixture).firstOrNull { it.title == title }
        }
        assertThat(created.columnId).isEqualTo(column.id)
        compose.awaitTag(TestTags.taskCard(created.id))
    }
}
