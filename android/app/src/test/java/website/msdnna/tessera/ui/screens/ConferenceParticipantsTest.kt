package website.msdnna.tessera.ui.screens

import android.content.Context
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.getUnclippedBoundsInRoot
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithTag
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onRoot
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTouchInput
import androidx.compose.ui.unit.height
import androidx.test.core.app.ApplicationProvider
import com.google.common.truth.Truth.assertThat
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import website.msdnna.tessera.R
import website.msdnna.tessera.data.conference.ConfPeer
import website.msdnna.tessera.data.conference.ConfPerson
import website.msdnna.tessera.data.conference.ConfRoomState
import website.msdnna.tessera.data.conference.ConfSession
import website.msdnna.tessera.data.conference.ROLE_HOST
import website.msdnna.tessera.data.conference.ROLE_MEMBER
import website.msdnna.tessera.ui.TestTags
import website.msdnna.tessera.ui.theme.TesseraTheme
import website.msdnna.tessera.ui.viewmodels.ConferenceRoomUiState
import website.msdnna.tessera.util.ConfMediaGrant
import website.msdnna.tessera.util.ConfMediaStatus
import website.msdnna.tessera.util.ConfQuality
import website.msdnna.tessera.util.ConfSheetDetent

/**
 * The participants panel (#2896 §6), rendered over a call that never existed.
 *
 * Same shape as [ConferenceRoomTest]: nobody publishes anything, because a tile
 * with a track mounts the SDK's renderer and Robolectric has no EGL surface.
 * Screen size pinned — the default 320×470px drops a tap below the edge in
 * silence.
 */
@RunWith(RobolectricTestRunner::class)
@Config(qualifiers = "ru-w411dp-h891dp-xhdpi")
class ConferenceParticipantsTest {
    @get:Rule
    val compose = createComposeRule()

    private val res = ApplicationProvider.getApplicationContext<Context>().resources

    private fun person(
        id: String,
        name: String = id,
        role: String = ROLE_MEMBER,
        mic: Boolean = true,
        forceMuted: Boolean = false,
        handAt: String? = null,
    ) = ConfPerson(userId = id, name = name, role = role, conns = 1, mic = mic, forceMuted = forceMuted, handAt = handAt)

    private fun peer(identity: String) = ConfPeer(
        identity = identity,
        sid = "sid-$identity",
        name = identity,
        local = identity == "me",
        speaking = false,
        micOn = true,
        camOn = false,
        quality = ConfQuality.EXCELLENT,
    )

    private fun state(
        people: List<ConfPerson>,
        canModerate: Boolean = false,
        meId: String = "me",
        mediaPeers: List<ConfPeer> = emptyList(),
        confirmingKick: String = "",
        localMuted: Set<String> = emptySet(),
    ) = ConferenceRoomUiState(
        session = ConfSession(
            conferenceId = "c1",
            status = ConfMediaStatus.LIVE,
            peers = mediaPeers,
            localMuted = localMuted,
        ),
        room = ConfRoomState(connected = true, meId = meId, canModerate = canModerate, people = people),
        grant = ConfMediaGrant(mic = true),
        panelOpen = true,
        confirmingKick = confirmingKick,
    )

    private fun mount(
        state: ConferenceRoomUiState,
        onForceMute: (String, Boolean) -> Unit = { _, _ -> },
        onAskKick: (String) -> Unit = {},
        onConfirmKick: () -> Unit = {},
        onLocalMute: (String) -> Unit = {},
        onClose: () -> Unit = {},
    ) {
        compose.setContent {
            TesseraTheme {
                ConferenceParticipantsPanel(
                    state = state,
                    onClose = onClose,
                    onForceMute = onForceMute,
                    onAskKick = onAskKick,
                    onCancelKick = {},
                    onConfirmKick = onConfirmKick,
                    onLocalMute = onLocalMute,
                    onVolume = { _, _ -> },
                    onDismissDenied = {},
                )
            }
        }
    }

    private fun nodesWith(text: String) =
        compose.onAllNodesWithText(text, substring = true).fetchSemanticsNodes()

    @Test
    fun `the roster puts a raised hand at the top`() {
        mount(
            state(
                listOf(
                    person("me", "Я"),
                    person("ann", "Аня"),
                    person("bob", "Боря", handAt = "2026-09-05T10:01:00Z"),
                ),
            ),
        )

        compose.onNodeWithTag(TestTags.CONFERENCE_PANEL).assertIsDisplayed()
        compose.onNodeWithTag(TestTags.conferenceHandUp("bob")).assertIsDisplayed()
        // Only the raised hand carries the marker: the rows are all there either
        // way, so the row tag alone would pass in a build that lost it.
        assertThat(hasTag(TestTags.conferenceHandUp("ann"))).isFalse()
    }

    @Test
    fun `an empty room says so instead of showing nothing`() {
        mount(state(emptyList()))
        compose.onNodeWithTag(TestTags.CONFERENCE_PANEL_EMPTY).assertIsDisplayed()
    }

