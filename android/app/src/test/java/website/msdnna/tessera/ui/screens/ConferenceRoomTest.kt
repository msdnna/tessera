package website.msdnna.tessera.ui.screens

import android.content.Context
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.assertTextEquals
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
import website.msdnna.tessera.data.conference.ConfPeer
import website.msdnna.tessera.data.conference.ConfPerson
import website.msdnna.tessera.data.conference.ConfRoomState
import website.msdnna.tessera.data.conference.ConfSession
import website.msdnna.tessera.ui.TestTags
import website.msdnna.tessera.ui.theme.TesseraTheme
import website.msdnna.tessera.ui.viewmodels.ConferenceRoomUiState
import website.msdnna.tessera.util.ConfAudioRoute
import website.msdnna.tessera.util.ConfJoinReason
import website.msdnna.tessera.util.ConfMediaGrant
import website.msdnna.tessera.util.ConfMediaStatus
import website.msdnna.tessera.util.ConfQuality

/**
 * The room screen (#2896 §5), rendered for real against a call that never
 * existed — no SFU, no token, no microphone.
 *
 * Every peer here publishes nothing, which is not a shortcut: a tile with a
 * track mounts the SDK's renderer, and an EGL surface is not a thing Robolectric
 * has. The camera-off path is also the one the call spends most of its time in.
 *
 * Screen size pinned as in [ConferencesScreenTest] — the default 320×470px drops
 * a tap below the edge in silence.
 */
@RunWith(RobolectricTestRunner::class)
@Config(qualifiers = "ru-w411dp-h891dp-xhdpi")
class ConferenceRoomTest {
    @get:Rule
    val compose = createComposeRule()

    private val res = ApplicationProvider.getApplicationContext<Context>().resources

    private fun peer(
        identity: String,
        name: String = identity,
        local: Boolean = false,
        speaking: Boolean = false,
        micOn: Boolean = true,
        quality: ConfQuality = ConfQuality.EXCELLENT,
    ) = ConfPeer(
        identity = identity,
        sid = "sid-$identity",
        name = name,
        local = local,
        speaking = speaking,
        micOn = micOn,
        camOn = false,
        quality = quality,
    )

    private fun mount(
        state: ConferenceRoomUiState,
        onToggleMic: () -> Unit = {},
        onSelectRoute: (ConfAudioRoute) -> Unit = {},
        onRetry: () -> Unit = {},
        onHangup: () -> Unit = {},
        onToggleStageOnly: () -> Unit = {},
        chatUnread: Int = 0,
        onOpenChat: () -> Unit = {},
    ) {
        compose.setContent {
            TesseraTheme {
                ConferenceRoomBody(
                    state = state,
                    onToggleMic = onToggleMic,
                    onToggleCam = {},
                    onSwitchCamera = {},
                    onOpenRoutes = {},
                    onCloseRoutes = {},
                    onSelectRoute = onSelectRoute,
                    onToggleStageOnly = onToggleStageOnly,
                    onRetry = onRetry,
                    onHangup = onHangup,
                    chatUnread = chatUnread,
                    onOpenChat = onOpenChat,
                )
            }
        }
    }

    private fun live(vararg peers: ConfPeer, mic: Boolean = true) = ConferenceRoomUiState(
        session = ConfSession(
            conferenceId = "c1",
            status = ConfMediaStatus.LIVE,
            peers = peers.toList(),
            mic = mic,
        ),
        grant = ConfMediaGrant(mic = true),
    )

    private fun nodesWith(text: String) =
        compose.onAllNodesWithText(text, substring = true).fetchSemanticsNodes()

    @Test
    fun `the speaker holds the stage and everyone else lines up in the strip`() {
        mount(live(peer("me", "Я", local = true), peer("ann", "Аня"), peer("bob", "Боря", speaking = true)))

        compose.onNodeWithTag(TestTags.CONFERENCE_STAGE).assertIsDisplayed()
        compose.onNodeWithTag(TestTags.CONFERENCE_STRIP).assertIsDisplayed()
        // The stage tile and the strip tile both carry the identity tag, so
        // «bob is on screen» is asserted without depending on which slot he is in.
        compose.onNodeWithTag(TestTags.conferenceTile("bob")).assertIsDisplayed()
        compose.onNodeWithTag(TestTags.conferenceTile("ann")).assertIsDisplayed()
    }

    @Test
    fun `alone in the call there is no strip at all`() {
        mount(live(peer("me", "Я", local = true)))

        compose.onNodeWithTag(TestTags.CONFERENCE_STAGE).assertIsDisplayed()
        // An empty strip below the stage reads as a row that failed to load.
        compose.onNodeWithTag(TestTags.CONFERENCE_STRIP).assertDoesNotExist()
    }

