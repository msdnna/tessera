package website.msdnna.tessera.e2e

import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithTag
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextInput
import androidx.compose.ui.test.performTextReplacement
import com.google.common.truth.Truth.assertThat
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import org.junit.Rule
import org.junit.Test
import org.junit.rules.RuleChain
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.ui.AppRoot
import website.msdnna.tessera.ui.TestTags

/**
 * The login flow driven through the real form against the real backend — the one
 * spec that must not shortcut the session, since every other spec starts from an
 * already-seeded one and would never notice a broken sign-in.
 *
 * The account is seeded over the API (`login = false`), then this test types its
 * credentials into [AppRoot]'s auth screen and asserts the session gate flips.
 */
@RunWith(RobolectricTestRunner::class)
class AuthE2eTest {
    private val e2e = E2eRule(login = false)
    private val compose = createComposeRule()

    /** Order matters: the backend seed and the DataStore wipe have to happen
     *  before the activity composes, or [AppRoot] boots off the previous test's
     *  prefs and lands on the board instead of the auth screen. */
    @get:Rule
    val rules: RuleChain = RuleChain.outerRule(e2e).around(compose)

    @Test
    fun `signing in through the form opens the app`() {
        compose.setContent { AppRoot() }
        compose.awaitTag(TestTags.AUTH_SUBMIT)

        compose.onNodeWithTag(TestTags.AUTH_EMAIL).performTextInput(e2e.account.email)
        compose.onNodeWithTag(TestTags.AUTH_PASSWORD).performTextInput(E2eBackend.PASSWORD)
        compose.onNodeWithTag(TestTags.AUTH_SUBMIT).performClick()

        compose.awaitTag(TestTags.MAIN_SHELL)
        // The gate is driven by the persisted token, so assert it actually landed
        // rather than trusting the screen swap alone.
        val token = runBlocking { AppContainer.prefs.authToken.first() }
        assertThat(token).isNotEmpty()
    }

    @Test
    fun `a wrong password reports an error and keeps us on the form`() {
        compose.setContent { AppRoot() }
        compose.awaitTag(TestTags.AUTH_SUBMIT)

        compose.onNodeWithTag(TestTags.AUTH_EMAIL).performTextInput(e2e.account.email)
        compose.onNodeWithTag(TestTags.AUTH_PASSWORD).performTextInput("definitely-not-the-password")
        compose.onNodeWithTag(TestTags.AUTH_SUBMIT).performClick()

        compose.awaitTag(TestTags.AUTH_ERROR)
        // Asserting the gate did not open, rather than that some particular field
        // is still on screen: the error text reflows the form, and a spec that
        // pinned a widget's position would fail on layout instead of on auth.
        compose.onAllNodesWithTag(TestTags.MAIN_SHELL).assertCountEquals(0)
        val token = runBlocking { AppContainer.prefs.authToken.first() }
        assertThat(token).isEmpty()
    }

    @Test
    fun `registering a new account from the form opens the app`() {
        compose.setContent { AppRoot() }
        compose.awaitTag(TestTags.AUTH_TOGGLE_MODE)

        compose.onNodeWithTag(TestTags.AUTH_TOGGLE_MODE).performClick()
        // The name field exists only in register mode — its presence is the proof
        // the toggle switched the form, not just the button label.
        compose.awaitTag(TestTags.AUTH_NAME)

        compose.onNodeWithTag(TestTags.AUTH_EMAIL).performTextInput(freshEmail())
        compose.onNodeWithTag(TestTags.AUTH_NAME).performTextInput("Form signup")
        compose.onNodeWithTag(TestTags.AUTH_PASSWORD).performTextInput(E2eBackend.PASSWORD)
        compose.onNodeWithTag(TestTags.AUTH_SUBMIT).performClick()

        compose.awaitTag(TestTags.MAIN_SHELL)
    }

    @Test
    fun `the server address typed into the popover is what the app talks to`() {
        compose.setContent { AppRoot() }
        compose.awaitTag(TestTags.AUTH_SERVER_TOGGLE)

        compose.onNodeWithTag(TestTags.AUTH_SERVER_TOGGLE).performClick()
        compose.awaitTag(TestTags.AUTH_SERVER_FIELD)
        // A URL with nothing behind it: the login must fail against *this* address,
        // which is only possible if the field really rebound the client.
        compose.onNodeWithTag(TestTags.AUTH_SERVER_FIELD)
            .performTextReplacement("http://localhost:$DEAD_PORT")
        compose.waitUntil(TIMEOUT_MS) {
            runBlocking { AppContainer.prefs.serverUrl.first() } == "http://localhost:$DEAD_PORT"
        }

        compose.onNodeWithTag(TestTags.AUTH_SERVER_TOGGLE).performClick()
        compose.onNodeWithTag(TestTags.AUTH_EMAIL).performTextInput(e2e.account.email)
        compose.onNodeWithTag(TestTags.AUTH_PASSWORD).performTextInput(E2eBackend.PASSWORD)
        compose.onNodeWithTag(TestTags.AUTH_SUBMIT).performClick()

        // Same credentials that succeed in the first test — so an error here can
        // only come from the address change.
        compose.awaitTag(TestTags.AUTH_ERROR)
    }

    private fun freshEmail(): String = "e2e+form-${System.nanoTime()}@test.local"

    private companion object {
        /** Nothing listens here; used to prove the server field is honoured. */
        const val DEAD_PORT = 8099
        const val TIMEOUT_MS = 20_000L
    }
}
