package website.msdnna.tessera.e2e

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.test.junit4.createComposeRule
import kotlinx.coroutines.delay
import org.junit.Assert.assertThrows
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import website.msdnna.tessera.ui.TestTags

/**
 * Self-test for [awaitShell] against a gate that fails on purpose.
 *
 * The helper exists for a flake that shows up roughly once in twenty runs of the
 * tier, which is no way to tell a fix from luck: the previous budget (one retry)
 * looked right by the same reasoning and still let a red run through, because the
 * gate can lose *twice* in a row (#2908). So the failure is modelled here instead
 * of waited for — a fake gate whose number of losses is an argument.
 *
 * The fake mirrors the real gate's shape rather than its code: a retry re-runs the
 * boot effect, and the error screen (with its button) stays up until that effect
 * gets its turn — which is the window that makes a naive retry loop burn its whole
 * budget on a single failure.
 *
 * Needs no backend: the gate is fake all the way down.
 */
@RunWith(RobolectricTestRunner::class)
class AwaitShellTest {
    @get:Rule
    val compose = createComposeRule()

    /** One attempt at the fake gate, so the whole test is over in seconds. */
    private val attemptMs = 3_000L

    @Test
    fun `the shell is reached when the gate loses twice in a row`() {
        compose.setContent { FakeGate(losses = 2) }

        compose.awaitShell(attemptMillis = attemptMs)

        compose.awaitTag(TestTags.MAIN_SHELL, attemptMs)
    }

    /**
     * The negative control, and the reason the budget changed: this is exactly the
     * old helper (`retries = 1`), and two losses defeat it.
     */
    @Test
    fun `a single retry is not enough for two losses`() {
        compose.setContent { FakeGate(losses = 2) }

        assertThrows(AssertionError::class.java) {
            compose.awaitShell(attempts = 1, attemptMillis = attemptMs)
        }
    }

    /**
     * A gate that never comes up must still fail — the retry loop is there to
     * outlast a stalled connect, not to paper over a shell that will never render.
     */
    @Test
    fun `a gate that always loses still fails`() {
        compose.setContent { FakeGate(losses = Int.MAX_VALUE) }

        assertThrows(AssertionError::class.java) {
            compose.awaitShell(attempts = 2, attemptMillis = attemptMs)
        }
    }

    /**
     * Stands in for `AppRoot`'s launch gate: [losses] bootstraps end on the retry
     * screen, the next one reaches the shell. Every bootstrap costs [bootMs], and
     * the retry screen survives the tap until the new bootstrap starts — both are
     * what the helper has to cope with on the real gate.
     */
    @Composable
    private fun FakeGate(losses: Int, bootMs: Long = 150) {
        var nonce by remember { mutableIntStateOf(0) }
        var state by remember { mutableStateOf(Gate.LOADING) }

        LaunchedEffect(nonce) {
            state = Gate.LOADING
            delay(bootMs)
            state = if (nonce >= losses) Gate.SHELL else Gate.ERROR
        }

        when (state) {
            Gate.LOADING -> Text("boot")

            Gate.SHELL -> Box(Modifier.testTag(TestTags.MAIN_SHELL)) { Text("shell") }

            Gate.ERROR -> Text(
                text = "retry",
                modifier = Modifier.testTag(TestTags.BOOT_RETRY).clickable { nonce++ },
            )
        }
    }

    private enum class Gate { LOADING, ERROR, SHELL }
}
