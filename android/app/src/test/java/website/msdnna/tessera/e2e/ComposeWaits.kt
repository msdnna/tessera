package website.msdnna.tessera.e2e

import androidx.compose.ui.test.ExperimentalTestApi
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.junit4.ComposeContentTestRule

/** How long a spec waits on a screen that is round-tripping to the backend. */
private const val AWAIT_TIMEOUT_MS = 20_000L

/**
 * Waits for exactly one node carrying [tag] to exist.
 *
 * Specs run against a live server, so every assertion that follows a click has
 * to outlast a real HTTP round trip — `assertIsDisplayed()` on its own would
 * check the tree one frame after the tap and fail on latency rather than on a
 * defect. Insisting on a *single* match also catches a screen that composed
 * twice, which otherwise surfaces much later as a confusing "multiple nodes".
 */
@OptIn(ExperimentalTestApi::class)
fun ComposeContentTestRule.awaitTag(tag: String, timeoutMillis: Long = AWAIT_TIMEOUT_MS) {
    waitUntilExactlyOneExists(hasTestTag(tag), timeoutMillis)
}
