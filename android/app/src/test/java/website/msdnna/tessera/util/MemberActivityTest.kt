package website.msdnna.tessera.util

import java.time.Instant
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/** Активность участника (#3073) — те же корзины, что и на вебе. */
class MemberActivityTest {
    private val now = Instant.parse("2026-10-10T12:00:00Z").toEpochMilli()
    private fun ago(ms: Long) = Instant.ofEpochMilli(now - ms).toString()

    @Test
    fun takesTheLaterStampAndMarksOnline() {
        val a = memberActivity(ago(3_600_000), ago(60_000), now)
        assertEquals(ActivityBucket.NOW, a.bucket)
        assertTrue(a.online)
    }

    @Test
    fun bucketsTheDistance() {
        assertEquals(ActivityBucket.NEVER, memberActivity(null, null, now).bucket)
        assertEquals(MemberActivity(ActivityBucket.MIN, 12, true), memberActivity(ago(12 * 60_000), null, now))
        val h = memberActivity(ago(2 * 3_600_000), null, now)
        assertEquals(ActivityBucket.HOURS, h.bucket)
        assertFalse(h.online)
        assertEquals(ActivityBucket.YESTERDAY, memberActivity(ago(30 * 3_600_000), null, now).bucket)
        assertEquals(ActivityBucket.DAYS, memberActivity(null, ago(3 * 86_400_000), now).bucket)
    }
}
