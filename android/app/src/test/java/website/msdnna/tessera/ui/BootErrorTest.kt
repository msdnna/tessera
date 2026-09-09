package website.msdnna.tessera.ui

import android.content.Context
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.test.core.app.ApplicationProvider
import com.google.common.truth.Truth.assertThat
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import website.msdnna.tessera.R
import website.msdnna.tessera.ui.theme.TesseraTheme

/**
 * The startup error gate (#2920).
 *
 * The bug was that the offline gate's only non-retry action closed the app, so
 * a wrong server address stranded the user — the way out is a second action the
 * gate supplies, not a hard-wired «Выход». The test is about that second action
 * being whatever the caller passed and firing its own callback: a build that
 * re-hardcoded the exit link would render identically and look just as working.
 *
 * Pinned to ru at a real phone size — Robolectric's default en locale would miss
 * the Russian labels the strings are looked up in, and its default 320×470px
 * silently drops a tap below the fold.
 */
@RunWith(RobolectricTestRunner::class)
@Config(qualifiers = "ru-w411dp-h891dp-xhdpi")
class BootErrorTest {
    @get:Rule
    val compose = createComposeRule()

    private val res = ApplicationProvider.getApplicationContext<Context>().resources

    private fun mount(
        secondaryLabel: String,
        onPrimary: () -> Unit = {},
        onSecondary: () -> Unit = {},
    ) {
        compose.setContent {
            TesseraTheme {
                BootError(
                    title = res.getString(R.string.gate_offline_title),
                    message = res.getString(R.string.gate_offline_message),
                    primaryLabel = res.getString(R.string.gate_offline_retry),
                    onPrimary = onPrimary,
                    secondaryLabel = secondaryLabel,
                    onSecondary = onSecondary,
                )
            }
        }
    }

    @Test
    fun `the offline gate offers changing the server, not closing the app`() {
        var changed = 0
        mount(res.getString(R.string.gate_change_server), onSecondary = { changed++ })

        compose.onNodeWithText(res.getString(R.string.gate_change_server)).assertIsDisplayed()
        // The old wording is what «closes the app» read as — it must be gone.
        compose.onNodeWithText(res.getString(R.string.gate_exit)).assertDoesNotExist()

        compose.onNodeWithText(res.getString(R.string.gate_change_server)).performClick()
        assertThat(changed).isEqualTo(1)
    }

    @Test
    fun `the retry button reports the primary action`() {
        var retried = 0
        mount(res.getString(R.string.gate_change_server), onPrimary = { retried++ })

        compose.onNodeWithText(res.getString(R.string.gate_offline_retry)).performClick()
        assertThat(retried).isEqualTo(1)
    }

    /** The session gate still keeps its own exit — the fix is scoped to offline. */
    @Test
    fun `the secondary label is whatever the caller passes`() {
        var exited = 0
        mount(res.getString(R.string.gate_exit), onSecondary = { exited++ })

        compose.onNodeWithText(res.getString(R.string.gate_exit)).performClick()
        assertThat(exited).isEqualTo(1)
    }
}
