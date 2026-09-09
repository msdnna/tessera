package website.msdnna.tessera.e2e

import androidx.compose.ui.test.assertIsDisplayed
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
import website.msdnna.tessera.data.model.ConferenceStatus
import website.msdnna.tessera.ui.TestTags

/**
 * The conferences section end to end (#2896 §9): a real backend, real rows out
 * of Postgres, and the app's own ViewModel and Retrofit stack underneath.
 *
 * **What this tier can and cannot reach.** A conference is two things — a
 * meeting (a plan, a roster, attendance) and a call (an SFU, tokens, tracks).
 * Only the first is HTTP, and only the first is here. There is no LiveKit on the
 * e2e stand and there is no audio device under Robolectric, so nothing below is
 * about media; the media rules are covered as pure functions in `util/`, which
 * is where they were put for exactly this reason.
 *
 * That leaves the seams the unit tier genuinely cannot see: that the list asks
 * the right path, that `status` really filters server-side rather than in the
 * app, that the detail route answers with a roster, and that a delete pressed in
 * the app removes the row from the database rather than only from the screen.
 *
 * Screen size pinned as in the other conference specs — Robolectric's default
 * 320×470px silently drops taps below the fold.
 */
@RunWith(RobolectricTestRunner::class)
@Config(qualifiers = "ru-w411dp-h891dp-xhdpi")
class ConferenceE2eTest {
    private val e2e = E2eRule()
    private val compose = createComposeRule()

    @get:Rule
    val rules: RuleChain = RuleChain.outerRule(e2e).around(compose)

    @Test
    fun `the list shows a conference created from outside`() {
        val fixture = e2e.fixture
        val conference = E2eBackend.createConference(fixture, "Планёрка ${System.nanoTime()}")

        compose.setConferencesContent(fixture, conference.id)

        // Its own title, not the fixture's idea of one: this is the round trip
        // through `title` on the wire, which a spec feeding itself JSON cannot
        // fail on.
        //
        // `awaitTextOn`, not `awaitTextIn`: the row is one node carrying both the
        // tap and the tag, so Compose merges the title into the row's own
        // semantics and there is no descendant left to match. The «in» form waits
        // out its twenty seconds on a row that is right there on screen.
        compose.awaitTextOn(TestTags.conferenceRow(conference.id), conference.title)
    }

    @Test
    fun `the status filter asks the server, not the list already on screen`() {
        val fixture = e2e.fixture
        val scheduled = E2eBackend.createConference(fixture, "Ретро ${System.nanoTime()}")
        val live = E2eBackend.createConference(fixture, "Стендап ${System.nanoTime()}")
        // Joining is what brings a room live (#2879) — there is no «start» route.
        E2eBackend.joinConference(fixture, live.id)

        compose.setConferencesContent(fixture, scheduled.id)
        compose.onNodeWithTag(TestTags.CONFERENCES_SCREEN).assertIsDisplayed()
        compose.onNodeWithTag(TestTags.conferenceFilter(ConferenceStatus.LIVE)).performClick()

        // The one that is live stays; the scheduled one goes. Both halves, because
        // a filter that dropped everything would satisfy the second assertion on
        // its own — and so would a screen stuck on a spinner.
        compose.awaitTag(TestTags.conferenceRow(live.id))
        compose.awaitNoTag(TestTags.conferenceRow(scheduled.id))
    }

    @Test
    fun `opening a conference loads its lobby from the server`() {
        val fixture = e2e.fixture
        val conference = E2eBackend.createConference(fixture, "Разбор ${System.nanoTime()}")
        E2eBackend.joinConference(fixture, conference.id)

        compose.setConferencesContent(fixture, conference.id)
        compose.onNodeWithTag(TestTags.conferenceRow(conference.id)).performClick()

        compose.awaitTag(TestTags.CONFERENCE_LOBBY)
        // The seat taken over HTTP above is the one the lobby paints: this is the
        // detail route and the roster it carries, joined up with the account that
        // registered for this run.
        compose.awaitTag(TestTags.conferenceSeat(fixture.account.user.id))
    }

    @Test
    fun `deleting from the app removes the conference from the database`() {
        val fixture = e2e.fixture
        val doomed = E2eBackend.createConference(fixture, "Лишняя ${System.nanoTime()}")
        val kept = E2eBackend.createConference(fixture, "Нужная ${System.nanoTime()}")

        compose.setConferencesContent(fixture, doomed.id)
        compose.onNodeWithTag(TestTags.conferenceDelete(doomed.id)).performClick()
        // The row's button only asks; the confirmation is what reaches the server.
        compose.awaitTag(TestTags.CONFERENCE_DELETE_CONFIRM)
        compose.onNodeWithTag(TestTags.CONFERENCE_DELETE_CONFIRM).performClick()

        // Asserted against the backend rather than against the screen: a list that
        // merely dropped a row locally looks identical, and it is the difference
        // between a deleted meeting and one that comes back on the next launch.
        val remaining = compose.awaitServer("the conference to be gone server-side") {
            val ids = E2eBackend.conferences(fixture).map { it.id }
            ids.takeIf { doomed.id !in it }
        }
        assertThat(remaining).contains(kept.id)
    }
}
