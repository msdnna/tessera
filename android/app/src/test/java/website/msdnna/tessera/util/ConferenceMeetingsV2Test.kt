package website.msdnna.tessera.util

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import website.msdnna.tessera.data.model.Conference

/** Meetings v2 (#3034): the plan, reactions, and the chat's «/» and «@» — the web's rules. */
class ConferenceMeetingsV2Test {
    @Test
    fun planDropsRepeatWithoutStartAndSendsZoneOnlyWithRepeat() {
        assertEquals(
            ConferencePlan(30, "weekdays", "Europe/Moscow", "m1"),
            conferencePlan("2026-10-12T08:00:00Z", 30, "weekdays", "m1", zone = "Europe/Moscow"),
        )
        assertEquals(ConferencePlan(null, "", "", null), conferencePlan(null, null, "daily", "", zone = "UTC"))
        assertEquals("", conferencePlan("2026-10-12T08:00:00Z", 15, "yearly", null).recurrence)
    }

    @Test
    fun rowIsFiledByNextOccurrence() {
        val c = Conference(status = "scheduled", scheduledAt = "2026-09-01T10:00:00Z", nextAt = "2026-10-12T10:00:00Z")
        assertEquals("2026-10-12T10:00:00Z", conferenceTimeLine(c).iso)
        assertEquals(1 to 30, conferenceDurationParts(90))
        assertNull(conferenceDurationParts(null))
    }

    @Test
    fun reactionsRoundTripOnlyThePalette() {
        CONF_REACTIONS.forEach { assertEquals(it, decodeReaction(encodeReaction(it))) }
        assertNull(encodeReaction("x"))
        assertNull(decodeReaction("""{"t":"reaction","e":"<b>"}""".toByteArray()))
        assertNull(decodeReaction("""{"t":"chat","e":"👍"}""".toByteArray()))
        assertNull(decodeReaction("nope".toByteArray()))
        assertFalse(canReact(listOf(1000L, 1100L, 1200L, 1300L), 1500L))
        assertTrue(canReact(listOf(1000L, 1100L, 1200L, 1300L), 5000L))
        var feed = emptyList<ConfReaction>()
        repeat(CONF_REACTION_MAX + 3) { feed = addReaction(feed, ConfReaction(it.toLong(), "a", "", "👍", 0L, 0)) }
        assertEquals(CONF_REACTION_MAX, feed.size)
        assertTrue(pruneReactions(feed, CONF_REACTION_TTL_MS).isEmpty())
    }

    private val members = listOf(
        ChatMember("u-anna", "Анна", "anna@x.io"),
        ChatMember("u-annak", "Анна Ким", "kim@x.io"),
        ChatMember("me", "Мария", "me@x.io"),
    )

    @Test
    fun taskCommandInEitherLanguage() {
        assertEquals("", chatSlashQuery("/"))
        assertNull(chatSlashQuery("и/или"))
        assertTrue(chatCommandMatches("за"))
        assertFalse(chatCommandMatches("zz"))
        assertEquals("Прогноз на burn-up", chatTaskCommand("/задача  Прогноз   на burn-up "))
        assertEquals("Fix", chatTaskCommand("/task Fix"))
        assertEquals("", chatTaskCommand("/задача"))
        assertNull(chatTaskCommand("/shrug x"))
    }

    @Test
    fun mentionsMatchChipAndCount() {
        assertEquals(7 to "Ан", chatMentionQuery("привет @Ан"))
        assertNull(chatMentionQuery("mail@x"))
        assertEquals(listOf("u-annak"), chatMentionMatches(members, "ки", me = "me").map { it.userId })
        assertFalse(chatMentionMatches(members, "", me = "me").any { it.userId == "me" })
        val parts = chatSplitMentions("@Анна Ким и @Анна, mail@Анна", members)
        assertEquals(listOf("u-annak", "u-anna"), parts.mapNotNull { it.member?.userId })
        assertEquals("@Анна Ким и @Анна, mail@Анна", parts.joinToString("") { it.text })
        assertEquals(listOf("u-annak"), chatMentionIds("@Анна Ким @Анна Ким", members))
        assertEquals("b", chatDefaultBoard(listOf("a", "b"), "b"))
        assertEquals("a", chatDefaultBoard(listOf("a", "b"), "zz"))
    }
}
