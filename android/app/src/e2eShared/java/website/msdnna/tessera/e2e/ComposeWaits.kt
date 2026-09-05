package website.msdnna.tessera.e2e

import androidx.compose.ui.test.ComposeTimeoutException
import androidx.compose.ui.test.ExperimentalTestApi
import androidx.compose.ui.test.hasAnyAncestor
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.isEnabled
import androidx.compose.ui.test.isRoot
import androidx.compose.ui.test.junit4.ComposeContentTestRule
import androidx.compose.ui.test.onAllNodesWithTag
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.printToString
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.ui.TestTags

/** How long a spec waits on a screen that is round-tripping to the backend. */
private const val AWAIT_TIMEOUT_MS = 20_000L

/**
 * How long *one* pass at the launch gate may take before it counts as lost.
 *
 * Sized off the client's connect timeout, not off [AWAIT_TIMEOUT_MS]: on this host
 * a connect to loopback can stall for the full 15s instead of being refused, so a
 * shorter window would give up on an attempt that was still going to arrive.
 */
private const val BOOT_ATTEMPT_MS = 20_000L

/**
 * How many passes at the launch gate a spec is willing to sit through.
 *
 * Three, because two were demonstrably survivable: the gate lost twice back to back
 * in one run of the tier (#2908). This is not a number to raise on a hunch — every
 * attempt is another [BOOT_ATTEMPT_MS] of wall clock on the failure path, and if it
 * ever needs a fourth, the stall is the bug, not the budget.
 */
private const val BOOT_ATTEMPTS = 3

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
    withRootsOnTimeout("tag «$tag»", timeoutMillis) { waitUntilExactlyOneExists(hasTestTag(tag), timeoutMillis) }
}

/**
 * Runs [wait] and, if it times out, re-throws with every semantics root dumped
 * into the message.
 *
 * A timed-out `awaitTag` says only «condition never became true», which reads the
 * same whether the node is absent, present twice, or present in a window this
 * assertion cannot see. That last case is not hypothetical here: a `Popup` is a
 * window of its own, so a picker that opened fine still leaves the main root
 * looking untouched. Dumping *all* roots — [useUnmergedTree] so a merged parent
 * cannot hide the tagged child — is what separates «the tap was swallowed» from
 * «the popup is up and the matcher is wrong», which are one-line fixes in
 * opposite files.
 */
private fun ComposeContentTestRule.withRootsOnTimeout(what: String, timeoutMillis: Long, wait: () -> Unit) {
    try {
        wait()
    } catch (e: ComposeTimeoutException) {
        val roots = runCatching {
            onAllNodes(isRoot(), useUnmergedTree = true).printToString(maxDepth = Int.MAX_VALUE)
        }.getOrElse { "<the tree could not be read: $it>" }
        throw AssertionError(
            "timed out after ${timeoutMillis}ms waiting for $what; ${address()}; roots:\n$roots",
            e,
        )
    }
}

/**
 * The address the app is *actually* pointed at, next to the one it was set up with.
 *
 * A timeout that leaves «нет связи с сервером» on screen has two very different
 * causes — the stand went away, or the app is dialling somewhere else — and the
 * tree alone cannot tell them apart. It is not a hypothetical mismatch: one spec
 * parks a deliberately dead port in DataStore to prove the server field is
 * honoured, and every later spec shares that DataStore singleton. Printing both
 * values turns «unreachable» into either «unreachable at the right address» or a
 * named leak.
 */
private fun address(): String = runCatching {
    "server: client=${AppContainer.serverUrl}, prefs=${runBlocking { AppContainer.prefs.serverUrl.first() }}, " +
        "expected=${E2eBackend.serverUrl}"
}.getOrElse { "server: <could not be read: $it>" }

/**
 * Waits for exactly one node carrying [tag] to exist **and be enabled**.
 *
 * The distinction is not pedantic: a disabled `Modifier.clickable` keeps its
 * `OnClick` semantics and only adds `Disabled`, so [awaitTag] is already
 * satisfied by a control that will drop the next tap on the floor. Compose does
 * not reject that tap — it silently does nothing — and the spec goes on to fail
 * twenty seconds later on whatever the click was meant to produce, pointing at
 * the wrong screen entirely. Screens here load in two waves (the modal renders
 * off the task, then fills in board columns, comments, attachments), so any
 * control gated on the second wave has a real window of being present but dead.
 * Waiting on `isEnabled()` fails inside that window instead, and names the
 * control that never came alive.
 */
@OptIn(ExperimentalTestApi::class)
fun ComposeContentTestRule.awaitEnabled(tag: String, timeoutMillis: Long = AWAIT_TIMEOUT_MS) {
    withRootsOnTimeout("tag «$tag» to become enabled", timeoutMillis) {
        waitUntilExactlyOneExists(hasTestTag(tag) and isEnabled(), timeoutMillis)
    }
}