    /** Two different silences, and the row says which — «заглушён ведущим» is
     *  not the same statement as a microphone somebody turned off. */
    @Test
    fun `a force-muted participant is marked as silenced by a host`() {
        mount(state(listOf(person("ann", "Аня", mic = false, forceMuted = true), person("bob", "Боря", mic = false))))

        compose.onNodeWithTag(TestTags.conferenceForced("ann")).assertIsDisplayed()
        assertThat(hasTag(TestTags.conferenceForced("bob"))).isFalse()
        assertThat(nodesWith(res.getString(R.string.conf_panel_mic_off))).isNotEmpty()
    }

    @Test
    fun `a plain member sees no moderation controls at all`() {
        mount(state(listOf(person("me", "Я"), person("ann", "Аня")), canModerate = false))

        assertThat(hasTag(TestTags.conferenceForceMute("ann"))).isFalse()
        assertThat(hasTag(TestTags.conferenceKick("ann"))).isFalse()
    }

    @Test
    fun `a moderator gets both controls on somebody else and none on themselves`() {
        mount(state(listOf(person("me", "Я"), person("ann", "Аня")), canModerate = true))

        compose.onNodeWithTag(TestTags.conferenceForceMute("ann")).assertIsDisplayed()
        compose.onNodeWithTag(TestTags.conferenceKick("ann")).assertIsDisplayed()
        assertThat(hasTag(TestTags.conferenceForceMute("me"))).isFalse()
        assertThat(hasTag(TestTags.conferenceKick("me"))).isFalse()
    }

    /** The server refuses «cannot kick a host», so the button is not offered —
     *  a control whose every press fails teaches that controls do not work. */
    @Test
    fun `a host can be muted but not removed`() {
        mount(state(listOf(person("me", "Я"), person("boss", "Ведущий", role = ROLE_HOST)), canModerate = true))

        compose.onNodeWithTag(TestTags.conferenceForceMute("boss")).assertIsDisplayed()
        assertThat(hasTag(TestTags.conferenceKick("boss"))).isFalse()
    }

    @Test
    fun `force-muting asks for the opposite of what is on the row`() {
        var asked: Pair<String, Boolean>? = null
        mount(
            state(listOf(person("me", "Я"), person("ann", "Аня", forceMuted = true)), canModerate = true),
            onForceMute = { id, muted -> asked = id to muted },
        )

        compose.onNodeWithTag(TestTags.conferenceForceMute("ann")).performClick()
        assertThat(asked).isEqualTo("ann" to false)
    }

    /** Visible to the whole room and impossible to take back, so it goes
     *  through a confirmation rather than straight off the row. */
    @Test
    fun `a kick is confirmed before it is sent`() {
        var asked = ""
        mount(
            state(listOf(person("me", "Я"), person("ann", "Аня")), canModerate = true),
            onAskKick = { asked = it },
        )

        compose.onNodeWithTag(TestTags.conferenceKick("ann")).performClick()
        assertThat(asked).isEqualTo("ann")
        // The panel alone does not send it: the confirmation is a separate state.
        assertThat(nodesWith("Аня")).isNotEmpty()
    }

    @Test
    fun `the confirmation names the person it is about`() {
        mount(state(listOf(person("me", "Я"), person("ann", "Аня")), canModerate = true, confirmingKick = "ann"))

        assertThat(nodesWith(res.getString(R.string.conf_panel_confirm_kick, "Аня"))).isNotEmpty()
    }

    /** And the confirming button is what sends it. Asserted on the button rather
     *  than on the dialog being up: the dialog is on screen either way, so it
     *  would pass in a build where pressing «Удалить» does nothing. */
    @Test
    fun `confirming is what actually sends the kick`() {
        var confirmed = false
        mount(
            state(listOf(person("me", "Я"), person("ann", "Аня")), canModerate = true, confirmingKick = "ann"),
            onConfirmKick = { confirmed = true },
        )

        compose.onNodeWithTag(TestTags.CONFERENCE_KICK_CONFIRM).performClick()
        assertThat(confirmed).isTrue()
    }

    // ── local playback, which never leaves this phone ─────────────────────

    /** Somebody in the room socket with no media descriptor is joining, or in
     *  the call with no devices: there is no audio to turn down. */
    @Test
    fun `the volume controls need audio to actually be arriving`() {
        mount(
            state(
                listOf(person("me", "Я"), person("ann", "Аня"), person("joining", "Новенький")),
                mediaPeers = listOf(peer("me"), peer("ann")),
            ),
        )

        compose.onNodeWithTag(TestTags.conferenceVolume("ann")).assertIsDisplayed()
        assertThat(hasTag(TestTags.conferenceVolume("joining"))).isFalse()
        // Never on our own row — the local track is not played back to us.
        assertThat(hasTag(TestTags.conferenceVolume("me"))).isFalse()
    }