    @Test
    fun `a muted participant is marked on their tile`() {
        mount(live(peer("me", "Я", local = true), peer("ann", "Аня", micOn = false)))

        compose.onNodeWithTag(TestTags.conferenceTile("ann")).assertIsDisplayed()
        compose.onNodeWithTag(TestTags.conferenceTileMuted("ann")).assertIsDisplayed()
        // And nobody else: a marker on every tile says nothing about anybody.
        compose.onNodeWithTag(TestTags.conferenceTileMuted("me")).assertDoesNotExist()
    }

    @Test
    fun `a weak link gets a badge`() {
        mount(live(peer("me", "Я", local = true, quality = ConfQuality.POOR)))

        compose.onNodeWithTag(TestTags.CONFERENCE_QUALITY).assertIsDisplayed()
    }

    @Test
    fun `a healthy link gets none`() {
        mount(live(peer("me", "Я", local = true)))

        // A badge on every face in a working call is a badge nobody reads.
        compose.onNodeWithTag(TestTags.CONFERENCE_QUALITY).assertDoesNotExist()
    }

    @Test
    fun `an install without an SFU says so and offers no retry`() {
        mount(
            ConferenceRoomUiState(
                session = ConfSession(status = ConfMediaStatus.UNAVAILABLE, reason = ConfJoinReason.NOT_CONFIGURED),
                grant = ConfMediaGrant(mic = true),
            ),
        )

        compose.onNodeWithTag(TestTags.CONFERENCE_BANNER).assertIsDisplayed()
        assertThat(nodesWith(res.getString(R.string.conf_media_unavailable))).isNotEmpty()
        // Pressing it again cannot configure LiveKit on the server.
        compose.onNodeWithTag(TestTags.CONFERENCE_RETRY).assertDoesNotExist()
    }

    @Test
    fun `a failure offers a retry and the press reaches the caller`() {
        var retried = 0
        mount(
            ConferenceRoomUiState(
                session = ConfSession(status = ConfMediaStatus.ERROR, error = "dial tcp: refused"),
                grant = ConfMediaGrant(mic = true),
            ),
            onRetry = { retried += 1 },
        )

        // The server's own sentence, not a generic line.
        assertThat(nodesWith("dial tcp: refused")).isNotEmpty()
        compose.onNodeWithTag(TestTags.CONFERENCE_RETRY).performClick()
        assertThat(retried).isEqualTo(1)
    }

    @Test
    fun `a live call without the microphone permission says nobody can hear us`() {
        mount(
            ConferenceRoomUiState(
                session = ConfSession(status = ConfMediaStatus.LIVE, peers = listOf(peer("me", "Я", local = true))),
                grant = ConfMediaGrant(mic = false),
            ),
        )

        assertThat(nodesWith(res.getString(R.string.conf_media_no_mic))).isNotEmpty()
    }

    @Test
    fun `a force-muted microphone swallows the press instead of publishing`() {
        var toggles = 0
        mount(
            ConferenceRoomUiState(
                session = ConfSession(
                    status = ConfMediaStatus.LIVE,
                    peers = listOf(peer("me", "Я", local = true)),
                    forceMuted = true,
                ),
                grant = ConfMediaGrant(mic = true),
            ),
            onToggleMic = { toggles += 1 },
        )

        compose.onNodeWithTag(TestTags.CONFERENCE_MIC).performClick()
        // The server would refuse the publish anyway; the button is there to say
        // so, not to try (#2878).
        assertThat(toggles).isEqualTo(0)
    }

    @Test
    fun `the routing menu lists the outputs the phone offers`() {
        var picked: ConfAudioRoute? = null
        mount(
            ConferenceRoomUiState(
                session = ConfSession(
                    status = ConfMediaStatus.LIVE,
                    peers = listOf(peer("me", "Я", local = true)),
                    route = ConfAudioRoute.SPEAKER,
                    routes = listOf(ConfAudioRoute.WIRED_HEADSET, ConfAudioRoute.SPEAKER),
                ),
                grant = ConfMediaGrant(mic = true),
                routeMenu = true,
            ),
            onSelectRoute = { picked = it },
        )

        compose.onNodeWithTag(TestTags.conferenceRoute(ConfAudioRoute.WIRED_HEADSET)).performClick()
        assertThat(picked).isEqualTo(ConfAudioRoute.WIRED_HEADSET)
        // An output the phone is not offering must not be listed: selecting it
        // would be a no-op the menu then shows as chosen.
        compose.onNodeWithTag(TestTags.conferenceRoute(ConfAudioRoute.BLUETOOTH)).assertDoesNotExist()
    }

    @Test
    fun `hanging up reaches the caller`() {
        var hungUp = 0
        mount(live(peer("me", "Я", local = true)), onHangup = { hungUp += 1 })

        compose.onNodeWithTag(TestTags.CONFERENCE_HANGUP).performClick()
        assertThat(hungUp).isEqualTo(1)
    }

