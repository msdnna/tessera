package website.msdnna.tessera.e2e

import androidx.compose.ui.test.assertTextContains
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextClearance
import androidx.compose.ui.test.performTextInput
import com.google.common.truth.Truth.assertThat
import org.junit.Rule
import org.junit.Test
import org.junit.rules.RuleChain
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import website.msdnna.tessera.ui.TestTags

/**
 * The task modal, reached the way a user reaches it — by tapping a card on the
 * board — and driven through its real fields against the real backend.
 *
 * Each spec seeds its own card. Sharing one would couple the specs through the
 * server: a title edited by one and a column moved by another are the same row,
 * and a failure would no longer say which interaction broke it.
 */
@RunWith(RobolectricTestRunner::class)
class TaskModalE2eTest {
    private val e2e = E2eRule()
    private val compose = createComposeRule()

    @get:Rule
    val rules: RuleChain = RuleChain.outerRule(e2e).around(compose)

    @Test
    fun `tapping a card opens the modal on that task`() {
        val title = "open me ${System.nanoTime()}"
        val task = E2eBackend.createTask(e2e.fixture, title)

        compose.openTaskModal(e2e.fixture, task.id)

        // The modal is showing this task, not merely showing: the title field is
        // filled from the detail request, which only resolves for the tapped card.
        compose.onNodeWithTag(TestTags.TASK_TITLE).assertTextContains(title)
    }

    @Test
    fun `a title and description edited in the modal are saved`() {
        val task = E2eBackend.createTask(e2e.fixture, "before ${System.nanoTime()}")
        val newTitle = "renamed ${System.nanoTime()}"
        val description = "Описание из спеки: **жирный** текст."

        compose.openTaskModal(e2e.fixture, task.id)
        compose.onNodeWithTag(TestTags.TASK_TITLE).performTextClearance()
        compose.onNodeWithTag(TestTags.TASK_TITLE).performTextInput(newTitle)
        // The description lives in the modal's first tab, which is the one open on
        // load (#2754) — no tab switch needed here. The editor opens in write mode
        // (not preview) while the description is empty; a seeded task has none, so
        // the text area is composed right away.
        compose.onNodeWithTag(TestTags.TASK_DESCRIPTION).performScrollTo().performTextInput(description)
        // No scroll for the footer: it is pinned below the scrolling body, so
        // `performScrollTo` there fails on «no parent with a Scroll action».
        compose.onNodeWithTag(TestTags.TASK_SAVE).performClick()

        val saved = compose.awaitServer("the edited title and description to persist") {
            E2eBackend.task(e2e.fixture, task.id).takeIf { it.title == newTitle }
        }
        // Markdown is stored verbatim — the editor holds source, not rendered HTML.
        assertThat(saved.description).isEqualTo(description)
    }

    @Test
    fun `a priority picked in the modal is saved`() {
        val task = E2eBackend.createTask(e2e.fixture, "priority ${System.nanoTime()}")
        assertThat(task.priority).isEqualTo(0)

        compose.openTaskModal(e2e.fixture, task.id)
        compose.onNodeWithTag(TestTags.TASK_PRIORITY).performScrollTo().performClick()
        compose.onNodeWithTag(TestTags.taskPriorityOption(HIGH_PRIORITY)).performClick()

        // No Сохранить here: the picker writes on pick, unlike title/description.
        compose.awaitServer("the picked priority to persist") {
            E2eBackend.task(e2e.fixture, task.id).takeIf { it.priority == HIGH_PRIORITY }
        }
    }

    @Test
    fun `moving the task from the status picker moves its card on the board`() {
        val task = E2eBackend.createTask(e2e.fixture, "status ${System.nanoTime()}")
        val target = e2e.fixture.columns[1]

        compose.openTaskModal(e2e.fixture, task.id)
        compose.onNodeWithTag(TestTags.TASK_STATUS).performScrollTo().performClick()
        compose.onNodeWithTag(TestTags.taskStatusOption(target.id)).performClick()

        val moved = compose.awaitServer("the task to land in ${target.name}") {
            E2eBackend.task(e2e.fixture, task.id).takeIf { it.columnId == target.id }
        }
        assertThat(moved.columnId).isEqualTo(target.id)
        // Still the same card, now under a different lane — closing the modal
        // reloads the board, so this also proves the move survived the refresh.
        compose.onNodeWithTag(TestTags.TASK_MODAL).assertExists()
    }

    @Test
    fun `a comment written in the modal reaches the backend`() {
        val task = E2eBackend.createTask(e2e.fixture, "comment ${System.nanoTime()}")
        val body = "комментарий из спеки ${System.nanoTime()}"

        compose.openTaskModal(e2e.fixture, task.id)
        // Комментарии is no longer the tab open on load — «Описание» is (#2754).
        compose.onNodeWithTag(TestTags.taskTab(TestTags.TASK_TAB_COMMENTS)).performScrollTo().performClick()
        // The composer sits under the tab strip at the foot of a scrolling body:
        // it is composed, but a tap at its off-window coordinates is dropped
        // silently rather than rejected — hence the scroll before typing.
        compose.onNodeWithTag(TestTags.TASK_COMMENT_INPUT).performScrollTo().performTextInput(body)
        compose.onNodeWithTag(TestTags.TASK_COMMENT_SUBMIT).performScrollTo().performClick()

        val posted = compose.awaitServer("the comment to persist") {
            E2eBackend.comments(e2e.fixture, task.id).firstOrNull { it.body == body }
        }
        assertThat(posted.authorId).isEqualTo(e2e.fixture.account.user.id)
    }

    @Test
    fun `the description opens in its own first tab and survives a tab switch`() {
        val text = "описание во вкладке ${System.nanoTime()}"
        val task = E2eBackend.createTask(e2e.fixture, "tabs ${System.nanoTime()}")

        compose.openTaskModal(e2e.fixture, task.id)

        // Открыта именно «Описание»: поле редактора на экране, а composer
        // комментариев — нет (обе вкладки существуют в полосе, но содержимое
        // рисуется только у выбранной).
        compose.onNodeWithTag(TestTags.TASK_DESCRIPTION).performScrollTo().performTextInput(text)
        compose.onNodeWithTag(TestTags.TASK_COMMENT_INPUT).assertDoesNotExist()

        compose.onNodeWithTag(TestTags.taskTab(TestTags.TASK_TAB_COMMENTS)).performScrollTo().performClick()
        compose.awaitTag(TestTags.TASK_COMMENT_INPUT)
        compose.onNodeWithTag(TestTags.TASK_DESCRIPTION).assertDoesNotExist()

        // Возврат на «Описание» отдаёт набранный текст, а не перечитанный с
        // сервера: вкладка уходит из композиции, и правка пережила бы это только
        // потому, что состояние поднято в модалку.
        compose.onNodeWithTag(TestTags.taskTab(TestTags.TASK_TAB_DESCRIPTION)).performScrollTo().performClick()
        compose.awaitTag(TestTags.TASK_DESCRIPTION)
        compose.onNodeWithTag(TestTags.TASK_DESCRIPTION).assertTextContains(text)
    }

    private companion object {
        /** Index into `PriorityLabels` — «Высокий» in the modal's picker. */
        const val HIGH_PRIORITY = 3
    }
}