/**
 * Waits for the app shell, tapping «Попробовать ещё раз» if the launch gate lands
 * on its connect-error screen first.
 *
 * Only a spec that boots [website.msdnna.tessera.ui.AppRoot] with a *live* session
 * needs this: that is the one path where the first frame is gated on a network call
 * (`authRepo.verify()`), so a stalled TCP connect turns into a screen that never
 * becomes the shell. And the connect really does stall on this host — a connect to
 * a closed loopback port hangs for the full timeout instead of being refused, which
 * is why the spec that parks a dead port costs a flat 15.9s. When the same stall
 * lands on the *live* port, `verify()` dies on the client's 15s connect timeout,
 * `Boot.ConnectError` paints, and a plain [awaitTag] then burns its remaining 5s
 * against a screen that will never change on its own: the gate is terminal until
 * someone taps retry (#2908).
 *
 * Retrying is what a user does, and it re-runs the same bootstrap, so nothing about
 * the spec's subject is weakened — a shell that only appears because the session is
 * valid still only appears because the session is valid. What it drops is a failure
 * mode that says «the shell never rendered» about a host-level network stall.
 *
 * The budget is counted per *attempt*, not once for the whole helper. A single
 * retry was not enough: the gate can lose twice in a row, and it did — the fast
 * refusal that opened the error screen was followed by a genuine stall, so the one
 * retry was spent before the expensive failure even started (#2908). One attempt
 * costs up to [attemptMillis] because that is what a stalled connect costs, so the
 * number of attempts has to be multiplied by that, not squeezed inside it.
 *
 * [attempts] and [attemptMillis] are open only so the harness can test itself
 * (`AwaitShellTest`) — with a real gate on the other end, both defaults apply.
 */
@OptIn(ExperimentalTestApi::class)
fun ComposeContentTestRule.awaitShell(attempts: Int = BOOT_ATTEMPTS, attemptMillis: Long = BOOT_ATTEMPT_MS) {
    for (attempt in 1..attempts) {
        // Neither tag showing up is not this helper's failure to report: fall
        // through to `awaitTag` below, whose dump names the roots and the address.
        runCatching {
            waitUntilAtLeastOneExists(
                hasTestTag(TestTags.MAIN_SHELL) or hasTestTag(TestTags.BOOT_RETRY),
                attemptMillis,
            )
        }
        // No retry button means the gate is not on its error screen — either the
        // shell is up, or nothing rendered at all, and `awaitTag` tells them apart.
        if (onAllNodesWithTag(TestTags.BOOT_RETRY).fetchSemanticsNodes().isEmpty()) break
        onNodeWithTag(TestTags.BOOT_RETRY).performClick()
        // Wait for the tap to land before looping. The retry re-enters the gate's
        // loading state, but not within the same frame — and until it does, the
        // error screen still holds the retry button. Without this the next
        // iteration would match that stale button instantly and tap it again, so
        // the whole budget would be spent in milliseconds against one failure.
        runCatching { waitUntilDoesNotExist(hasTestTag(TestTags.BOOT_RETRY), attemptMillis) }
    }
    awaitTag(TestTags.MAIN_SHELL, attemptMillis)
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
 * Waits for the node [tag] to itself read [text] (substring match).
 *
 * The counterpart of [awaitTextIn] for a node whose text is its *own*, merged
 * semantics rather than a descendant's — a counter, a badge, a chip label. The
 * match is on a substring because such a label usually wraps the interesting part
 * in localised chrome («показано: 2 (4)»), and a spec should assert the numbers,
 * not the wording around them.
 */
@OptIn(ExperimentalTestApi::class)
fun ComposeContentTestRule.awaitTextOn(tag: String, text: String, timeoutMillis: Long = AWAIT_TIMEOUT_MS) {
    waitUntilExactlyOneExists(hasTestTag(tag) and hasText(text, substring = true), timeoutMillis)
}

/** Waits for [tag] to leave the tree — the counterpart of [awaitTag] for a node
 *  whose disappearance is the observable effect (an overlay, a loader). */
@OptIn(ExperimentalTestApi::class)
fun ComposeContentTestRule.awaitNoTag(tag: String, timeoutMillis: Long = AWAIT_TIMEOUT_MS) {
    waitUntilDoesNotExist(hasTestTag(tag), timeoutMillis)
}

/**
 * Waits for the card [taskId] to be rendered *inside* the lane [laneId].
 *
 * Grouping specs live or die on this distinction: a plain `awaitTag(taskCard(id))`
 * is satisfied by the card sitting in any lane at all, so it would pass on a board
 * where the grouping switched on but dropped every card into the catch-all.
 */
@OptIn(ExperimentalTestApi::class)
fun ComposeContentTestRule.awaitCardInLane(laneId: String, taskId: String, timeoutMillis: Long = AWAIT_TIMEOUT_MS) {
    waitUntilExactlyOneExists(
        hasAnyAncestor(hasTestTag(TestTags.boardColumn(laneId))) and hasTestTag(TestTags.taskCard(taskId)),
        timeoutMillis,
    )
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
