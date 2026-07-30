package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import java.util.Calendar
import org.junit.Test
import website.msdnna.tessera.data.model.Recurrence

class RecurrenceTest {
    /** Local-zone midnight millis for the given calendar date (matches Recurrence's own zone). */
    private fun localMidnight(y: Int, mo1: Int, d: Int): Long =
        Calendar.getInstance().apply {
            clear()
            set(y, mo1 - 1, d, 0, 0, 0)
        }.timeInMillis

    private fun keyOf(millis: Long?) = millis?.let { millisDayKey(it) }

    // ── null / blank rule ────────────────────────────────────────────────────
    @Test
    fun `nextOccurrence null or blank freq is null`() {
        assertThat(nextOccurrence(null, localMidnight(2026, 6, 1))).isNull()
        assertThat(nextOccurrence(Recurrence(freq = ""), localMidnight(2026, 6, 1))).isNull()
        assertThat(nextOccurrence(Recurrence(freq = "bogus"), localMidnight(2026, 6, 1))).isNull()
    }

    // ── daily ────────────────────────────────────────────────────────────────
    @Test
    fun `daily advances by interval`() {
        val from = localMidnight(2026, 6, 10) // Wed
        assertThat(keyOf(nextOccurrence(Recurrence(freq = "daily", interval = 1), from)))
            .isEqualTo("2026-06-11")
        assertThat(keyOf(nextOccurrence(Recurrence(freq = "daily", interval = 3), from)))
            .isEqualTo("2026-06-13")
    }

    @Test
    fun `daily interval floored to 1`() {
        val from = localMidnight(2026, 6, 10)
        assertThat(keyOf(nextOccurrence(Recurrence(freq = "daily", interval = 0), from)))
            .isEqualTo("2026-06-11")
    }

    @Test
    fun `daily with skip weekends jumps over sat and sun`() {
        // 2026-06-12 is Friday → +1 = Saturday → skip to Monday 15th
        val fri = localMidnight(2026, 6, 12)
        assertThat(keyOf(nextOccurrence(Recurrence(freq = "daily", skipWeekends = true), fri)))
            .isEqualTo("2026-06-15")
    }

    // ── weekly ───────────────────────────────────────────────────────────────
    @Test
    fun `weekly without weekdays advances 7 days times interval`() {
        val from = localMidnight(2026, 6, 10)
        assertThat(keyOf(nextOccurrence(Recurrence(freq = "weekly", interval = 1), from)))
            .isEqualTo("2026-06-17")
        assertThat(keyOf(nextOccurrence(Recurrence(freq = "weekly", interval = 2), from)))
            .isEqualTo("2026-06-24")
    }

    @Test
    fun `weekly picks next selected weekday within the same week`() {
        // Wed 2026-06-10; select Mon(1),Fri(5). Next selected later this week = Fri 12th.
        val from = localMidnight(2026, 6, 10)
        val rule = Recurrence(freq = "weekly", weekdays = listOf(1, 5))
        assertThat(keyOf(nextOccurrence(rule, from))).isEqualTo("2026-06-12")
    }

    @Test
    fun `weekly wraps to next week when no later weekday this week`() {
        // Fri 2026-06-12; select only Mon(1). Next = Mon 15th.
        val from = localMidnight(2026, 6, 12)
        val rule = Recurrence(freq = "weekly", weekdays = listOf(1))
        assertThat(keyOf(nextOccurrence(rule, from))).isEqualTo("2026-06-15")
    }

    // ── monthly ──────────────────────────────────────────────────────────────
    @Test
    fun `monthly advances by interval months`() {
        val from = localMidnight(2026, 6, 15)
        assertThat(keyOf(nextOccurrence(Recurrence(freq = "monthly", interval = 1), from)))
            .isEqualTo("2026-07-15")
        assertThat(keyOf(nextOccurrence(Recurrence(freq = "monthly", interval = 8), from)))
            .isEqualTo("2027-02-15")
    }

    @Test
    fun `monthly clamps day to month length`() {
        // Jan 31 + 1 month → Feb 28 (2026 not leap)
        val from = localMidnight(2026, 1, 31)
        assertThat(keyOf(nextOccurrence(Recurrence(freq = "monthly", interval = 1), from)))
            .isEqualTo("2026-02-28")
    }

    // ── yearly ───────────────────────────────────────────────────────────────
    @Test
    fun `yearly advances by interval years`() {
        val from = localMidnight(2026, 6, 15)
        assertThat(keyOf(nextOccurrence(Recurrence(freq = "yearly", interval = 1), from)))
            .isEqualTo("2027-06-15")
    }

    @Test
    fun `yearly leap day clamps to feb 28 in non-leap year`() {
        // 2028 is leap → Feb 29; +1 year → 2029 non-leap → Feb 28
        val from = localMidnight(2028, 2, 29)
        assertThat(keyOf(nextOccurrence(Recurrence(freq = "yearly", interval = 1), from)))
            .isEqualTo("2029-02-28")
    }

    // ── custom ───────────────────────────────────────────────────────────────
    @Test
    fun `custom picks first date strictly after from`() {
        val from = localMidnight(2026, 6, 10)
        val rule = Recurrence(freq = "custom", dates = listOf("2026-06-10", "2026-06-20", "2026-07-01"))
        assertThat(keyOf(nextOccurrence(rule, from))).isEqualTo("2026-06-20")
    }

    @Test
    fun `custom returns null when no later date`() {
        val from = localMidnight(2026, 8, 1)
        val rule = Recurrence(freq = "custom", dates = listOf("2026-06-20"))
        assertThat(nextOccurrence(rule, from)).isNull()
    }

    // ── occurrenceKeys ───────────────────────────────────────────────────────
    @Test
    fun `occurrenceKeys null inputs empty`() {
        assertThat(occurrenceKeys(null, 123L)).isEmpty()
        assertThat(occurrenceKeys(Recurrence(freq = "daily"), null)).isEmpty()
    }

    @Test
    fun `occurrenceKeys custom returns its dates`() {
        val rule = Recurrence(freq = "custom", dates = listOf("2026-06-10", "2026-06-20"))
        assertThat(occurrenceKeys(rule, localMidnight(2026, 6, 1)))
            .containsExactly("2026-06-10", "2026-06-20")
    }

    @Test
    fun `occurrenceKeys once yields current plus one`() {
        val from = localMidnight(2026, 6, 10)
        val rule = Recurrence(freq = "daily", once = true)
        assertThat(occurrenceKeys(rule, from)).containsExactly("2026-06-10", "2026-06-11")
    }

    @Test
    fun `occurrenceKeys generates n occurrences`() {
        val from = localMidnight(2026, 6, 10)
        val rule = Recurrence(freq = "daily")
        val keys = occurrenceKeys(rule, from, n = 3)
        assertThat(keys).containsExactly("2026-06-10", "2026-06-11", "2026-06-12", "2026-06-13")
    }
}
