package website.msdnna.tessera.e2e

import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import com.google.common.truth.Truth.assertThat
import org.junit.Rule
import org.junit.Test
import org.junit.rules.RuleChain
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import website.msdnna.tessera.ui.TestTags

/**
 * The Get Started guide as a user meets it (#2860): started from the sidebar
 * footer, drawn over the shell, and ended in a way the server can see.
 *
 * The engine's own rules are covered by `TourTest` against the pure
 * [website.msdnna.tessera.util.TourEngine], and the anchor registry by
 * `TourAnchorsTest` — neither needs a screen. What only a live run can prove is
 * the wiring: that the footer button reaches the ViewModel, that the overlay is
 * composed above the drawer it points into, and that the outcome is written to
 * the shared `getstarted:*` acknowledgement rather than kept in memory.
 *
 * The qualifiers are not decoration. Robolectric's default screen is 320×470,
 * and the footer sits at the bottom of a full-height drawer: a tap below the
 * bottom edge is dropped in silence, so the spec would fail as though the button
 * did nothing.
 */
@RunWith(RobolectricTestRunner::class)
@Config(qualifiers = "w411dp-h891dp-xhdpi")
class TourE2eTest {
    private val e2e = E2eRule()
    private val compose = createComposeRule()

    @get:Rule
    val rules: RuleChain = RuleChain.outerRule(e2e).around(compose)

    @Test
    fun `the guide starts from the sidebar footer and its first step is drawn over the drawer`() {
        compose.setShellContent(e2e.fixture)

        compose.onNodeWithTag(TestTags.TOPBAR_MENU).performClick()
        compose.awaitTag(TestTags.TOUR_START)
        compose.onNodeWithTag(TestTags.TOUR_START).performClick()

        // The card only appears once the step's anchor has reported a rect — i.e.
        // this also proves the first step found the workspace switcher inside the
        // drawer, which is the whole point of mounting the shell.
        compose.awaitTag(TestTags.TOUR_CARD)
        compose.awaitTextIn(TestTags.TOUR_CARD, "1/28")
    }

    @Test
    fun `walking a step forward moves the guide on`() {
        compose.setShellContent(e2e.fixture)

        compose.onNodeWithTag(TestTags.TOPBAR_MENU).performClick()
        compose.awaitTag(TestTags.TOUR_START)
        compose.onNodeWithTag(TestTags.TOUR_START).performClick()
        compose.awaitTag(TestTags.TOUR_CARD)

        // «Понятно» on the opening info step. The one after it is an action step
        // («нажмите +»), which offers no «Понятно» at all — so the button going
        // away is itself the proof the guide advanced, and the counter says where to.
        compose.onNodeWithTag(TestTags.TOUR_NEXT).performClick()
        compose.awaitTextIn(TestTags.TOUR_CARD, "2/28")
        compose.awaitNoTag(TestTags.TOUR_NEXT)
    }

    @Test
    fun `skipping ends the guide and records it on the server`() {
        compose.setShellContent(e2e.fixture)
        assertThat(E2eBackend.acknowledgements(e2e.fixture)).doesNotContain("getstarted:skipped")

        compose.onNodeWithTag(TestTags.TOPBAR_MENU).performClick()
        compose.awaitTag(TestTags.TOUR_START)
        compose.onNodeWithTag(TestTags.TOUR_START).performClick()
        compose.awaitTag(TestTags.TOUR_CARD)

        compose.onNodeWithTag(TestTags.TOUR_SKIP).performClick()
        compose.awaitNoTag(TestTags.TOUR_CARD)

        // Asserted server-side, not from preferences: the key space is shared with
        // the web, and this is what stops the guide greeting the same person twice.
        compose.awaitServer("the skip to reach the acknowledgements") {
            E2eBackend.acknowledgements(e2e.fixture).firstOrNull { it == "getstarted:skipped" }
        }
    }
}
