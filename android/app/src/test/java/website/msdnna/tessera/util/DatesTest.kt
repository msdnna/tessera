package website.msdnna.tessera.util

import android.content.Context
import android.content.res.Resources
import androidx.test.core.app.ApplicationProvider
import com.google.common.truth.Truth.assertThat
import java.util.Calendar
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner

/**
 * Подписи дат собираются из ресурсов (#2803, волна 11), поэтому тест идёт под
 * Robolectric и проверяет обе локали: у английской другой порядок «месяц день,
 * год», и склейка строкой сломала бы именно его.
 */
@RunWith(RobolectricTestRunner::class)
class DatesTest {
    private fun res(language: String): Resources =
        ApplicationProvider.getApplicationContext<Context>().withLanguage(language).resources

    private val ru: Resources get() = res("ru")
    private val en: Resources get() = res("en")

    private fun currentYear() = Calendar.getInstance().get(Calendar.YEAR).toString()

    // ── shortDate ────────────────────────────────────────────────────────────
    @Test
    fun `shortDate blank or too-short is empty`() {
        assertThat(shortDate(ru, null)).isEmpty()
        assertThat(shortDate(ru, "")).isEmpty()
        assertThat(shortDate(ru, "2026-06")).isEmpty()
    }

    @Test
    fun `shortDate strips leading zero on day and maps month`() {
        val yr = currentYear()
        assertThat(shortDate(ru, "$yr-06-05T00:00:00Z")).isEqualTo("5 июн")
        assertThat(shortDate(ru, "$yr-12-25")).isEqualTo("25 дек")
        assertThat(shortDate(ru, "$yr-01-01")).isEqualTo("1 янв")
    }

    @Test
    fun `shortDate in English puts the month first`() {
        val yr = currentYear()
        assertThat(shortDate(en, "$yr-06-05T00:00:00Z")).isEqualTo("Jun 5")
        assertThat(shortDate(en, "2020-03-10")).isEqualTo("Mar 10, 2020")
    }

    @Test
    fun `shortDate appends year when not current`() {
        assertThat(shortDate(ru, "2020-03-10")).isEqualTo("10 мар 2020")
    }

    @Test
    fun `shortDate invalid month yields empty`() {
        assertThat(shortDate(ru, "2026-13-10")).isEmpty()
    }

    // ── longDate ─────────────────────────────────────────────────────────────
    @Test
    fun `longDate formats with year and g suffix`() {
        assertThat(longDate(ru, "2026-06-04T00:00:00Z")).isEqualTo("4 июн. 2026 г.")
        assertThat(longDate(ru, "2020-01-31")).isEqualTo("31 янв. 2020 г.")
    }

    @Test
    fun `longDate in English drops the year suffix`() {
        assertThat(longDate(en, "2026-06-04T00:00:00Z")).isEqualTo("Jun 4, 2026")
    }

    @Test
    fun `longDate blank empty`() {
        assertThat(longDate(ru, null)).isEmpty()
        assertThat(longDate(ru, "bad")).isEmpty()
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
        assertThat(whenLabel(ru, "$yr-06-04T14:30:00Z")).isEqualTo("4 июн, 14:30")
        assertThat(whenLabel(ru, "$yr-06-04")).isEqualTo("4 июн")
        assertThat(whenLabel(en, "$yr-06-04T14:30:00Z")).isEqualTo("Jun 4, 14:30")
    }

    @Test
    fun `whenLabel empty for bad input`() {
        assertThat(whenLabel(ru, null)).isEmpty()
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
        assertThat(dueLabel(ru, "2026-06-04T00:00:00Z")).isEqualTo("4 июн. 2026 г.")
        // even with +03:00 offset representing the same UTC-midnight instant
        assertThat(dueLabel(ru, "2026-06-04T03:00:00+03:00")).isEqualTo("4 июн. 2026 г.")
        assertThat(dueLabel(en, "2026-06-04T00:00:00Z")).isEqualTo("Jun 4, 2026")
    }

    @Test
    fun `dueLabel blank empty and unparseable falls back to longDate`() {
        assertThat(dueLabel(ru, null)).isEmpty()
        // unparseable instant → longDate
        assertThat(dueLabel(ru, "2026-06-04")).isEqualTo("4 июн. 2026 г.")
    }

