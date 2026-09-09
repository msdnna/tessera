package website.msdnna.tessera.e2e

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.Surface
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.test.junit4.ComposeContentTestRule
import androidx.compose.ui.test.onAllNodesWithTag
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import website.msdnna.tessera.ui.TestTags
import website.msdnna.tessera.ui.screens.BoardScreen
import website.msdnna.tessera.ui.screens.DocumentsScreen
import website.msdnna.tessera.ui.screens.documents.DocChrome
import website.msdnna.tessera.ui.screens.documents.DocTitleSwitcher
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
 *
 * The shell's top bar is part of the harness since #2894's rework: an open
 * document no longer draws a header of its own, it hands one up to
 * [website.msdnna.tessera.ui.screens.MainScreen] and the actions live behind
 * the title. Mounting the section alone would leave every one of them
 * unreachable — the specs would be testing a screen the app never shows.
 */
fun ComposeContentTestRule.setDocumentsContent(fixture: E2eBackend.Fixture, anchorId: String) {
    setContent {
        TesseraTheme {
            Surface(Modifier.fillMaxSize(), color = Tessera.colors.bg) {
                var chrome by remember { mutableStateOf<DocChrome?>(null) }
                Column(Modifier.fillMaxSize()) {
                    chrome?.let { DocTitleSwitcher(it) }
                    DocumentsScreen(workspaceId = fixture.workspace.id, onChrome = { chrome = it })
                }
            }
        }
    }
    awaitTag(TestTags.documentRow(anchorId))
}

/**
 * Opens the title menu and taps one of its items.
 *
 * The menu is a [androidx.compose.ui.window.Popup], composed only while it is
 * open, so its items cannot be waited on before the tap — hence the wait on
 * [TestTags.DOCUMENT_BACK], the one item that is always in it.
 *
 * The scroll is not decoration: the menu is longer than even the widened
 * Robolectric screen, and a tap below the fold is swallowed in silence — the
 * spec then fails on whatever the item was supposed to open, several lines
 * later, pointing at the wrong thing entirely.
 */
fun ComposeContentTestRule.pickDocMenu(tag: String) {
    onNodeWithTag(TestTags.DOCUMENT_MENU).performClick()
    awaitTag(TestTags.DOCUMENT_BACK)
    awaitTag(tag)
    onNodeWithTag(tag).performScrollTo().performClick()
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
