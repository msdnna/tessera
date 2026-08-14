package website.msdnna.tessera.e2e

import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.Surface
import androidx.compose.ui.Modifier
import androidx.compose.ui.test.junit4.ComposeContentTestRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import website.msdnna.tessera.ui.TestTags
import website.msdnna.tessera.ui.screens.BoardScreen
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.TesseraTheme

/**
 * Mounts the seeded board and waits until it has actually loaded from the
 * backend (its leftmost column is on screen).
 *
 * The board is composed directly rather than through `AppRoot`, because
 * [website.msdnna.tessera.ui.screens.MainScreen] boots on Home and reaching a
 * board from there means walking the sidebar drawer. Everything below the screen
 * is still the real thing — [website.msdnna.tessera.ui.viewmodels.BoardViewModel],
 * Retrofit, the websocket, Postgres — so a board spec fails on board behaviour,
 * not on navigation. Navigation is a separate concern and belongs in its own spec
 * with its own anchors; folding it into every board spec would make each one fail
 * for two unrelated reasons.
 *
 * The theme wrapper is not decoration: `Tessera.colors` and the accent gradients
 * come from [TesseraTheme]'s composition locals, and a board composed without it
 * crashes before the first assertion.
 */
fun ComposeContentTestRule.setBoardContent(fixture: E2eBackend.Fixture) {
    setContent {
        TesseraTheme {
            Surface(Modifier.fillMaxSize(), color = Tessera.colors.bg) {
                BoardScreen(board = fixture.board, workspaceId = fixture.workspace.id)
            }
        }
    }
    awaitTag(TestTags.boardColumn(fixture.firstColumn.id))
}

/**
 * Mounts the board and opens [taskId] the way a user does — by tapping its card.
 *
 * Waiting on the title field rather than on the modal root is deliberate: the
 * modal composes immediately but shows a loader until
 * [website.msdnna.tessera.ui.viewmodels.TaskDetailViewModel] has fetched the task,
 * so a spec that only waited for the root would start typing into a screen that is
 * about to be replaced by the loaded one.
 */
fun ComposeContentTestRule.openTaskModal(fixture: E2eBackend.Fixture, taskId: String) {
    setBoardContent(fixture)
    onNodeWithTag(TestTags.taskCard(taskId)).performClick()
    awaitTag(TestTags.TASK_TITLE)
}
