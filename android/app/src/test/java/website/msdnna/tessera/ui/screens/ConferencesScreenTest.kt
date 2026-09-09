package website.msdnna.tessera.ui.screens

import android.content.Context
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
import website.msdnna.tessera.data.model.Conference
import website.msdnna.tessera.data.model.ConferenceStatus
import website.msdnna.tessera.ui.TestTags
import website.msdnna.tessera.ui.theme.TesseraTheme

/**
 * The conference list's row (#2896 §2), rendered for real. It is driven as a
 * stateless piece: no repository, no socket — what is checked here is what the
 * row decides on its own.
 *
 * The screen size is pinned for the same reason as [HelpScreenTest]: Robolectric's
 * default 320×470px drops a tap below the edge in silence.
 */
@RunWith(RobolectricTestRunner::class)
@Config(qualifiers = "ru-w411dp-h891dp-xhdpi")
class ConferencesScreenTest {
    @get:Rule
    val compose = createComposeRule()

    private val res = ApplicationProvider.getApplicationContext<Context>().resources

    private fun conference(active: Int) = Conference(
        id = "c1",
        title = "Standup",
        status = ConferenceStatus.LIVE,
        startedAt = "2026-09-05T09:02:00Z",
        createdByName = "Ада",
        participantCount = 4,
        activeCount = active,
    )

    private fun mountRow(
        conference: Conference,
        confirming: Boolean = false,
        onAskDelete: () -> Unit = {},
        onConfirmDelete: () -> Unit = {},
    ) {
        compose.setContent {
            TesseraTheme {
                ConferenceRow(
                    conference = conference,
                    confirmingDelete = confirming,
                    onClick = {},
                    onAskDelete = onAskDelete,
                    onConfirmDelete = onConfirmDelete,
                    onCancelDelete = {},
                )
            }
        }
    }

    /** Nodes carrying [text] somewhere in them — the row wraps its facts in longer
     *  sentences, so the match is by substring. */
    private fun nodesWith(text: String) =
        compose.onAllNodesWithText(text, substring = true).fetchSemanticsNodes()

    @Test
    fun `a row keyed by id carries the invite count and the author`() {
        mountRow(conference(active = 0))
        compose.onNodeWithTag(TestTags.conferenceRow("c1")).assertIsDisplayed()
        assertThat(nodesWith(res.getString(R.string.conf_row_invited, 4))).isNotEmpty()
        assertThat(nodesWith(res.getString(R.string.conf_row_author, "Ада"))).isNotEmpty()
    }

    /** «В комнате: 0» on a call nobody is in duplicates the status pill; the row
     *  drops the fact instead of printing a zero. */
    @Test
    fun `an empty room says nothing about who is in it`() {
        mountRow(conference(active = 0))
        assertThat(nodesWith(res.getString(R.string.conf_row_in_room, 0))).isEmpty()
    }

    @Test
    fun `an occupied room reports its headcount`() {
        mountRow(conference(active = 2))
        assertThat(nodesWith(res.getString(R.string.conf_row_in_room, 2))).isNotEmpty()
    }

    /** Deleting takes two taps: the trash asks, the popover confirms. A row that
     *  deleted on the first tap would take the chat and the recordings with it. */
    @Test
    fun `the trash asks before it deletes`() {
        var asked = 0
        mountRow(conference(active = 0), onAskDelete = { asked++ })
        compose.onNodeWithTag(TestTags.conferenceDelete("c1")).performClick()
        assertThat(asked).isEqualTo(1)
    }

    // The schedule dialog is not driven from here. Its text fields keep the
    // composition from ever reaching idle under Robolectric — `setContent`
    // itself dies on `AppNotIdleException`, and pinning the clock by hand only
    // starves it of the frames it is waiting for. What the dialog decides is
    // covered in `ConferencesTest` through `conferenceDraft`, the single read
    // that both enables its button and builds its payload.
}
