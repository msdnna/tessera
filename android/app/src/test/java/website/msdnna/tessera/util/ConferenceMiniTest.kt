package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import org.junit.Test

/**
 * The rules of a call nobody is looking at (#2896 §9).
 *
 * These are the ones that fail silently by construction: the bar is the only
 * thing on screen that knows a meeting is still running, so a rule that hides it
 * too eagerly does not produce a wrong pixel — it produces a microphone that is
 * open with nothing anywhere saying so.
 */
class ConferenceMiniTest {
    private class Person(
        override val identity: String,
        override val local: Boolean = false,
        override val speaking: Boolean = false,
        override val sharingScreen: Boolean = false,
    ) : ConfStagePerson

    // ── when there is a bar at all ────────────────────────────────────────

    @Test
    fun `a live call off-screen gets a bar`() {
        assertThat(confMiniLine(ConfMediaStatus.LIVE, reconnecting = false, roomOnScreen = false))
            .isEqualTo(ConfMiniLine.LIVE)
    }

    @Test
    fun `the room being on screen takes the bar away`() {
        // The one that would be a visible bug: a bar drawn over the very call it
        // is a shortcut to, covering the toolbar with a smaller copy of itself.
        assertThat(confMiniLine(ConfMediaStatus.LIVE, reconnecting = false, roomOnScreen = true)).isNull()
    }

    @Test
    fun `connecting counts as a call worth minimising`() {
        // Someone taps «Войти» and immediately switches app. The connection is
        // still being made and the service is already up (`needsCallService`),
        // so a bar that waited for LIVE would leave a call running unannounced.
        assertThat(confMiniLine(ConfMediaStatus.CONNECTING, reconnecting = false, roomOnScreen = false))
            .isEqualTo(ConfMiniLine.CONNECTING)
    }

    @Test
    fun `an idle session has no bar`() {
        assertThat(confMiniLine(ConfMediaStatus.IDLE, reconnecting = false, roomOnScreen = false)).isNull()
    }

    @Test
    fun `an install without an SFU has no bar`() {
        // Nothing to go back to: UNAVAILABLE is a fact about the server, and a
        // bar promising a call would lead to a room that cannot exist here.
        assertThat(confMiniLine(ConfMediaStatus.UNAVAILABLE, reconnecting = false, roomOnScreen = false)).isNull()
    }

    // ── what it says ──────────────────────────────────────────────────────

    @Test
    fun `reconnecting outranks live`() {
        // The session stays LIVE across a reconnect, so a bar reading the status
        // alone would keep saying «идёт звонок» through a silence the user is
        // hearing. This is the line that has to move first.
        assertThat(confMiniLine(ConfMediaStatus.LIVE, reconnecting = true, roomOnScreen = false))
            .isEqualTo(ConfMiniLine.RECONNECTING)
    }

    @Test
    fun `a reconnect flag left on an idle session raises nothing`() {
        // `leave()` resets the session but a stale flag arriving a frame later
        // must not resurrect a bar for a call that has been hung up.
        assertThat(confMiniLine(ConfMediaStatus.IDLE, reconnecting = true, roomOnScreen = false)).isNull()
    }

    @Test
    fun `a failed connection keeps the bar and says so`() {
        // The bar does not vanish on failure: disappearing would end the call
        // from the user's side without a word, and «Повторить» lives in the room
        // this bar is the way back to.
        assertThat(confMiniLine(ConfMediaStatus.ERROR, reconnecting = false, roomOnScreen = false))
            .isEqualTo(ConfMiniLine.LOST)
    }

    @Test
    fun `every line is worth returning to`() {
        ConfMiniLine.entries.forEach { assertThat(confMiniReturnable(it)).isTrue() }
        assertThat(confMiniReturnable(null)).isFalse()
    }

    // ── the count ─────────────────────────────────────────────────────────

    @Test
    fun `the count leaves us out of it`() {
        val peers = listOf(Person("me", local = true), Person("a"), Person("b"))
        assertThat(confMiniOthers(peers)).isEqualTo(2)
    }

    @Test
    fun `alone in the call is zero, not one`() {
        assertThat(confMiniOthers(listOf(Person("me", local = true)))).isEqualTo(0)
    }

    // ── the microphone ────────────────────────────────────────────────────

    @Test
    fun `the bar's microphone answers exactly as the toolbar's does`() {
        // Two surfaces onto one microphone. Asserted against `confControls`
        // rather than against a hand-written expectation, so the day the rule
        // moves it moves for both — the §6 lesson about a second copy of a rule.
        val cases = listOf(
            ConfMediaStatus.LIVE to false,
            ConfMediaStatus.LIVE to true,
            ConfMediaStatus.CONNECTING to false,
            ConfMediaStatus.ERROR to false,
            ConfMediaStatus.IDLE to false,
        )
        cases.forEach { (status, forced) ->
            assertThat(confMiniMicEnabled(status, forced))
                .isEqualTo(confControls(status, micOn = true, camOn = false, forceMuted = forced).micEnabled)
        }
    }

    @Test
    fun `a host's mute reaches the bar too`() {
        assertThat(confMiniMicEnabled(ConfMediaStatus.LIVE, forceMuted = false)).isTrue()
        assertThat(confMiniMicEnabled(ConfMediaStatus.LIVE, forceMuted = true)).isFalse()
    }

    @Test
    fun `a call still connecting has nothing to mute`() {
        assertThat(confMiniMicEnabled(ConfMediaStatus.CONNECTING, forceMuted = false)).isFalse()
    }
}
