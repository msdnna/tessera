package website.msdnna.tessera.data.conference

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The room socket's frame handling (#2896 §6).
 *
 * Frames are fed in directly: the transport needs a server, and none of the
 * decisions below do. What is checked here is the contract with
 * `internal/confroom` — a wrong `@SerializedName` costs nothing at compile time
 * and shows up as a call where nobody has a microphone.
 */
class ConferenceRoomSocketTest {
    private fun socket() = ConferenceRoomSocket()

    private val welcome = """
        {"type":"welcome","conn_id":"c1","user_id":"u-me","role":"member","can_moderate":true,
         "conference_id":"conf-1","stage_ttl_ms":30000}
    """.trimIndent()

    private fun state(vararg people: String) =
        """{"type":"state","participants":[${people.joinToString(",")}],"stage":null,"queue":[],"recording":null}"""

    private fun person(
        id: String,
        role: String = "member",
        mic: Boolean = true,
        forceMuted: Boolean = false,
        handAt: String? = null,
    ): String {
        val hand = handAt?.let { "\"$it\"" } ?: "null"
        return """{"user_id":"$id","name":"$id","role":"$role","conns":1,"mic":$mic,"cam":false,
                   "force_muted":$forceMuted,"hand_at":$hand,"joined_at":"2026-09-05T10:00:00Z"}"""
    }

    @Test
    fun `welcome carries who we are and what we may do`() {
        val s = socket()
        s.receiveForTest(welcome)
        val st = s.state.value
        assertEquals("c1", st.connId)
        assertEquals("u-me", st.meId)
        // Moderation is not the host label: a workspace admin sits in the call
        // as a plain member and still holds the controls (#2878).
        assertEquals(ROLE_MEMBER, st.role)
        assertTrue(st.canModerate)
    }

    @Test
    fun `a snapshot replaces the roster rather than merging into it`() {
        val s = socket()
        s.receiveForTest(state(person("a"), person("b")))
        assertEquals(listOf("a", "b"), s.state.value.people.map { it.userId })
        // Somebody who left must not survive the next frame — that is the whole
        // reason the server sends the room whole instead of deltas.
        s.receiveForTest(state(person("a")))
        assertEquals(listOf("a"), s.state.value.people.map { it.userId })
    }

    @Test
    fun `the wire names of a participant are read`() {
        val s = socket()
        s.receiveForTest(state(person("a", role = "host", mic = false, forceMuted = true, handAt = "2026-09-05T10:01:00Z")))
        val p = s.state.value.people.single()
        assertTrue(p.host)
        assertFalse(p.mic)
        assertTrue(p.forceMuted)
        assertEquals("2026-09-05T10:01:00Z", p.handAt)
    }

    /** Our own force-mute is read off the roster, not off a reply to whoever
     *  pressed the button — the engine refuses the microphone on it. */
    @Test
    fun `our own force-mute is found by identity`() {
        val s = socket()
        s.receiveForTest(welcome)
        s.receiveForTest(state(person("u-other", forceMuted = true)))
        assertFalse(s.state.value.forceMuted)
        s.receiveForTest(state(person("u-me", forceMuted = true), person("u-other")))
        assertTrue(s.state.value.forceMuted)
    }

    @Test
    fun `our own hand is read off the roster`() {
        val s = socket()
        s.receiveForTest(welcome)
        s.receiveForTest(state(person("u-me")))
        assertFalse(s.state.value.handUp)
        s.receiveForTest(state(person("u-me", handAt = "2026-09-05T10:01:00Z")))
        assertTrue(s.state.value.handUp)
    }

    @Test
    fun `a refusal names the command it belonged to`() {
        val s = socket()
        s.receiveForTest("""{"type":"denied","action":"kick","reason":"not a moderator"}""")
        assertEquals("kick", s.state.value.denied?.action)
        s.clearDenied()
        assertNull(s.state.value.denied)
    }

    /** Messages are fetched over HTTP, so the nudge is payload-free: a frame
     *  lost to a reconnect costs a stale panel, not a phantom message (§7). */
    @Test
    fun `a chat frame only bumps the counter`() {
        val s = socket()
        s.receiveForTest("""{"type":"chat"}""")
        s.receiveForTest("""{"type":"chat"}""")
        assertEquals(2, s.state.value.chatNudge)
    }

    @Test
    fun `the end reason is kept apart`() {
        val kicked = socket()
        kicked.receiveForTest("""{"type":"ended","reason":"kicked"}""")
        assertEquals(ConfRoomEnd.KICKED, kicked.state.value.ended)

        val deleted = socket()
        deleted.receiveForTest("""{"type":"ended","reason":"deleted"}""")
        assertEquals(ConfRoomEnd.DELETED, deleted.state.value.ended)

        // An unknown reason is still an end: staying in a room the server has
        // said goodbye to is the one thing this must not do.
        val other = socket()
        other.receiveForTest("""{"type":"ended","reason":"who-knows"}""")
        assertEquals(ConfRoomEnd.ENDED, other.state.value.ended)
    }

    /** A room we were thrown out of answers 403 on the handshake; retrying it
     *  would loop until the screen goes away. */
    @Test
    fun `an ended room stops being connected`() {
        val s = socket()
        s.receiveForTest(welcome)
        s.receiveForTest(state(person("u-me")))
        s.receiveForTest("""{"type":"ended","reason":"ended"}""")
        assertFalse(s.state.value.connected)
    }

    @Test
    fun `a malformed frame is ignored rather than fatal`() {
        val s = socket()
        s.receiveForTest(state(person("a")))
        s.receiveForTest("not json at all")
        s.receiveForTest("""{"type":"unheard-of"}""")
        assertEquals(listOf("a"), s.state.value.people.map { it.userId })
    }

    /** The stage and the recording ride in the same snapshot (§8 uses them);
     *  parsing them here is what keeps there being one parser. */
    @Test
    fun `the stage and the recording are parsed out of the snapshot`() {
        val s = socket()
        s.receiveForTest(
            """{"type":"state","participants":[],
                "stage":{"user_id":"u-1","conn_id":"c9","name":"Ann"},
                "queue":[{"user_id":"u-2","conn_id":"c8","name":"Bob","role":"member"}],
                "recording":{"id":"r1","started_at":"2026-09-05T10:00:00Z","started_by":"Ann"}}""",
        )
        val st = s.state.value
        assertEquals("c9", st.stage?.connId)
        assertEquals(listOf("u-2"), st.queue.map { it.userId })
        assertEquals("Ann", st.recording?.startedBy)
    }
}