    @Test
    fun `folding the chrome away leaves only the stage`() {
        mount(
            live(peer("me", "Я", local = true), peer("ann", "Аня")).copy(stageOnly = true),
        )

        compose.onNodeWithTag(TestTags.CONFERENCE_STAGE).assertIsDisplayed()
        compose.onNodeWithTag(TestTags.CONFERENCE_STRIP).assertDoesNotExist()
        compose.onNodeWithTag(TestTags.CONFERENCE_MIC).assertDoesNotExist()
    }

    @Test
    fun `the fold button reaches the caller`() {
        var toggles = 0
        mount(live(peer("me", "Я", local = true)), onToggleStageOnly = { toggles += 1 })

        compose.onNodeWithTag(TestTags.CONFERENCE_FULLSCREEN).performClick()
        assertThat(toggles).isEqualTo(1)
    }

    // ── the roster's half of the toolbar (#2896 §6) ───────────────────────

    /** The count is the hands and not the heads: a badge reading «5» on every
     *  call is a badge nobody looks at. */
    @Test
    fun `the toolbar badges how many hands are up`() {
        mount(
            live(peer("me", "Я", local = true)).copy(
                room = ConfRoomState(
                    connected = true,
                    meId = "me",
                    people = listOf(
                        ConfPerson(userId = "me", name = "Я"),
                        ConfPerson(userId = "ann", name = "Аня", handAt = "2026-09-05T10:01:00Z"),
                        ConfPerson(userId = "bob", name = "Боря", handAt = "2026-09-05T10:02:00Z"),
                    ),
                ),
            ),
        )

        compose.onNodeWithTag(TestTags.CONFERENCE_HANDS).assertTextEquals("2")
    }

    @Test
    fun `nobody waiting means no badge at all`() {
        mount(
            live(peer("me", "Я", local = true)).copy(
                room = ConfRoomState(connected = true, meId = "me", people = listOf(ConfPerson(userId = "me", name = "Я"))),
            ),
        )

        // The button is there either way — it is the badge that has to go.
        compose.onNodeWithTag(TestTags.CONFERENCE_PEOPLE).assertIsDisplayed()
        compose.onNodeWithTag(TestTags.CONFERENCE_HANDS).assertDoesNotExist()
    }

    /** Nothing in the roster half works without the room socket, and a control
     *  that only ever fails teaches that controls do not work. */
    @Test
    fun `the roster controls wait for the room socket`() {
        mount(live(peer("me", "Я", local = true)))

        compose.onNodeWithTag(TestTags.CONFERENCE_PEOPLE).assertIsNotEnabled()
        compose.onNodeWithTag(TestTags.CONFERENCE_HAND).assertIsNotEnabled()
        // The chat is HTTP, but its nudges are not: without the socket the badge
        // would never move, so the button waits with the rest of them (§7).
        compose.onNodeWithTag(TestTags.CONFERENCE_CHAT_OPEN).assertIsNotEnabled()
    }

    @Test
    fun `the chat button badges unread lines and opens the sheet`() {
        var opened = 0
        mount(connected(), chatUnread = 3, onOpenChat = { opened += 1 })

        compose.onNodeWithTag(TestTags.CONFERENCE_CHAT_UNREAD).assertTextEquals("3")
        compose.onNodeWithTag(TestTags.CONFERENCE_CHAT_OPEN).performClick()
        assertThat(opened).isEqualTo(1)
    }

    @Test
    fun `a chat nobody has spoken in carries no badge`() {
        mount(connected(), chatUnread = 0)

        // Same shape as the hands badge: the button stays, the badge is what a
        // quiet chat has to lose.
        compose.onNodeWithTag(TestTags.CONFERENCE_CHAT_OPEN).assertIsDisplayed()
        compose.onNodeWithTag(TestTags.CONFERENCE_CHAT_UNREAD).assertDoesNotExist()
    }

    /**
     * Eight controls at 48dp with 10dp between them are 454dp wide, and this
     * screen is 411dp. A `Row` would clip the overflow in silence — and what it
     * clips is the button on the end, which is «Завершить».
     */
    @Test
    fun `the toolbar keeps every control on a phone-width screen`() {
        mount(connected(), chatUnread = 1)

        compose.onNodeWithTag(TestTags.CONFERENCE_MIC).assertIsDisplayed()
        compose.onNodeWithTag(TestTags.CONFERENCE_CHAT_OPEN).assertIsDisplayed()
        compose.onNodeWithTag(TestTags.CONFERENCE_HANGUP).assertIsDisplayed()
    }

    private fun connected() = live(peer("me", "Я", local = true)).copy(
        room = ConfRoomState(connected = true, meId = "me", people = listOf(ConfPerson(userId = "me", name = "Я"))),
    )
}
