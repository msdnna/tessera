package website.msdnna.tessera.e2e

import androidx.compose.ui.test.ComposeTimeoutException
import androidx.compose.ui.test.ExperimentalTestApi
import androidx.compose.ui.test.hasAnyAncestor
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
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

/**
 * Waits for [text] to show up inside the subtree anchored by [tag].
 *
 * Scoped to one node rather than to the whole screen because the text being
 * waited on is data (a task's title), and the same string can legitimately appear
 * elsewhere — in a toast about the change, say — which would satisfy a screen-wide
 * match without the card itself having been redrawn.
 */
@OptIn(ExperimentalTestApi::class)
fun ComposeContentTestRule.awaitTextIn(tag: String, text: String, timeoutMillis: Long = AWAIT_TIMEOUT_MS) {
    waitUntilExactlyOneExists(hasAnyAncestor(hasTestTag(tag)) and hasText(text), timeoutMillis)
}

/**
 * Polls the backend until [read] returns non-null, then hands the value back.
 *
 * A UI action that writes is only half-done when the frame renders: the request
 * is still in flight. Polling the server for the row — rather than sleeping, or
 * trusting the screen — is what makes the assertion about persistence. [what]
 * lands in the timeout message, so a failure says which write never arrived
 * instead of just "condition never became true".
 */
fun <T> ComposeContentTestRule.awaitServer(
    what: String,
    timeoutMillis: Long = AWAIT_TIMEOUT_MS,
    read: () -> T?,
): T {
    var value: T? = null
    try {
        waitUntil(timeoutMillis) {
            value = read()
            value != null
        }
    } catch (e: ComposeTimeoutException) {
        throw AssertionError("timed out after ${timeoutMillis}ms waiting for $what", e)
    }
    return value ?: error("waited for $what and got nothing")
}
