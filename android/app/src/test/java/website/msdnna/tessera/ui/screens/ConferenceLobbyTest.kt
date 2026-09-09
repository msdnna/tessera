package website.msdnna.tessera.ui.screens

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithTag
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import com.google.common.truth.Truth.assertThat
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import website.msdnna.tessera.data.model.Conference
import website.msdnna.tessera.data.model.ConferenceParticipant
import website.msdnna.tessera.data.model.ConferenceRecording
import website.msdnna.tessera.data.model.ConferenceStatus
import website.msdnna.tessera.data.model.RecordingStatus
import website.msdnna.tessera.ui.TestTags
import website.msdnna.tessera.ui.UiText
import website.msdnna.tessera.ui.theme.TesseraTheme
import website.msdnna.tessera.ui.viewmodels.ConferenceLobbyUiState

/**
 * The lobby (#2896 §3) rendered for real, driven as a stateless piece: a
 * hand-built state in, no repository and no socket.
 *
 * What is checked here is the wiring the pure rules cannot see — that the state's
 * answers reach the right controls. Which answer is correct is
 * `ConferenceLobbyTest`'s business.
 *
 * The screen size is pinned as in [ConferencesScreenTest]: Robolectric's default
 * 320×470px drops a tap below the edge in silence.
 */
@RunWith(RobolectricTestRunner::class)
@Config(qualifiers = "ru-w411dp-h891dp-xhdpi")
class ConferenceLobbyTest {
    @get:Rule
    val compose = createComposeRule()

    private val me = "u-me"

    private fun seat(
        userId: String,
        role: String = "member",
        joined: String? = null,
        name: String? = null,
    ) = ConferenceParticipant(
        conferenceId = "c1",
        userId = userId,
        role = role,
        invitedAt = "2026-09-05T08:00:00Z",
        joinedAt = joined,
        userName = name,
    )

    private fun state(
        status: String = ConferenceStatus.LIVE,
        participants: List<ConferenceParticipant> = emptyList(),
        workspaceRole: String = "member",
    ) = ConferenceLobbyUiState(
        loading = false,
        conference = Conference(
            id = "c1",
            workspaceId = "w1",
            title = "Планёрка",
            status = status,
            createdBy = "u-other",
            startedAt = "2026-09-05T09:00:00Z",
        ),
        participants = participants,
        meId = me,
        workspaceRole = workspaceRole,
    )

    private fun mount(
        state: ConferenceLobbyUiState,
        onJoin: () -> Unit = {},
        onAskEnd: () -> Unit = {},
        onDownloadRecording: (ConferenceRecording) -> Unit = {},
        onAskDeleteRecording: (String) -> Unit = {},
        onConfirmDeleteRecording: () -> Unit = {},
    ) {
        compose.setContent {
            TesseraTheme {
                ConferenceLobbyBody(
                    state = state,
                    onJoin = onJoin,
                    onLeave = {},
                    onAskEnd = onAskEnd,
                    onConfirmEnd = {},
                    onCancelEnd = {},
                    onInvite = {},
                    onDownloadRecording = onDownloadRecording,
                    onAskDeleteRecording = onAskDeleteRecording,
                    onConfirmDeleteRecording = onConfirmDeleteRecording,
                )
            }
        }
    }

    private fun exists(tag: String) =
        compose.onAllNodesWithTag(tag).fetchSemanticsNodes().isNotEmpty()

    /** Join and leave are one control in two states — never both at once. */
    @Test
    fun `an outsider is offered the way in`() {
        var joined = 0
        mount(state(), onJoin = { joined++ })
        compose.onNodeWithTag(TestTags.CONFERENCE_JOIN).performClick()
        assertThat(joined).isEqualTo(1)
        assertThat(exists(TestTags.CONFERENCE_LEAVE)).isFalse()
    }

