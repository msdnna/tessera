package website.msdnna.tessera.util

import org.junit.Assert.assertEquals
import org.junit.Test
import website.msdnna.tessera.data.model.Member

/** Tags and mentions of a «Входящие» record as the notes / reminders rows show them (#3040). */
class RecordLabelsTest {
    @Test
    fun `tags read as one hash line, a missing list as nothing`() {
        assertEquals("#релиз #frontend", recordTagLine(listOf("релиз", " frontend ", "")))
        assertEquals("", recordTagLine(null))
        assertEquals("", recordTagLine(emptyList()))
    }

    @Test
    fun `mentions resolve to first names, a member who left drops out`() {
        val members = listOf(
            Member(userId = "u1", name = "Анна Ким"),
            Member(userId = "u2", name = "", email = "ilya@x.dev"),
        )
        assertEquals(listOf("Анна", "ilya@x.dev"), recordMentionNames(listOf("u1", "gone", "u2", "u1"), members))
        assertEquals(emptyList<String>(), recordMentionNames(null, members))
    }
}
