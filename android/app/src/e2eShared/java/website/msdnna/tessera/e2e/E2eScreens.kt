package website.msdnna.tessera.e2e

import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.Surface
import androidx.compose.ui.Modifier
import androidx.compose.ui.test.junit4.ComposeContentTestRule
import androidx.compose.ui.test.onAllNodesWithTag
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import website.msdnna.tessera.ui.TestTags
import website.msdnna.tessera.ui.screens.BoardScreen
import website.msdnna.tessera.ui.screens.DocumentsScreen
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

/**
 * Mounts the documents section and waits until the tree has loaded from the
 * backend ([anchorId]'s row is on screen).
 *
 * Composed directly, for the same reason [setBoardContent] is: reaching the
 * section from Home means walking the drawer, and folding navigation into every
 * documents spec would give each one a second way to fail.
 */
fun ComposeContentTestRule.setDocumentsContent(fixture: E2eBackend.Fixture, anchorId: String) {
    setContent {
        TesseraTheme {
            Surface(Modifier.fillMaxSize(), color = Tessera.colors.bg) {
                DocumentsScreen(workspaceId = fixture.workspace.id)
            }
        }
    }
    awaitTag(TestTags.documentRow(anchorId))
}

/**
 * Switches the board's grouping through the composer's chip menu — the way a
 * user does it, rather than by calling `BoardViewModel.setGrouping` directly.
 *
 * The bar starts collapsed, and while it is, a transparent overlay swallows every
 * tap and only expands it (`BoardComposerBar`) — so the first tap on the chip
 * would silently expand the bar instead of opening the menu. Expanding is
 * therefore an explicit first step, conditional because the bar stays expanded
 * after a pick and a spec may switch grouping twice.
 *
 * The menu row lives in a [androidx.compose.ui.window.Popup] composed only while
 * the menu is open, hence the wait between the two taps.
 */
fun ComposeContentTestRule.selectGrouping(optionTag: String) {
    if (onAllNodesWithTag(TestTags.BOARD_COMPOSER_EXPAND).fetchSemanticsNodes().isNotEmpty()) {
        onNodeWithTag(TestTags.BOARD_COMPOSER_EXPAND).performClick()
        awaitNoTag(TestTags.BOARD_COMPOSER_EXPAND)
    }
    onNodeWithTag(TestTags.BOARD_GROUP).performClick()
    awaitTag(optionTag)
    onNodeWithTag(optionTag).performClick()
}