    @Test
    fun `somebody in the room is offered the way out`() {
        mount(state(participants = listOf(seat(me, joined = "2026-09-05T09:01:00Z"))))
        compose.onNodeWithTag(TestTags.CONFERENCE_LEAVE).assertIsDisplayed()
        assertThat(exists(TestTags.CONFERENCE_JOIN)).isFalse()
    }

    /**
     * «Завершить» ends the call for everyone, so it is the one control that must
     * not appear on a guess: the server answers 403 and the tap becomes a bug
     * report about a button that does nothing.
     */
    @Test
    fun `a plain member gets no end button`() {
        mount(state(participants = listOf(seat(me, joined = "2026-09-05T09:01:00Z"))))
        assertThat(exists(TestTags.CONFERENCE_END)).isFalse()
    }

    @Test
    fun `a host is offered to end a running call`() {
        var asked = 0
        mount(
            state(participants = listOf(seat(me, role = "host", joined = "2026-09-05T09:01:00Z"))),
            onAskEnd = { asked++ },
        )
        compose.onNodeWithTag(TestTags.CONFERENCE_END).performClick()
        assertThat(asked).isEqualTo(1)
    }

    /** An idle room has nothing to end — a reusable conference pauses back to
     *  `scheduled` (#2879) rather than staying live between sessions. */
    @Test
    fun `an idle room offers no end button even to its host`() {
        mount(
            state(
                status = ConferenceStatus.SCHEDULED,
                participants = listOf(seat(me, role = "host")),
            ),
        )
        assertThat(exists(TestTags.CONFERENCE_END)).isFalse()
    }

    @Test
    fun `an ended call cannot be invited to`() {
        mount(state(status = ConferenceStatus.ENDED))
        assertThat(exists(TestTags.CONFERENCE_INVITE)).isFalse()
    }

    /**
     * The two roster lists are complementary — the same person must not be both
     * in the room and still being called.
     */
    @Test
    fun `the roster separates who is here from who is being called`() {
        mount(
            state(
                participants = listOf(
                    seat("u1", joined = "2026-09-05T09:01:00Z", name = "Ада"),
                    seat("u2", name = "Пётр"),
                ),
            ),
        )
        compose.onNodeWithTag(TestTags.conferenceSeat("u1")).assertIsDisplayed()
        compose.onNodeWithTag(TestTags.conferenceSeat("u2")).assertIsDisplayed()
    }

    // ── recordings (#2896 §8) ────────────────────────────────────────────

    private fun recording(
        id: String,
        status: String = RecordingStatus.COMPLETED,
        startedAt: String = "2026-09-05T09:30:00Z",
    ) = ConferenceRecording(
        id = id,
        conferenceId = "c1",
        status = status,
        startedAt = startedAt,
        durationSec = 125,
    )

    @Test
    fun `a team that never recorded carries no empty section`() {
        mount(state())

        // A permanent «Записей пока нет» under every conference is a line that
        // costs a screenful and says nothing.
        assertThat(exists(TestTags.CONFERENCE_RECORDINGS)).isFalse()
    }

    @Test
    fun `a finished recording can be opened`() {
        var opened = ""
        mount(
            state().copy(recordings = listOf(recording("r1"))),
            onDownloadRecording = { opened = it.id },
        )

        compose.onNodeWithTag(TestTags.CONFERENCE_RECORDINGS).assertIsDisplayed()
        compose.onNodeWithTag(TestTags.conferenceRecordingDownload("r1")).performClick()
        assertThat(opened).isEqualTo("r1")
    }

    /** Shown rather than filtered out — «запись не удалась» is what people came
     *  to this list to find out — but there are no bytes behind the row. */
    @Test
    fun `a failed recording is listed and offers nothing to download`() {
        mount(state().copy(recordings = listOf(recording("r1", status = RecordingStatus.FAILED))))

        compose.onNodeWithTag(TestTags.conferenceRecordingRow("r1")).assertIsDisplayed()
        assertThat(exists(TestTags.conferenceRecordingDownload("r1"))).isFalse()
    }

