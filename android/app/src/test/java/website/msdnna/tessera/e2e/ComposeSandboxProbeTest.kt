package website.msdnna.tessera.e2e

import androidx.compose.material3.Text
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner

/**
 * Smoke probe for the e2e tier's foundation: a real Compose tree composed and
 * asserted on the JVM under Robolectric. If this fails, no e2e spec can pass —
 * so it is deliberately the cheapest possible test.
 */
@RunWith(RobolectricTestRunner::class)
class ComposeSandboxProbeTest {
    @get:Rule
    val compose = createComposeRule()

    @Test
    fun `compose tree renders under robolectric`() {
        compose.setContent { Text("tessera-e2e-probe") }
        compose.onNodeWithText("tessera-e2e-probe").assertIsDisplayed()
    }
}
