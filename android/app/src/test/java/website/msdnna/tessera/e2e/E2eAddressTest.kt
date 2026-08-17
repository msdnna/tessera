package website.msdnna.tessera.e2e

import com.google.common.truth.Truth.assertThat
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner

/**
 * Guards a failure mode the rest of the suite cannot see.
 *
 * [E2eBackend] picks its address from the tier it detects (host JVM → `localhost`,
 * device → the emulator's `10.0.2.2` alias). If that detection ever stopped
 * recognising Robolectric, this tier would aim at an address nothing answers on,
 * [E2eBackend.requireBackend] would *skip* every spec, and the run would report
 * green while testing nothing.
 *
 * Deliberately carries no [E2eRule]: the rule skips on an unreachable backend,
 * which is exactly the state being guarded against — the check has to survive it.
 * It touches no network, so it is the one spec here that always executes.
 */
@RunWith(RobolectricTestRunner::class)
class E2eAddressTest {
    @Test
    fun `the jvm tier resolves the backend on the host, not on a device`() {
        assertThat(E2eBackend.serverUrl).contains("localhost")
    }
}