    // ── dueShort (UTC-midnight path) ─────────────────────────────────────────
    @Test
    fun `dueShort date-only far year appends year`() {
        // Far past year, UTC-midnight → not relative, shows day+month+year
        assertThat(dueShort(ru, "2020-03-10T00:00:00Z")).isEqualTo("10 мар 2020")
        assertThat(dueShort(en, "2020-03-10T00:00:00Z")).isEqualTo("Mar 10, 2020")
    }

    @Test
    fun `dueShort today reads as the relative label in both locales`() {
        val today = Calendar.getInstance()
        val iso = "%04d-%02d-%02dT00:00:00Z".format(
            today.get(Calendar.YEAR), today.get(Calendar.MONTH) + 1, today.get(Calendar.DAY_OF_MONTH),
        )
        assertThat(dueShort(ru, iso)).isEqualTo("сегодня")
        assertThat(dueShort(en, iso)).isEqualTo("today")
    }

    /**
     * Массив дней недели идёт с понедельника, а `Calendar.DAY_OF_WEEK` — с
     * воскресенья: перепутанный сдвиг тихо назовёт вторник средой. Проверяем все
     * дни текущей недели, кроме вчера/сегодня/завтра (у них своя подпись), —
     * таких в неделе всегда минимум четыре, так что тест не зависит от даты.
     */
    @Test
    fun `dueShort names the weekday inside the current week`() {
        val monday = Calendar.getInstance().apply {
            firstDayOfWeek = Calendar.MONDAY
            set(Calendar.HOUR_OF_DAY, 0)
            set(Calendar.MINUTE, 0)
            set(Calendar.SECOND, 0)
            set(Calendar.MILLISECOND, 0)
            set(Calendar.DAY_OF_WEEK, Calendar.MONDAY)
        }
        // Полночь, как и в самой подписи: иначе разница в днях округлится иначе и
        // «вчера» попадёт в проверку дней недели.
        val today = Calendar.getInstance().apply {
            set(Calendar.HOUR_OF_DAY, 0)
            set(Calendar.MINUTE, 0)
            set(Calendar.SECOND, 0)
            set(Calendar.MILLISECOND, 0)
        }
        val names = ru.getStringArray(website.msdnna.tessera.R.array.dates_weekdays_short)
        var checked = 0
        for (offset in 0..6) {
            val day = (monday.clone() as Calendar).apply { add(Calendar.DAY_OF_MONTH, offset) }
            val diff = Math.round((day.timeInMillis - today.timeInMillis) / 86_400_000.0).toInt()
            if (Math.abs(diff) <= 1) continue
            val iso = "%04d-%02d-%02dT00:00:00Z".format(
                day.get(Calendar.YEAR), day.get(Calendar.MONTH) + 1, day.get(Calendar.DAY_OF_MONTH),
            )
            assertThat(dueShort(ru, iso)).isEqualTo(names[offset])
            checked++
        }
        assertThat(checked).isAtLeast(4)
    }

    @Test
    fun `dueShort blank empty`() {
        assertThat(dueShort(ru, null)).isEmpty()
        assertThat(dueShort(ru, "x")).isEmpty()
    }

    // ── localDateTimeLabel ───────────────────────────────────────────────────
    @Test
    fun `localDateTimeLabel empty for unparseable`() {
        assertThat(localDateTimeLabel(ru, null)).isEmpty()
        assertThat(localDateTimeLabel(ru, "bad")).isEmpty()
    }

    @Test
    fun `localDateTimeLabel formats a full instant`() {
        // Format is "<day> <mon> <year>, HH:mm" in local zone; assert shape.
        val out = localDateTimeLabel(ru, "2026-06-15T14:30:00Z")
        assertThat(out).matches("""\d+ \S+ 2026, \d{2}:\d{2}""")
        // English reorders to "<mon> <day>, <year>, HH:mm".
        assertThat(localDateTimeLabel(en, "2026-06-15T14:30:00Z"))
            .matches("""\S+ \d+, 2026, \d{2}:\d{2}""")
    }
}
