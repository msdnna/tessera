package website.msdnna.tessera.smoke

import android.Manifest
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextInput
import androidx.test.rule.GrantPermissionRule
import com.google.common.truth.Truth.assertThat
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import org.junit.Rule
import org.junit.Test
import org.junit.rules.RuleChain
import website.msdnna.tessera.MainActivity
import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.e2e.E2eBackend
import website.msdnna.tessera.e2e.E2eRule
import website.msdnna.tessera.e2e.awaitTag
import website.msdnna.tessera.ui.TestTags

/**
 * Signing in on a real device, through the real [MainActivity].
 *
 * This is the one thing the JVM tier structurally cannot claim: there the
 * composition is hosted by a stand-in activity and taps are injected into a
 * simulated tree. Here the app is launched the way the launcher launches it —
 * `enableEdgeToEdge`, the runtime permission request, the real IME, the real
 * render pipeline — and the text is typed into a real input connection.
 *
 * It is deliberately thin. The instrumented tier is a smoke check that the app
 * works as an app; behaviour coverage (wrong password, register, server address)
 * belongs to the JVM tier, which runs on every PR in seconds rather than minutes.
 */
class LoginSmokeTest {
    private val e2e = E2eRule(login = false)
    private val compose = createAndroidComposeRule<MainActivity>()

    /**
     * [MainActivity] asks for POST_NOTIFICATIONS in `onCreate`, and on API 33+ that
     * puts a system dialog over the app. It is not part of what this spec tests,
     * and left alone it would swallow the first taps — so the permission is granted
     * up front and the dialog never appears.
     */
    private val permissions: GrantPermissionRule =
        GrantPermissionRule.grant(Manifest.permission.POST_NOTIFICATIONS)

    /** Order matters: the seed and the DataStore wipe have to land before the
     *  activity composes, or the app boots off the previous test's session and
     *  never shows the auth screen. */
    @get:Rule
    val rules: RuleChain = RuleChain.outerRule(permissions).around(e2e).around(compose)

    @Test
    fun signingInThroughTheFormOpensTheApp() {
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
}
