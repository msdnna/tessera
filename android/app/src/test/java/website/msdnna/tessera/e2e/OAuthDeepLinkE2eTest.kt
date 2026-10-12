package website.msdnna.tessera.e2e

import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithTag
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
import website.msdnna.tessera.ui.OAuthResult
import website.msdnna.tessera.ui.TestTags
import website.msdnna.tessera.util.newOAuthNonce

/**
 * The last hop of the GitLab login is a plain OS-wide URL — `tessera://oauth/callback#…`
 * — and any page open in the phone's browser can navigate to it. So the tokens riding in
 * that fragment prove nothing on their own: without a value only this app knows, an
 * unsolicited deep link signs the user into whatever account the attacker minted, over a
 * live session and without a single tap (session fixation, #2932; the desktop client got
 * its nonce then, Android shipped without one).
 *
 * Both specs drive [AppRoot] with a *real* session — registered against the live e2e
 * backend, exactly what an attacker would hand over — and differ only in whether a login
 * was pending. That difference is the whole fix.
 */
@RunWith(RobolectricTestRunner::class)
class OAuthDeepLinkE2eTest {
    /** `login = false`: the app must start at the auth gate, or "did it let us in?"
     *  has no meaning. The seeded account plays the attacker's — its tokens are valid. */
    private val e2e = E2eRule(login = false)
    private val compose = createComposeRule()

    @get:Rule
    val rules: RuleChain = RuleChain.outerRule(e2e).around(compose)

    @Test
    fun `an unsolicited deep link does not sign us in`() {
        // Nothing pending: nobody pressed "Continue with GitLab". This is the link a web
        // page fires on its own, carrying tokens the attacker minted for their account.
        val handoff = OAuthResult.Success(e2e.account.accessToken, e2e.account.refreshToken, state = "")

        compose.setContent { AppRoot(oauthResult = handoff) }
        compose.awaitTag(TestTags.AUTH_ERROR)

        compose.onAllNodesWithTag(TestTags.MAIN_SHELL).assertCountEquals(0)
        // The gate is driven by the persisted token, so assert the session never landed
        // rather than trusting the screen alone.
        val token = runBlocking { AppContainer.prefs.authToken.first() }
        assertThat(token).isEmpty()
    }

    @Test
    fun `the handoff of a login we started signs us in`() {
        // The positive control, and the reason the guard can't just be "refuse everything":
        // a nonce minted before the browser opened is waiting on disk, and the backend
        // echoes it back in the fragment.
        val nonce = newOAuthNonce()
        runBlocking { AppContainer.prefs.setOAuthNonce(nonce) }
        val handoff = OAuthResult.Success(e2e.account.accessToken, e2e.account.refreshToken, state = nonce)

        compose.setContent { AppRoot(oauthResult = handoff) }
        compose.awaitTag(TestTags.MAIN_SHELL)

        val token = runBlocking { AppContainer.prefs.authToken.first() }
        assertThat(token).isNotEmpty()
        // Burned on read: replaying the same deep link later finds nothing pending.
        val pending = runBlocking { AppContainer.prefs.consumeOAuthNonce() }
        assertThat(pending).isEmpty()
    }
}
