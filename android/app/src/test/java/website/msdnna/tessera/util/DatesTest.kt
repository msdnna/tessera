package website.msdnna.tessera.util

import android.content.Context
import android.content.res.Resources
import androidx.test.core.app.ApplicationProvider
import com.google.common.truth.Truth.assertThat
import java.util.Calendar
import java.util.TimeZone
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

    /** Форма полной даты до #2857 — теперь это пресет `medium`, а дефолт — `short`. */
    private val medium = DateFormatPrefs(datePreset = DatePresets.MEDIUM)
    private val h12 = DateFormatPrefs(time12h = true, datePreset = DatePresets.MEDIUM)

    private fun currentYear() = Calendar.getInstance().get(Calendar.YEAR).toString()

    /**
     * ISO момента, который в зоне устройства читается как [hour]:[minute] числа
     * [day].[month] [year]. Часовые подписи рисуются в локальной зоне, поэтому
     * зашитый в тест `…T14:30:00Z` проверял бы зону хоста, а не формат.
     */
    private fun isoAtLocal(year: Int, month: Int, day: Int, hour: Int, minute: Int): String {
        val cal = Calendar.getInstance().apply {
            clear()
            set(year, month - 1, day, hour, minute, 0)
        }
        return millisToUtcIso(cal.timeInMillis)
    }

    /** Местная полночь сегодняшнего дня — та же точка отсчёта, что и у подписи. */
    private fun midnightToday(): Calendar = Calendar.getInstance().apply {
        set(Calendar.HOUR_OF_DAY, 0)
        set(Calendar.MINUTE, 0)
        set(Calendar.SECOND, 0)
        set(Calendar.MILLISECOND, 0)
    }

    /** Календарь конкретной даты ([month] с единицы), время не участвует. */
    private fun date(year: Int, month: Int, day: Int): Calendar = Calendar.getInstance().apply {
        clear()
        set(year, month - 1, day)
    }

    /** ISO чистой UTC-полуночи дня `base + offset` — форма срока без времени. */
    private fun isoUtcMidnight(base: Calendar, offset: Int): String {
        val day = (base.clone() as Calendar).apply { add(Calendar.DAY_OF_MONTH, offset) }
        return "%04d-%02d-%02dT00:00:00Z".format(
            day.get(Calendar.YEAR), day.get(Calendar.MONTH) + 1, day.get(Calendar.DAY_OF_MONTH),
        )
    }

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
        assertThat(longDate(ru, "2026-06-04T00:00:00Z", medium)).isEqualTo("4 июн. 2026 г.")
        assertThat(longDate(ru, "2020-01-31", medium)).isEqualTo("31 янв. 2020 г.")
    }

    @Test
    fun `longDate in English drops the year suffix`() {
        assertThat(longDate(en, "2026-06-04T00:00:00Z", medium)).isEqualTo("Jun 4, 2026")
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

    // ── dueDayKey ────────────────────────────────────────────────────────────
    // Какому дню принадлежит срок, решают «просрочено» и фильтры «Сегодня/На
    // неделе» — и решать это надо тем же днём, который стоит в подписи карточки.
    // Зона фиксируется в самом тесте: срез сырой ISO совпадает с локальным днём
    // ровно до тех пор, пока хост сидит в UTC, и на хосте в UTC любая проверка
    // этого расхождения выродилась бы в тавтологию.

    /** Выполняет [body] с подменённой зоной устройства, возвращая прежнюю. */
    private fun inZone(id: String, body: () -> Unit) {
        val prev = TimeZone.getDefault()
        TimeZone.setDefault(TimeZone.getTimeZone(id))
        try {
            body()
        } finally {
            TimeZone.setDefault(prev)
        }
    }

    @Test
    fun `dueDayKey reads a timed due in the device zone`() = inZone("Asia/Tokyo") {
        // Срок «15 сентября, локальная полночь» в Токио (+09) уезжает на сутки
        // назад, когда сервер сериализует его в UTC. Подпись покажет 15-е —
        // ключ дня обязан согласиться, иначе задача краснеет за день до срока.
        assertThat(dueDayKey("2026-09-14T15:00:00Z")).isEqualTo("2026-09-15")
        assertThat(dueDayKey("2026-09-15T00:00:00+09:00")).isEqualTo("2026-09-15")
    }

    @Test
    fun `dueDayKey keeps a date-only due on its UTC day`() = inZone("America/New_York") {
        // Дата без времени (GitLab/legacy) — это чистая UTC-полночь, и читать её
        // в локальной зоне нельзя: в Нью-Йорке (-04) она сползла бы на 13-е.
        // Ровно эту ветку рисует dueLabel/dueShort.
        assertThat(dueDayKey("2026-09-14T00:00:00Z")).isEqualTo("2026-09-14")
        assertThat(dueDayKey("2026-09-14T03:00:00+03:00")).isEqualTo("2026-09-14")
    }

    @Test
    fun `dueDayKey falls back to the raw date portion`() {
        // Голая дата без времени не разбирается как инстант — берём срез, как и
        // делали раньше: это уже календарный день, конвертировать нечего.
        assertThat(dueDayKey("2026-06-15")).isEqualTo("2026-06-15")
        assertThat(dueDayKey(null)).isEmpty()
        assertThat(dueDayKey("")).isEmpty()
        assertThat(dueDayKey("2026")).isEmpty()
    }

    @Test
    fun `dueDayKey agrees with the label the card shows`() = inZone("Asia/Tokyo") {
        // Тот же момент, что в первом тесте: подпись рисует 15 сентября, значит
        // и ключ дня — 15-е. Раньше подпись и ключ расходились на сутки.
        assertThat(dueLabel(ru, "2026-09-14T15:00:00Z", fmt = medium))
            .isEqualTo("15 сен. 2026 г.")
        assertThat(dueDayKey("2026-09-14T15:00:00Z")).isEqualTo("2026-09-15")
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
        assertThat(dueLabel(ru, "2026-06-04T00:00:00Z", fmt = medium)).isEqualTo("4 июн. 2026 г.")
        // even with +03:00 offset representing the same UTC-midnight instant
        assertThat(dueLabel(ru, "2026-06-04T03:00:00+03:00", fmt = medium)).isEqualTo("4 июн. 2026 г.")
        assertThat(dueLabel(en, "2026-06-04T00:00:00Z", fmt = medium)).isEqualTo("Jun 4, 2026")
    }

    @Test
    fun `dueLabel blank empty and unparseable falls back to longDate`() {
        assertThat(dueLabel(ru, null)).isEmpty()
        // unparseable instant → longDate
        assertThat(dueLabel(ru, "2026-06-04", fmt = medium)).isEqualTo("4 июн. 2026 г.")
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

    /**
     * Неделя рвётся там, где её рвёт преф `week_start`, а не всегда в понедельник.
     * Расхождение двух календарей — ровно два воскресенья вокруг сегодняшнего дня:
     * то, которым понедельничная неделя закрывается, и то, которым воскресная
     * открывается. Проверяем оба (кроме дней, у которых своя подпись «вчера/
     * сегодня/завтра»): под «своим» префом это «вс», под чужим — обычная дата.
     */
    @Test
    fun `dueShort takes the week boundary from the week_start preference`() {
        val today = midnightToday()
        val names = ru.getStringArray(website.msdnna.tessera.R.array.dates_weekdays_short)
        // Пн = 0 … Вс = 6 (порядок массива подписей), и отдельно номер дня от воскресенья.
        val fromMonday = (today.get(Calendar.DAY_OF_WEEK) + 5) % 7
        val fromSunday = today.get(Calendar.DAY_OF_WEEK) - Calendar.SUNDAY
        // Воскресенье — единственный день, у которого обе границы сходятся на нём
        // самом: тогда проверять нечего, у «сегодня» своя подпись.
        val candidates = listOf(6 - fromMonday to WEEK_START_MONDAY, -fromSunday to WEEK_START_SUNDAY)
        var checked = 0
        for ((offset, ownWeekStart) in candidates) {
            if (Math.abs(offset) <= 1) continue
            val iso = isoUtcMidnight(today, offset)
            val alien = if (ownWeekStart == WEEK_START_MONDAY) WEEK_START_SUNDAY else WEEK_START_MONDAY
            assertThat(dueShort(ru, iso, DateFormatPrefs(weekStart = ownWeekStart))).isEqualTo(names[6])
            assertThat(dueShort(ru, iso, DateFormatPrefs(weekStart = alien))).isNotEqualTo(names[6])
            checked++
        }
        if (checked == 0) assertThat(fromSunday).isEqualTo(0)
    }

    @Test
    fun `dueShort keeps Monday as the default and for an unknown preference value`() {
        // Преф правится через API, и незнакомое число не должно уводить неделю в
        // день, которого нет ни в одном календаре (веб сужает так же).
        val today = midnightToday()
        val sundayOffset = 6 - (today.get(Calendar.DAY_OF_WEEK) + 5) % 7
        if (Math.abs(sundayOffset) <= 1) return
        val iso = isoUtcMidnight(today, sundayOffset)
        val sunday = ru.getStringArray(website.msdnna.tessera.R.array.dates_weekdays_short)[6]
        assertThat(dueShort(ru, iso)).isEqualTo(sunday)
        assertThat(dueShort(ru, iso, DateFormatPrefs(weekStart = 42))).isEqualTo(sunday)
    }

    // ── sameWeek ─────────────────────────────────────────────────────────────
    @Test
    fun `sameWeek breaks the week at the chosen first day`() {
        val friday = date(2026, 9, 18)
        val sunday = date(2026, 9, 20)
        val tuesday = date(2026, 9, 22)
        // Понедельничная неделя 14–20.09: пятница и воскресенье вместе, вторник уже нет.
        assertThat(sameWeek(friday, sunday, WEEK_START_MONDAY)).isTrue()
        assertThat(sameWeek(sunday, tuesday, WEEK_START_MONDAY)).isFalse()
        // Воскресная неделя 20–26.09: ровно наоборот.
        assertThat(sameWeek(friday, sunday, WEEK_START_SUNDAY)).isFalse()
        assertThat(sameWeek(sunday, tuesday, WEEK_START_SUNDAY)).isTrue()
    }

    @Test
    fun `sameWeek holds across the turn of the year`() {
        // Номер недели тут не работает: у 31.12 и 01.01 разный YEAR, и неделя
        // распадалась надвое — подпись «пт» пропадала ровно в новогоднюю неделю.
        val thursday = date(2026, 12, 31)
        val friday = date(2027, 1, 1)
        assertThat(sameWeek(thursday, friday, WEEK_START_MONDAY)).isTrue()
        assertThat(sameWeek(thursday, friday, WEEK_START_SUNDAY)).isTrue()
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
        val out = localDateTimeLabel(ru, "2026-06-15T14:30:00Z", medium)
        assertThat(out).matches("""\d+ \S+ 2026 г\., \d{2}:\d{2}""")
        // English reorders to "<mon> <day>, <year>, HH:mm".
        assertThat(localDateTimeLabel(en, "2026-06-15T14:30:00Z", medium))
            .matches("""\S+ \d+, 2026, \d{2}:\d{2}""")
    }

    // ── пресеты даты и 12-часовое время (#2857) ───────────────────────────────
    @Test
    fun `longDate renders every date preset`() {
        val iso = "2026-12-31"
        assertThat(longDate(ru, iso, DateFormatPrefs(datePreset = DatePresets.SHORT))).isEqualTo("31.12.2026")
        assertThat(longDate(ru, iso, DateFormatPrefs(datePreset = DatePresets.MEDIUM))).isEqualTo("31 дек. 2026 г.")
        assertThat(longDate(ru, iso, DateFormatPrefs(datePreset = DatePresets.LONG))).isEqualTo("31 декабря 2026 г.")
        assertThat(longDate(ru, iso, DateFormatPrefs(datePreset = DatePresets.ISO))).isEqualTo("2026-12-31")
    }

    /** English keeps day-before-month (en-GB, как на вебе), а `iso` локали не знает. */
    @Test
    fun `longDate presets keep the English field order`() {
        val iso = "2026-12-31"
        assertThat(longDate(en, iso, DateFormatPrefs(datePreset = DatePresets.SHORT))).isEqualTo("31/12/2026")
        assertThat(longDate(en, iso, DateFormatPrefs(datePreset = DatePresets.MEDIUM))).isEqualTo("Dec 31, 2026")
        assertThat(longDate(en, iso, DateFormatPrefs(datePreset = DatePresets.LONG))).isEqualTo("31 December 2026")
        assertThat(longDate(en, iso, DateFormatPrefs(datePreset = DatePresets.ISO))).isEqualTo("2026-12-31")
    }

    /** Значение из чужого клиента не должно рисовать битую дату. */
    @Test
    fun `longDate falls back to short on an unknown preset`() {
        val fmt = DateFormatPrefs(datePreset = "totally-unknown")
        assertThat(longDate(ru, "2026-12-31", fmt)).isEqualTo("31.12.2026")
        assertThat(longDate(ru, "2026-13-31", fmt)).isEmpty()
    }

    @Test
    fun `dueLabel follows the 12h preference`() {
        val iso = isoAtLocal(2026, 6, 4, 14, 30)
        assertThat(dueLabel(ru, iso, fmt = h12)).isEqualTo("4 июн. 2026 г., 2:30 PM")
        assertThat(dueLabel(ru, iso, fmt = medium)).isEqualTo("4 июн. 2026 г., 14:30")
    }

    /** Полночь и полдень — «12», а не «0»: остаток от деления назвал бы их 0:30. */
    @Test
    fun `12h clock names midnight and noon as 12`() {
        assertThat(dueLabel(ru, isoAtLocal(2026, 6, 4, 0, 30), fmt = h12)).isEqualTo("4 июн. 2026 г., 12:30 AM")
        assertThat(dueLabel(ru, isoAtLocal(2026, 6, 4, 12, 30), fmt = h12)).isEqualTo("4 июн. 2026 г., 12:30 PM")
    }

    @Test
    fun `dueShort keeps the compact date but follows the 12h preference`() {
        // Дата остаётся коротким «день + месяц» (веб-паритет), меняются только часы.
        val iso = isoAtLocal(2020, 3, 10, 14, 30)
        assertThat(dueShort(ru, iso, h12)).isEqualTo("10 мар 2020 2:30 PM")
        assertThat(dueShort(ru, iso, medium)).isEqualTo("10 мар 2020 14:30")
    }

    @Test
    fun `whenLabel follows the 12h preference`() {
        val yr = currentYear()
        assertThat(whenLabel(ru, "$yr-06-04T14:30:00Z", h12)).isEqualTo("4 июн, 2:30 PM")
        assertThat(whenLabel(ru, "$yr-06-04T14:30:00Z", medium)).isEqualTo("4 июн, 14:30")
    }

    @Test
    fun `localDateTimeLabel follows the date preset`() {
        assertThat(localDateTimeLabel(ru, "2026-06-15T14:30:00Z", DateFormatPrefs(datePreset = DatePresets.ISO)))
            .matches("""2026-06-1\d, \d{2}:\d{2}""")
    }
}
