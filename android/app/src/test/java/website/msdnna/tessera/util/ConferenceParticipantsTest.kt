package website.msdnna.tessera.util

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import website.msdnna.tessera.data.conference.ConfDenied
import website.msdnna.tessera.data.conference.ConfPerson
import website.msdnna.tessera.data.conference.ROLE_HOST
import website.msdnna.tessera.data.conference.ROLE_MEMBER

/** The participants panel's rules (#2896 §6). */
class ConferenceParticipantsTest {
    private fun person(
        id: String,
        name: String = id,
        role: String = ROLE_MEMBER,
        mic: Boolean = true,
        forceMuted: Boolean = false,
        handAt: String? = null,
    ) = ConfPerson(userId = id, name = name, role = role, mic = mic, forceMuted = forceMuted, handAt = handAt)

    // ── ordering ──────────────────────────────────────────────────────────

    @Test
    fun `raised hands come first`() {
        val order = confPanelOrder(
            listOf(person("a"), person("b", handAt = "2026-09-05T10:00:00Z"), person("c")),
        )
        assertEquals(listOf("b", "a", "c"), order.map { it.userId })
    }

    @Test
    fun `hands keep the order they went up`() {
        val order = confPanelOrder(
            listOf(
                person("late", handAt = "2026-09-05T10:05:00Z"),
                person("early", handAt = "2026-09-05T10:01:00Z"),
                person("plain"),
            ),
        )
        assertEquals(listOf("early", "late", "plain"), order.map { it.userId })
    }

    /** The rest keep the room's own order — a re-sort would shuffle faces under
     *  a finger reaching for a kick button. */
    @Test
    fun `everyone without a hand keeps join order`() {
        val order = confPanelOrder(listOf(person("c"), person("a"), person("b")))
        assertEquals(listOf("c", "a", "b"), order.map { it.userId })
    }

    /** The badge counts the hands, not the heads: «5» on every call is a badge
     *  nobody looks at. */
    @Test
    fun `the badge counts only raised hands`() {
        assertEquals(
            2,
            confHandCount(
                listOf(
                    person("second", handAt = "2026-09-05T10:02:00Z"),
                    person("none"),
                    person("first", handAt = "2026-09-05T10:01:00Z"),
                ),
            ),
        )
        assertEquals(0, confHandCount(listOf(person("none"))))
    }

    // ── badges ────────────────────────────────────────────────────────────

    /** Two different silences, and the panel has to say which. */
    @Test
    fun `a force-mute outranks an ordinary muted microphone`() {
        assertEquals(ConfMicBadge.FORCE_MUTED, confMicBadge(person("a", mic = false, forceMuted = true)))
        assertEquals(ConfMicBadge.FORCE_MUTED, confMicBadge(person("b", mic = true, forceMuted = true)))
        assertEquals(ConfMicBadge.OFF, confMicBadge(person("c", mic = false)))
        assertEquals(ConfMicBadge.ON, confMicBadge(person("d", mic = true)))
    }

    // ── moderation ────────────────────────────────────────────────────────

    @Test
    fun `a plain member gets no moderation controls`() {
        val actions = confRowActions(person("them"), meId = "me", canModerate = false)
        assertFalse(actions.forceMute)
        assertFalse(actions.kick)
    }

    @Test
    fun `a moderator gets both on somebody else`() {
        val actions = confRowActions(person("them"), meId = "me", canModerate = true)
        assertTrue(actions.forceMute)
        assertTrue(actions.kick)
    }

    /** The server answers «cannot kick yourself» — and a force-mute we could
     *  lift ourselves is not a moderation decision at all. */
    @Test
    fun `never on our own row`() {
        val actions = confRowActions(person("me"), meId = "me", canModerate = true)
        assertFalse(actions.forceMute)
        assertFalse(actions.kick)
    }

    /**
     * A host may be silenced and may not be removed — the same split the server
     * makes. Showing the kick anyway puts a control on screen whose every press
     * answers «cannot kick a host».
     */
    @Test
    fun `a host can be muted but not kicked`() {
        val actions = confRowActions(person("boss", role = ROLE_HOST), meId = "me", canModerate = true)
        assertTrue(actions.forceMute)
        assertFalse(actions.kick)
    }

    /** Before the welcome lands we do not know who we are, and every row would
     *  look like somebody else's. */
    @Test
    fun `nothing is offered before we know our own id`() {
        val actions = confRowActions(person("them"), meId = "", canModerate = true)
        assertFalse(actions.forceMute)
        assertFalse(actions.kick)
    }

    // ── local audio ───────────────────────────────────────────────────────

    @Test
    fun `local controls need a media descriptor`() {
        assertTrue(confHasLocalAudio("them", meId = "me", mediaIdentities = listOf("them", "me")))
        // In the room socket but not publishing to the SFU yet — there is no
        // audio to turn down, so a slider would move nothing.
        assertFalse(confHasLocalAudio("joining", meId = "me", mediaIdentities = listOf("them")))
        assertFalse(confHasLocalAudio("me", meId = "me", mediaIdentities = listOf("me")))
    }

    @Test
    fun `volume is clamped to what the SDK accepts`() {
        assertEquals(0f, confClampVolume(-1f), 0.001f)
        assertEquals(CONF_VOLUME_MAX, confClampVolume(9f), 0.001f)
        assertEquals(1.4f, confClampVolume(1.4f), 0.001f)
    }

    // ── refusals ──────────────────────────────────────────────────────────

    /** Keyed on the action, not the server's sentence: the reason is English
     *  prose for a developer and would be a log line in front of a user. */
    @Test
    fun `a refusal is read by the command it belonged to`() {
        assertEquals(ConfDeniedKind.NONE, confDeniedKind(null))
        assertEquals(ConfDeniedKind.KICK, confDeniedKind(ConfDenied("kick", "not a moderator")))
        assertEquals(ConfDeniedKind.MUTE, confDeniedKind(ConfDenied("mute", "not a moderator")))
        assertEquals(ConfDeniedKind.OTHER, confDeniedKind(ConfDenied("screen.request", "taken")))
    }
}
