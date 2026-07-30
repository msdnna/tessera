package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import java.util.Calendar
import org.junit.Test

class DatesTest {
    private fun currentYear() = Calendar.getInstance().get(Calendar.YEAR).toString()

    // ── shortDate ────────────────────────────────────────────────────────────
    @Test
    fun `shortDate blank or too-short is empty`() {
        assertThat(shortDate(null)).isEmpty()
        assertThat(shortDate("")).isEmpty()
        assertThat(shortDate("2026-06")).isEmpty()
    }

    @Test
    fun `shortDate strips leading zero on day and maps month`() {
        val yr = currentYear()
        assertThat(shortDate("$yr-06-05T00:00:00Z")).isEqualTo("5 июн")
        assertThat(shortDate("$yr-12-25")).isEqualTo("25 дек")
        assertThat(shortDate("$yr-01-01")).isEqualTo("1 янв")
    }

    @Test
    fun `shortDate appends year when not current`() {
        assertThat(shortDate("2020-03-10")).isEqualTo("10 мар 2020")
    }

    @Test
    fun `shortDate invalid month yields empty`() {
        assertThat(shortDate("2026-13-10")).isEmpty()
    }

    // ── longDate ─────────────────────────────────────────────────────────────
    @Test
    fun `longDate formats with year and g suffix`() {
        assertThat(longDate("2026-06-04T00:00:00Z")).isEqualTo("4 июн. 2026 г.")
        assertThat(longDate("2020-01-31")).isEqualTo("31 янв. 2020 г.")
    }

    @Test
    fun `longDate blank empty`() {
        assertThat(longDate(null)).isEmpty()
        assertThat(longDate("bad")).isEmpty()
    }

    // ── isoDateKey / isOverdue ───────────────────────────────────────────────
    @Test
    fun `isoDateKey takes date portion`() {
        assertThat(isoDateKey("2026-06-15T14:30:00Z")).isEqualTo("2026-06-15")
        assertThat(isoDateKey(null)).isEmpty()
        assertThat(isoDateKey("2026")).isEmpty()
    }

    @Test
    fun `isOverdue past true future false`() {
        assertThat(isOverdue("2000-01-01")).isTrue()
        assertThat(isOverdue("2999-12-31")).isFalse()
        assertThat(isOverdue(null)).isFalse()
        assertThat(isOverdue("")).isFalse()
    }

    // ── whenLabel ────────────────────────────────────────────────────────────
    @Test
    fun `whenLabel appends time when present`() {
        val yr = currentYear()
        assertThat(whenLabel("$yr-06-04T14:30:00Z")).isEqualTo("4 июн, 14:30")
        assertThat(whenLabel("$yr-06-04")).isEqualTo("4 июн")
    }

    @Test
    fun `whenLabel empty for bad input`() {
        assertThat(whenLabel(null)).isEmpty()
    }

    // ── parseInstantMillis ───────────────────────────────────────────────────
    @Test
    fun `parseInstantMillis parses Z form`() {
        // 2026-01-01T00:00:00Z = 1767225600000
        assertThat(parseInstantMillis("2026-01-01T00:00:00Z")).isEqualTo(1767225600000L)
    }

    @Test
    fun `parseInstantMillis parses offset and strips fractional seconds`() {
        val z = parseInstantMillis("2026-01-01T03:00:00.123456+03:00")
        assertThat(z).isEqualTo(1767225600000L) // same instant as midnight UTC
    }

    @Test
    fun `parseInstantMillis null on bad or blank`() {
        assertThat(parseInstantMillis(null)).isNull()
        assertThat(parseInstantMillis("")).isNull()
        assertThat(parseInstantMillis("not-a-date")).isNull()
    }

    @Test
    fun `parseInstantMillis roundtrips through millisToUtcIso`() {
        val iso = "2026-06-15T14:30:00Z"
        val millis = parseInstantMillis(iso)!!
        assertThat(millisToUtcIso(millis)).isEqualTo(iso)
    }

    // ── dueLabel (UTC-midnight date-only path is zone-independent) ────────────
    @Test
    fun `dueLabel date-only renders UTC calendar date without time`() {
        assertThat(dueLabel("2026-06-04T00:00:00Z")).isEqualTo("4 июн. 2026 г.")
        // even with +03:00 offset representing the same UTC-midnight instant
        assertThat(dueLabel("2026-06-04T03:00:00+03:00")).isEqualTo("4 июн. 2026 г.")
    }

    @Test
    fun `dueLabel blank empty and unparseable falls back to longDate`() {
        assertThat(dueLabel(null)).isEmpty()
        assertThat(dueLabel("2026-06-04")).isEqualTo("4 июн. 2026 г.") // unparseable instant → longDate
    }

    // ── dueShort (UTC-midnight path) ─────────────────────────────────────────
    @Test
    fun `dueShort date-only far year appends year`() {
        // Far past year, UTC-midnight → not relative, shows day+month+year
        assertThat(dueShort("2020-03-10T00:00:00Z")).isEqualTo("10 мар 2020")
    }

    @Test
    fun `dueShort blank empty`() {
        assertThat(dueShort(null)).isEmpty()
        assertThat(dueShort("x")).isEmpty()
    }

    // ── localDateTimeLabel ───────────────────────────────────────────────────
    @Test
    fun `localDateTimeLabel empty for unparseable`() {
        assertThat(localDateTimeLabel(null)).isEmpty()
        assertThat(localDateTimeLabel("bad")).isEmpty()
    }

    @Test
    fun `localDateTimeLabel formats a full instant`() {
        // Format is "<day> <mon> <year>, HH:mm" in local zone; assert shape.
        val out = localDateTimeLabel("2026-06-15T14:30:00Z")
        assertThat(out).matches("""\d+ \S+ 2026, \d{2}:\d{2}""")
    }
}
