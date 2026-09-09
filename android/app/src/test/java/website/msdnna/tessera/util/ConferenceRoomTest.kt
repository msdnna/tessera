package website.msdnna.tessera.util

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The room's layout and banner rules (#2896 §5).
 *
 * Deliberately built on a local [Person] rather than on `ConfPeer`: the layout
 * is supposed to work without the SDK, and a test that had to construct a real
 * peer would be proving the opposite.
 */
class ConferenceRoomTest {
    private data class Person(
        override val identity: String,
        override val local: Boolean = false,
        override val speaking: Boolean = false,
        override val sharingScreen: Boolean = false,
    ) : ConfStagePerson

    // ── who gets the big tile ────────────────────────────────────────────

    @Test
    fun `speaker takes the stage`() {
        val me = Person("me", local = true)
        val ann = Person("ann")
        val bob = Person("bob", speaking = true)

        val layout = confStage(listOf(me, ann, bob))

        assertEquals(bob, layout.stage)
        assertFalse(layout.screen)
        assertEquals(listOf(me, ann), layout.strip)
    }

    @Test
    fun `stage holds the last speaker through a silence`() {
        val peers = listOf(Person("me", local = true), Person("ann"), Person("bob"))

        // Nobody is speaking now, but bob was: without the memory this falls
        // through to «first remote» and the tile jumps to ann every time the
        // room goes quiet.
        assertEquals("bob", confStage(peers, lastSpeaker = "bob").stage?.identity)
    }

    @Test
    fun `a stage remembered for somebody who left falls back to a remote`() {
        val peers = listOf(Person("me", local = true), Person("ann"))

        assertEquals("ann", confStage(peers, lastSpeaker = "bob").stage?.identity)
    }

    @Test
    fun `before the first word a remote face beats our own`() {
        val peers = listOf(Person("me", local = true), Person("ann"))

        assertEquals("ann", confStage(peers).stage?.identity)
    }

    @Test
    fun `alone in the call we get the stage`() {
        assertEquals("me", confStage(listOf(Person("me", local = true))).stage?.identity)
    }

    @Test
    fun `an empty call has no stage and no strip`() {
        val layout = confStage(emptyList<Person>())

        assertEquals(null, layout.stage)
        assertTrue(layout.strip.isEmpty())
    }

    @Test
    fun `a shared screen outranks the speaker and keeps the presenter in the strip`() {
        val me = Person("me", local = true)
        val ann = Person("ann", sharingScreen = true)
        val bob = Person("bob", speaking = true)

        val layout = confStage(listOf(me, ann, bob), lastSpeaker = "bob")

        assertEquals(ann, layout.stage)
        assertTrue(layout.screen)
        // The presenter's own face stays below their screen — it is still worth
        // seeing, and dropping it would leave the room watching an anonymous IDE.
        assertEquals(listOf(me, ann, bob), layout.strip)
    }

    // ── the remembered speaker ───────────────────────────────────────────

    @Test
    fun `whoever is speaking becomes the remembered speaker`() {
        val peers = listOf(Person("ann"), Person("bob", speaking = true))

        assertEquals("bob", confLastSpeaker(peers, previous = "ann"))
    }

    @Test
    fun `silence keeps the previous speaker`() {
        val peers = listOf(Person("ann"), Person("bob"))

        assertEquals("bob", confLastSpeaker(peers, previous = "bob"))
    }

    @Test
    fun `a speaker who left is forgotten`() {
        assertEquals("", confLastSpeaker(listOf(Person("ann")), previous = "bob"))
    }

    // ── what the screen says ─────────────────────────────────────────────

    @Test
    fun `an install without an SFU says so instead of failing`() {
        val banner = confBanner(
            ConfMediaStatus.UNAVAILABLE,
            ConfJoinReason.NOT_CONFIGURED,
            reconnecting = false,
            micGranted = true,
        )

        assertEquals(ConfBanner.UNAVAILABLE, banner)
        // Pressing «Повторить» cannot configure LiveKit on the server.
        assertFalse(confCanRetry(banner))
    }

    @Test
    fun `an ended call is not an error`() {
        assertEquals(
            ConfBanner.ENDED,
            confBanner(ConfMediaStatus.IDLE, ConfJoinReason.ENDED, reconnecting = false, micGranted = true),
        )
    }

    @Test
    fun `a kick is final, a plain failure is not`() {
        assertFalse(
            confCanRetry(
                confBanner(ConfMediaStatus.ERROR, ConfJoinReason.KICKED, reconnecting = false, micGranted = true),
            ),
        )
        assertTrue(
            confCanRetry(
                confBanner(ConfMediaStatus.ERROR, ConfJoinReason.NONE, reconnecting = false, micGranted = true),
            ),
        )
    }

    @Test
    fun `reconnecting outranks connecting but not a refusal`() {
        assertEquals(
            ConfBanner.RECONNECTING,
            confBanner(ConfMediaStatus.CONNECTING, ConfJoinReason.NONE, reconnecting = true, micGranted = true),
        )
        assertEquals(
            ConfBanner.ERROR,
            confBanner(ConfMediaStatus.ERROR, ConfJoinReason.NONE, reconnecting = true, micGranted = true),
        )
    }

    @Test
    fun `a live call without the microphone permission says nobody can hear us`() {
        assertEquals(
            ConfBanner.NO_MIC,
            confBanner(ConfMediaStatus.LIVE, ConfJoinReason.NONE, reconnecting = false, micGranted = false),
        )
    }

    @Test
    fun `a denied microphone is not announced before the call connects`() {
        // The join is still in flight: «вас не слышат» over a room nobody is in
        // yet reads as the connection having failed.
        assertEquals(
            ConfBanner.CONNECTING,
            confBanner(ConfMediaStatus.CONNECTING, ConfJoinReason.NONE, reconnecting = false, micGranted = false),
        )
    }

    @Test
    fun `a healthy live call says nothing`() {
        assertEquals(
            ConfBanner.NONE,
            confBanner(ConfMediaStatus.LIVE, ConfJoinReason.NONE, reconnecting = false, micGranted = true),
        )
    }

    // ── the toolbar ──────────────────────────────────────────────────────

    @Test
    fun `a force-muted microphone is disabled rather than merely off`() {
        val controls = confControls(ConfMediaStatus.LIVE, micOn = false, camOn = false, forceMuted = true)

        assertFalse(controls.micEnabled)
        // The camera is nobody else's business — a host silences voices, not faces.
        assertTrue(controls.camEnabled)
    }

    @Test
    fun `nothing but hanging up works before the call is live`() {
        val controls = confControls(ConfMediaStatus.CONNECTING, micOn = true, camOn = true, forceMuted = false)

        assertFalse(controls.micEnabled)
        assertFalse(controls.camEnabled)
        assertFalse(controls.routeEnabled)
    }

    @Test
    fun `front-back only means something with a camera published`() {
        assertFalse(
            confControls(ConfMediaStatus.LIVE, micOn = true, camOn = false, forceMuted = false).switchCamEnabled,
        )
        assertTrue(
            confControls(ConfMediaStatus.LIVE, micOn = true, camOn = true, forceMuted = false).switchCamEnabled,
        )
    }

    // ── the signal badge ─────────────────────────────────────────────────

    @Test
    fun `only a struggling link is badged`() {
        assertTrue(ConfQuality.POOR.weak)
        assertTrue(ConfQuality.LOST.weak)
        assertFalse(ConfQuality.EXCELLENT.weak)
        assertFalse(ConfQuality.GOOD.weak)
        // A link the SFU has not rated yet is not a bad one.
        assertFalse(ConfQuality.UNKNOWN.weak)
    }
}
