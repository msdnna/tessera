package website.msdnna.tessera.ui.screens

import android.content.Context
import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.test.core.app.ApplicationProvider
import com.google.common.truth.Truth.assertThat
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import website.msdnna.tessera.R
import website.msdnna.tessera.ui.TestTags
import website.msdnna.tessera.ui.theme.TesseraTheme
import website.msdnna.tessera.util.ConfMiniLine

/**
 * The minimised call bar (#2896 §9).
 *
 * The surface that exists precisely when nobody is watching the call, which is
 * why every one of its states is worth an assertion: a bar that says «идёт
 * звонок» through a dropped connection, or one whose hang-up reaches nothing,
 * looks exactly like a bar that works.
 *
 * Screen size pinned as in the other conference specs — Robolectric's default
 * 320×470px drops a tap below the edge without saying so.
 */
@RunWith(RobolectricTestRunner::class)
@Config(qualifiers = "ru-w411dp-h891dp-xhdpi")
class ConferenceMiniBarTest {
    @get:Rule
    val compose = createComposeRule()

    private val res = ApplicationProvider.getApplicationContext<Context>().resources

    private fun mount(
        line: ConfMiniLine = ConfMiniLine.LIVE,
        title: String = "Планёрка",
        others: Int = 0,
        micOn: Boolean = true,
        micEnabled: Boolean = true,
        onReturn: () -> Unit = {},
        onToggleMic: () -> Unit = {},
        onHangup: () -> Unit = {},
    ) {
        compose.setContent {
            TesseraTheme {
                ConferenceMiniBar(
                    line = line,
                    title = title,
                    others = others,
                    micOn = micOn,
                    micEnabled = micEnabled,
                    onReturn = onReturn,
                    onToggleMic = onToggleMic,
                    onHangup = onHangup,
                )
            }
        }
    }

    private fun nodesWith(text: String) =
        compose.onAllNodesWithText(text, substring = true).fetchSemanticsNodes()

    /**
     * Anchors inside the bar are read off the unmerged tree.
     *
     * The whole bar is one tap target, and `clickable` merges its descendants'
     * semantics into itself — so the line and the counter are present but carry
     * no bounds of their own in the merged tree, and an assertion against them
     * fails with «is not displayed» on a bar that is right there on screen.
     */
    private fun tag(tag: String) = compose.onNodeWithTag(tag, useUnmergedTree = true)

    @Test
    fun `the bar names the meeting it belongs to`() {
        mount(title = "Планёрка")

        compose.onNodeWithTag(TestTags.CONFERENCE_MINI).assertIsDisplayed()
        // The name is the whole point: a bar saying only «идёт звонок» is no help
        // to somebody who has two meetings in the day and left one running.
        assertThat(nodesWith("Планёрка")).isNotEmpty()
    }

    @Test
    fun `a call with no name still says something`() {
        mount(title = "")

        assertThat(nodesWith(res.getString(R.string.conf_untitled))).isNotEmpty()
    }

    @Test
    fun `a live call and a lost one do not read the same`() {
        mount(line = ConfMiniLine.LIVE)
        tag(TestTags.CONFERENCE_MINI_LINE).assertExists()
        assertThat(nodesWith(res.getString(R.string.conf_mini_live))).isNotEmpty()
        assertThat(nodesWith(res.getString(R.string.conf_mini_lost))).isEmpty()
    }

    @Test
    fun `a dropped connection says so on the bar`() {
        mount(line = ConfMiniLine.LOST)

        assertThat(nodesWith(res.getString(R.string.conf_mini_lost))).isNotEmpty()
        assertThat(nodesWith(res.getString(R.string.conf_mini_live))).isEmpty()
    }

    @Test
    fun `reconnecting is its own line, not the live one`() {
        mount(line = ConfMiniLine.RECONNECTING)

        assertThat(nodesWith(res.getString(R.string.conf_mini_reconnecting))).isNotEmpty()
        assertThat(nodesWith(res.getString(R.string.conf_mini_live))).isEmpty()
    }

    @Test
    fun `the count appears only when somebody else is there`() {
        mount(others = 2)
        tag(TestTags.CONFERENCE_MINI_COUNT).assertExists()
        assertThat(nodesWith("2")).isNotEmpty()
    }

    @Test
    fun `alone in the call there is no counter`() {
        // A «0» beside a people icon reads as a counter that broke, not as
        // «nobody has joined yet».
        mount(others = 0)

        tag(TestTags.CONFERENCE_MINI_COUNT).assertDoesNotExist()
    }

    @Test
    fun `the microphone is one tap away`() {
        var toggled = 0
        mount(onToggleMic = { toggled++ })

        compose.onNodeWithTag(TestTags.CONFERENCE_MINI_MIC).performClick()

        assertThat(toggled).isEqualTo(1)
    }

    @Test
    fun `a host's mute makes the bar's microphone unpressable`() {
        // Not hidden — disabled, exactly as the room's toolbar does it: the
        // server would refuse the publish anyway, and a button that silently
        // does nothing reads as a broken microphone rather than a host's call.
        var toggled = 0
        mount(micEnabled = false, onToggleMic = { toggled++ })

        compose.onNodeWithTag(TestTags.CONFERENCE_MINI_MIC).performClick()

        assertThat(toggled).isEqualTo(0)
    }

    @Test
    fun `hanging up from the bar is possible without the room`() {
        // The only way out of a call whose lobby is nowhere on screen. Without
        // it a meeting left running can be ended only by finding it again.
        var hung = 0
        mount(onHangup = { hung++ })

        compose.onNodeWithTag(TestTags.CONFERENCE_MINI_HANGUP).performClick()

        assertThat(hung).isEqualTo(1)
    }

    @Test
    fun `tapping the bar goes back to the call`() {
        var returned = 0
        mount(onReturn = { returned++ })

        compose.onNodeWithTag(TestTags.CONFERENCE_MINI).performClick()

        assertThat(returned).isEqualTo(1)
    }

    @Test
    fun `the bar says what tapping it does`() {
        // Sighted users have the whole screen for context; a screen reader has the
        // bar. Its text names the meeting and its state, so without a label on the
        // action itself the one gesture back into a call reads as «активировать».
        mount()

        val onClick = compose.onNodeWithTag(TestTags.CONFERENCE_MINI)
            .fetchSemanticsNode().config[SemanticsActions.OnClick]

        assertThat(onClick.label).isEqualTo(res.getString(R.string.conf_mini_return))
    }

    @Test
    fun `pressing a control does not also navigate away`() {
        // The controls sit inside the tappable bar. A press that fell through to
        // it would hang up and change screens on one tap — and the screen change
        // is the half that gets noticed, so the mute would look like it worked.
        var returned = 0
        var toggled = 0
        mount(onReturn = { returned++ }, onToggleMic = { toggled++ })

        compose.onNodeWithTag(TestTags.CONFERENCE_MINI_MIC).performClick()

        assertThat(toggled).isEqualTo(1)
        assertThat(returned).isEqualTo(0)
    }
}