    @Test
    fun `a recording still being written cannot be opened either`() {
        mount(state().copy(recordings = listOf(recording("r1", status = RecordingStatus.ACTIVE))))

        compose.onNodeWithTag(TestTags.conferenceRecordingRow("r1")).assertIsDisplayed()
        assertThat(exists(TestTags.conferenceRecordingDownload("r1"))).isFalse()
    }

    /** Delete mirrors exactly who may start one, and a member is not it. */
    @Test
    fun `a member is offered no delete`() {
        mount(
            state(participants = listOf(seat(me, joined = "2026-09-05T09:01:00Z")))
                .copy(recordings = listOf(recording("r1"))),
        )

        assertThat(exists(TestTags.conferenceRecordingDelete("r1"))).isFalse()
    }

    /**
     * Unlike a deleted message, which at least existed in somebody's scroll, the
     * file is gone from the disk — so the press that erases it is behind a
     * confirmation and never the one on the row.
     */
    @Test
    fun `a moderator deletes behind a confirmation`() {
        var asked = ""
        mount(
            state(participants = listOf(seat(me, role = "host", joined = "2026-09-05T09:01:00Z")))
                .copy(recordings = listOf(recording("r1"))),
            onAskDeleteRecording = { asked = it },
        )

        compose.onNodeWithTag(TestTags.conferenceRecordingDelete("r1")).performClick()
        assertThat(asked).isEqualTo("r1")
        // The row's own press only asks — nothing is erased until the dialog is.
        assertThat(exists(TestTags.CONFERENCE_RECORDING_DELETE_CONFIRM)).isFalse()
    }

    @Test
    fun `the confirmation is what actually erases the file`() {
        var confirmed = 0
        mount(
            state(participants = listOf(seat(me, role = "host", joined = "2026-09-05T09:01:00Z")))
                .copy(recordings = listOf(recording("r1")), confirmingDeleteId = "r1"),
            onConfirmDeleteRecording = { confirmed += 1 },
        )

        compose.onNodeWithTag(TestTags.CONFERENCE_RECORDING_DELETE_CONFIRM).performClick()
        assertThat(confirmed).isEqualTo(1)
    }

    @Test
    fun `a row with a call in flight cannot be pressed twice`() {
        mount(
            state(participants = listOf(seat(me, role = "host", joined = "2026-09-05T09:01:00Z")))
                .copy(recordings = listOf(recording("r1")), recordingBusyId = "r1"),
        )

        compose.onNodeWithTag(TestTags.conferenceRecordingDownload("r1")).assertIsNotEnabled()
        compose.onNodeWithTag(TestTags.conferenceRecordingDelete("r1")).assertIsNotEnabled()
    }

    /** Printed under the list rather than toasted: this panel is below the fold,
     *  and a toast about a list nobody is looking at is noise. */
    @Test
    fun `a failed download is reported under the list`() {
        mount(
            state().copy(
                recordings = listOf(recording("r1")),
                recordingsError = UiText.Raw("нет связи"),
            ),
        )

        compose.onNodeWithTag(TestTags.CONFERENCE_RECORDINGS_ERROR).assertIsDisplayed()
    }

    @Test
    fun `a running recording is pinned above the finished ones`() {
        mount(
            state().copy(
                recordings = listOf(
                    recording("newest", startedAt = "2026-09-05T12:00:00Z"),
                    recording("live", status = RecordingStatus.ACTIVE, startedAt = "2026-09-04T09:00:00Z"),
                ),
            ),
        )

        // Which order is correct is `ConferenceRecordingsTest`'s business; what
        // this checks is that the ordered list — and not the raw one — is drawn.
        val rows = compose.onAllNodesWithTag(TestTags.conferenceRecordingRow("live")).fetchSemanticsNodes()
        assertThat(rows).isNotEmpty()
        assertThat(rows.first().positionInRoot.y).isLessThan(
            compose.onNodeWithTag(TestTags.conferenceRecordingRow("newest"))
                .fetchSemanticsNode().positionInRoot.y,
        )
    }
}
