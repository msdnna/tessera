package website.msdnna.tessera.e2e

import android.content.Context
import androidx.datastore.preferences.core.edit
import androidx.test.core.app.ApplicationProvider
import kotlinx.coroutines.runBlocking
import org.junit.rules.TestRule
import org.junit.runner.Description
import org.junit.runners.model.Statement
import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.api.RetrofitClient
import website.msdnna.tessera.data.preferences.dataStore

/**
 * Puts the app into a known state in front of a live e2e backend: server URL
 * pointed at :8092, DataStore wiped, and (by default) a freshly seeded
 * workspace with the account already logged in.
 *
 * The reset is not optional bookkeeping. [AppContainer], [RetrofitClient] and
 * the `preferencesDataStore` delegate are all singletons living in Robolectric's
 * sandbox classloader, which is shared across test classes — without an explicit
 * wipe the second test inherits the first one's session and "logged in" would
 * pass in a spec that never logged in. Hence the reset runs both before and
 * after each test.
 */
class E2eRule(
    private val seed: Boolean = true,
    private val login: Boolean = true,
) : TestRule {

    /** The seeded workspace/board. Only valid when the rule was built with `seed = true`. */
    lateinit var fixture: E2eBackend.Fixture
        private set

    val account: E2eBackend.Account get() = fixture.account

    override fun apply(base: Statement, description: Description): Statement =
        object : Statement() {
            override fun evaluate() {
                E2eBackend.requireBackend()
                val context = ApplicationProvider.getApplicationContext<Context>()
                AppContainer.init(context)
                reset(context)
                AppContainer.serverUrl = E2eBackend.serverUrl
                runBlocking { AppContainer.prefs.setServerUrl(E2eBackend.serverUrl) }

                if (seed) {
                    fixture = E2eBackend.seedBoard(E2eBackend.registerAccount())
                    if (login) authenticate()
                }

                try {
                    base.evaluate()
                } finally {
                    reset(context)
                }
            }
        }

    /** Hands the seeded token pair to both the network client and DataStore —
     *  the same pair of places a real login writes to, so the app boots straight
     *  past the auth gate. */
    private fun authenticate() {
        RetrofitClient.authToken = account.accessToken
        RetrofitClient.refreshToken = account.refreshToken
        runBlocking {
            AppContainer.prefs.setSession(account.accessToken to account.refreshToken, account.user)
        }
    }

    private fun reset(context: Context) {
        RetrofitClient.authToken = ""
        RetrofitClient.refreshToken = ""
        // Callbacks capture the previous test's composition scope; leaving them
        // wired would let a dead scope handle this test's 401s.
        RetrofitClient.onTokensRefreshed = null
        RetrofitClient.onUnauthorized = null
        RetrofitClient.reset()
        runBlocking { context.dataStore.edit { it.clear() } }
    }
}