    @Test
    fun `the local mute is a local decision and reports it`() {
        var muted = ""
        mount(
            state(listOf(person("me", "Я"), person("ann", "Аня")), mediaPeers = listOf(peer("me"), peer("ann"))),
            onLocalMute = { muted = it },
        )

        compose.onNodeWithTag(TestTags.conferenceLocalMute("ann")).performClick()
        assertThat(muted).isEqualTo("ann")
    }

    @Test
    fun `our own force-mute is said out loud rather than left to the toolbar`() {
        val base = state(listOf(person("me", "Я", forceMuted = true)))
        mount(base)

        assertThat(nodesWith(res.getString(R.string.conf_panel_you_force_muted))).isNotEmpty()
    }

    @Test
    fun `a refusal is shown in the panel the button lives in`() {
        val base = state(listOf(person("me", "Я"), person("ann", "Аня")), canModerate = true)
        mount(base.copy(room = base.room.copy(denied = website.msdnna.tessera.data.conference.ConfDenied("kick", "not a moderator"))))

        compose.onNodeWithTag(TestTags.CONFERENCE_DENIED).assertIsDisplayed()
        assertThat(nodesWith(res.getString(R.string.conf_denied_kick))).isNotEmpty()
    }

    @Test
    fun `the close button closes it`() {
        var closed = false
        mount(state(listOf(person("me", "Я"))), onClose = { closed = true })

        compose.onNodeWithTag(TestTags.CONFERENCE_PANEL_CLOSE).performClick()
        // The panel leaves through its exit animation, so the caller hears about
        // it a frame later — see the same wait in `ConferenceChatSheetTest`.
        compose.waitUntil { closed }
        assertThat(closed).isTrue()
    }

    /**
     * The bug the second round of review found: collapsed from full screen, the
     * sheet stopped somewhere near the dismissal threshold with its contents cut
     * in half instead of landing on a detent.
     *
     * The rule was never wrong — «вниз из полного на шаг» is specced in
     * `ConferenceSheetTest`. What was wrong is that the height it was supposed to
     * animate to was a coroutine racing the ones the drag itself had launched.
     * So the assertion here is deliberately about geometry and not about the
     * rule: where the sheet actually ends up on screen.
     */
    @Test
    fun `collapsed from full screen the sheet lands on half, not between detents`() {
        mount(state(listOf(person("me", "Я"), person("ann", "Аня"))))

        drag(-DRAG_PX)
        drag(DRAG_PX * 2)

        val screen = compose.onRoot().getUnclippedBoundsInRoot().height
        val sheet = compose.onNodeWithTag(TestTags.CONFERENCE_PANEL).getUnclippedBoundsInRoot().height
        assertThat(sheet.value).isWithin(SLACK_DP).of(screen.value * ConfSheetDetent.HALF.fraction)
    }

    /** The other half of the same rule: from half, the same pull is a dismissal. */
    @Test
    fun `pulled down from half the sheet closes`() {
        var closed = false
        mount(state(listOf(person("me", "Я"))), onClose = { closed = true })

        drag(DRAG_PX)

        compose.waitUntil { closed }
        assertThat(closed).isTrue()
    }

    /**
     * Moderation used to be two labelled buttons stacked under the person, each
     * of them taller than the row they belonged to — which read as controls for
     * the *next* name down.
     */
    @Test
    fun `the moderation controls stay inside the row they act on`() {
        mount(state(listOf(person("me", "Я"), person("ann", "Аня")), canModerate = true))

        val row = compose.onNodeWithTag(TestTags.conferencePerson("ann")).getUnclippedBoundsInRoot()
        for (tag in listOf(TestTags.conferenceForceMute("ann"), TestTags.conferenceKick("ann"))) {
            val control = compose.onNodeWithTag(tag).getUnclippedBoundsInRoot()
            assertThat(control.height.value).isLessThan(row.height.value)
            assertThat(control.top.value).isAtLeast(row.top.value)
            assertThat(control.bottom.value).isAtMost(row.bottom.value)
        }
    }

    /** A drag on the grabber, positive downwards. One move rather than a swipe:
     *  the sheet reads velocity, and a gesture split into steps is not a flick. */
    private fun drag(dy: Float) {
        compose.onNodeWithTag(TestTags.CONFERENCE_PANEL_HANDLE).performTouchInput {
            down(center)
            moveBy(Offset(0f, dy))
            up()
        }
        compose.waitForIdle()
    }

    /** True when at least one node carries the tag — `onNodeWithTag` throws
     *  rather than answering, which is not what an absence assertion needs. */
    private fun hasTag(tag: String): Boolean =
        compose.onAllNodesWithTag(tag).fetchSemanticsNodes().isNotEmpty()

    private companion object {
        /** Well past both thresholds (56dp of travel) at 2× density. */
        const val DRAG_PX = 600f

        /** Rounding, and the couple of dp the settle animation may still be short. */
        const val SLACK_DP = 12f
    }
}
