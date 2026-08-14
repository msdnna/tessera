package website.msdnna.tessera.e2e

import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.Surface
import androidx.compose.ui.Modifier
import androidx.compose.ui.test.junit4.ComposeContentTestRule
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
