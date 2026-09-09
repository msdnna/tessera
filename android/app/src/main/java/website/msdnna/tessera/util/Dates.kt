package website.msdnna.tessera.util

import android.content.res.Resources
import java.text.SimpleDateFormat
import java.util.Calendar
import java.util.Date
import java.util.Locale
import java.util.TimeZone
import website.msdnna.tessera.R

/**
 * Подписи дат. Названия месяцев и дней недели, относительные «сегодня/завтра» и
 * сам порядок «день месяц год» живут в ресурсах (#2803, волна 11): в английской
 * локали порядок другой («Jun 4, 2026»), поэтому строки собираются шаблоном, а не
 * склейкой.
 *
 * Форматирующие функции берут [Resources] явным параметром — они зовутся и из
 * композиции (там приходит `LocalResources`, уже подменённый `AppLocale` на язык
 * профиля), и из обычных функций-хелперов экранов. Посчитать подпись заранее и
 * положить в состояние нельзя: она застыла бы на языке момента расчёта.
 *
 * Разбор ISO ([parseInstantMillis], [isoDateKey], [isOverdue], [millisToUtcIso])
 * локали не знает и ресурсов не требует — это данные, а не интерфейс.
 *
 * Формат времени и форма полной даты приходят отдельным параметром
 * [DateFormatPrefs] (#2857): это преф профиля, а не язык, и подменой [Resources]
 * его не передать. В композиции значение берут из `LocalDateFormat`.
 */
private fun months(res: Resources): Array<String> = res.getStringArray(R.array.dates_months_short)

private fun monthsFull(res: Resources): Array<String> = res.getStringArray(R.array.dates_months_full)

/** Время суток: `14:30` или `2:30 PM` — по префу `time_format`. */
private fun clock(res: Resources, hh: Int, mm: Int, fmt: DateFormatPrefs): String {
    if (!fmt.time12h) return res.getString(R.string.dates_clock, hh, mm)
    // 0 и 12 часов в 12-часовой записи — это «12», отсюда сдвиг, а не остаток.
    val h12 = ((hh + 11) % 12) + 1
    val suffix = res.getString(if (hh < 12) R.string.dates_am else R.string.dates_pm)
    return res.getString(R.string.dates_clock_12, h12, mm, suffix)
}

/**
 * Полная дата в форме выбранного пресета ([DatePresets]); [mo0] — месяц с нуля.
 *
 * `iso` собирается вручную и одинаков в обеих локалях — он локале-независим по
 * определению; остальные три идут через ресурсы, потому что порядок полей
 * («31 дек. 2026 г.» против «Dec 31, 2026») задаёт язык, а не мы.
 */
private fun presetDate(res: Resources, y: Int, mo0: Int, d: Int, preset: String): String = when (preset) {
    DatePresets.ISO -> "%04d-%02d-%02d".format(y, mo0 + 1, d)
    DatePresets.MEDIUM -> res.getString(R.string.dates_long, d, months(res)[mo0], y)
    DatePresets.LONG -> res.getString(R.string.dates_preset_long, d, monthsFull(res)[mo0], y)
    else -> res.getString(R.string.dates_preset_short, d, mo0 + 1, y)
}

/**
 * A relative day label for near-future/near-past dues, mirroring the web
 * `formatDue`: «Сегодня»/«Завтра»/«Вчера», or a capitalised weekday when the date
 * is elsewhere in the current (Monday-anchored) week. Returns "" to fall back to an
 * absolute date. [y]/[mo0]/[d] are the due's calendar components (mo0 zero-based).
 */
private fun relativeDue(res: Resources, y: Int, mo0: Int, d: Int): String {
    val due = Calendar.getInstance().apply {
        clear()
        set(y, mo0, d)
    }
    val today = Calendar.getInstance().apply {
        set(Calendar.HOUR_OF_DAY, 0)
        set(Calendar.MINUTE, 0)
        set(Calendar.SECOND, 0)
        set(Calendar.MILLISECOND, 0)
    }
    val diff = Math.round((due.timeInMillis - today.timeInMillis) / 86_400_000.0).toInt()
    when (diff) {
        0 -> return res.getString(R.string.dates_today)
        1 -> return res.getString(R.string.dates_tomorrow)
        -1 -> return res.getString(R.string.dates_yesterday)
    }
    if (Math.abs(diff) <= 6 && sameWeek(due, today)) {
        // Массив идёт с понедельника, Calendar.DAY_OF_WEEK — с воскресенья (1).
        // Подписи в ru намеренно строчные («пн»), как на вебе.
        val idx = (due.get(Calendar.DAY_OF_WEEK) + 5) % 7
        return res.getStringArray(R.array.dates_weekdays_short)[idx]
    }
    return ""
}

/** True when two dates fall in the same Monday-anchored calendar week. */
private fun sameWeek(a: Calendar, b: Calendar): Boolean {
    val ca = (a.clone() as Calendar).apply { firstDayOfWeek = Calendar.MONDAY }
    val cb = (b.clone() as Calendar).apply { firstDayOfWeek = Calendar.MONDAY }
    return ca.get(Calendar.YEAR) == cb.get(Calendar.YEAR) &&
        ca.get(Calendar.WEEK_OF_YEAR) == cb.get(Calendar.WEEK_OF_YEAR)
}

/**
 * Formats an ISO-8601 timestamp (e.g. `2026-06-10T00:00:00Z`) to a short
 * `10 июн` label. String-based to avoid java.time desugaring on minSdk 24.
 * Returns "" for blank/unparseable input.
 */
fun shortDate(res: Resources, iso: String?): String {
    if (iso.isNullOrBlank() || iso.length < 10) return ""
    return try {
        val day = iso.substring(8, 10).toInt()
        val month = months(res)[iso.substring(5, 7).toInt() - 1]
        val year = iso.substring(0, 4).toInt()
        // Include the year only when it isn't the current one, so a far-off (or
        // stale) date isn't mistaken for one this year.
        if (year == currentYear()) {
            res.getString(R.string.dates_short, day, month)
        } else {
            res.getString(R.string.dates_short_year, day, month, year)
        }
    } catch (_: Exception) {
        ""
    }
}

private fun currentYear(): Int = Calendar.getInstance().get(Calendar.YEAR)

/** Today's `yyyy-MM-dd` in the device zone. */
private fun todayKey(): String {
    val cal = Calendar.getInstance()
    return "%04d-%02d-%02d".format(
        cal.get(Calendar.YEAR), cal.get(Calendar.MONTH) + 1, cal.get(Calendar.DAY_OF_MONTH),
    )
}

/** True when a date-only due is before today (a task overdue if not completed). */
fun isOverdue(iso: String?): Boolean {
    val key = isoDateKey(iso)
    return key.isNotEmpty() && key < todayKey()
}

/** The `yyyy-MM-dd` date portion of an ISO timestamp, or "" for the undated. */
fun isoDateKey(iso: String?): String =
    if (iso.isNullOrBlank() || iso.length < 10) "" else iso.substring(0, 10)

/**
 * A full date label for the task modal's due-date row — its shape follows the
 * user's `date_format` preset (`31.12.2026` / `31 дек. 2026 г.` / `31 декабря
 * 2026 г.` / `2026-12-31`). Returns "" for blank/unparseable input. String-based
 * (no java.time on minSdk 24).
 */
fun longDate(res: Resources, iso: String?, fmt: DateFormatPrefs = DateFormatPrefs.Default): String {
    if (iso.isNullOrBlank() || iso.length < 10) return ""
    return try {
        val year = iso.substring(0, 4).toInt()
        val mo0 = iso.substring(5, 7).toInt() - 1
        val day = iso.substring(8, 10).toInt()
        // Месяц проверяем здесь: presetDate для iso/short в массив названий не
        // ходит и «13-й месяц» дорисовал бы битую дату вместо пустой строки.
        require(mo0 in 0..11)
        presetDate(res, year, mo0, day, fmt.datePreset)
    } catch (_: Exception) {
        ""
    }
}

/**
 * A due-date label for the task modal/cards. Mirrors the web `formatDue`:
 *  - a pure UTC-midnight value is a date-only due (GitLab/legacy) → render the raw
 *    UTC calendar date, no time;
 *  - otherwise it's a real instant → show the local date, plus the local time when
 *    it isn't local-midnight.
 * [withTime] = false forces the date-only form (for cramped card pills).
 */
fun dueLabel(
    res: Resources,
    iso: String?,
    withTime: Boolean = true,
    fmt: DateFormatPrefs = DateFormatPrefs.Default,
): String {
    if (iso.isNullOrBlank() || iso.length < 10) return ""
    val millis = parseInstantMillis(iso) ?: return longDate(res, iso, fmt)
    // A pure UTC-midnight instant is a date-only due (GitLab/legacy) — render the
    // UTC calendar date, no time, so a server that serialises with a +03:00 offset
    // (e.g. `…T03:00:00+03:00`) doesn't surface a phantom "03:00". Mirrors web.
    if (isUtcMidnight(millis)) return utcLongDate(res, millis, fmt)
    val cal = Calendar.getInstance().apply { timeInMillis = millis }
    val date = presetDate(
        res,
        cal.get(Calendar.YEAR),
        cal.get(Calendar.MONTH),
        cal.get(Calendar.DAY_OF_MONTH),
        fmt.datePreset,
    )
    val hh = cal.get(Calendar.HOUR_OF_DAY)
    val mm = cal.get(Calendar.MINUTE)
    return if (withTime && (hh != 0 || mm != 0)) {
        res.getString(R.string.dates_at_time, date, clock(res, hh, mm, fmt))
    } else {
        date
    }
}

/**
 * A compact due label for cards. Near dates read as relative shorthand
 * («Завтра», or a weekday within the current week); otherwise `10 июн` (+ year
 * when not current), plus `14:30` when a time is set. Mirrors the web `formatDue`.
 *
 * The `date_format` preset deliberately does NOT apply here: web's `formatDue`
 * pins the compact form to day + short month and lets only the clock follow
 * `time_format`. A card pill asking for `31.12.2026` would be the wider shape,
 * not the shorthand.
 */
fun dueShort(res: Resources, iso: String?, fmt: DateFormatPrefs = DateFormatPrefs.Default): String {
    if (iso.isNullOrBlank() || iso.length < 10) return ""
    val millis = parseInstantMillis(iso) ?: return shortDate(res, iso)
    // A pure UTC-midnight instant is a date-only due (GitLab/legacy) — read its
    // calendar date in UTC so a +03:00 server offset doesn't push it to "03:00".
    // A real timed/local-midnight due falls through to local rendering.
    val utcMid = isUtcMidnight(millis)
    val cal = Calendar.getInstance(if (utcMid) TimeZone.getTimeZone("UTC") else TimeZone.getDefault())
        .apply { timeInMillis = millis }
    val day = cal.get(Calendar.DAY_OF_MONTH)
    val mo0 = cal.get(Calendar.MONTH)
    val year = cal.get(Calendar.YEAR)
    val time = if (!utcMid) {
        val hh = cal.get(Calendar.HOUR_OF_DAY)
        val mm = cal.get(Calendar.MINUTE)
        if (hh != 0 || mm != 0) clock(res, hh, mm, fmt) else ""
    } else {
        ""
    }
    val rel = relativeDue(res, year, mo0, day)
    val date = if (rel.isNotEmpty()) {
        rel
    } else if (year == currentYear()) {
        res.getString(R.string.dates_short, day, months(res)[mo0])
    } else {
        res.getString(R.string.dates_short_year, day, months(res)[mo0], year)
    }
    return if (time.isNotEmpty()) res.getString(R.string.dates_with_time, date, time) else date
}

/**
 * A `4 июн., 14:30` timestamp for comments / journal entries. The time is the
 * raw ISO (UTC) clock — good enough for an at-a-glance "when".
 */
fun whenLabel(res: Resources, iso: String?, fmt: DateFormatPrefs = DateFormatPrefs.Default): String {
    val date = shortDate(res, iso)
    if (date.isEmpty()) return ""
    val raw = if (iso != null && iso.length >= 16) iso.substring(11, 16) else ""
    if (raw.isEmpty()) return date
    // The clock is the raw ISO one (see above) — only its *shape* follows
    // `time_format`, so a 12h user reads "2:30 PM" over the same instant.
    val time = if (fmt.time12h) {
        val hh = raw.substring(0, 2).toIntOrNull()
        val mm = raw.substring(3, 5).toIntOrNull()
        if (hh != null && mm != null) clock(res, hh, mm, fmt) else raw
    } else {
        raw
    }
    return res.getString(R.string.dates_at_time, date, time)
}

// ── Full instants (reminders) — these carry a real time-of-day, so they parse
// to an epoch and render/deliver in the device's local zone (unlike the
// date-only helpers above, which read the raw UTC clock). ────────────────────

/**
 * Parses a full ISO-8601 instant (`2026-06-15T14:30:00Z`, with offset, or with
 * fractional seconds) to epoch millis, or null if unparseable. Fractional
 * seconds are stripped first so a single set of patterns covers Go's RFC3339.
 */
fun parseInstantMillis(iso: String?): Long? {
    if (iso.isNullOrBlank()) return null
    val cleaned = iso.replace(Regex("""\.\d+"""), "")
    val patterns = listOf("yyyy-MM-dd'T'HH:mm:ssXXX", "yyyy-MM-dd'T'HH:mm:ss'Z'", "yyyy-MM-dd'T'HH:mmXXX")
    for (pat in patterns) {
        runCatching {
            SimpleDateFormat(pat, Locale.US).apply { timeZone = TimeZone.getTimeZone("UTC") }.parse(cleaned)
        }.getOrNull()?.let { return it.time }
    }
    return null
}

/** True when [millis] lands exactly on UTC midnight — i.e. a date-only due. */
private fun isUtcMidnight(millis: Long): Boolean {
    val cal = Calendar.getInstance(TimeZone.getTimeZone("UTC")).apply { timeInMillis = millis }
    return cal.get(Calendar.HOUR_OF_DAY) == 0 &&
        cal.get(Calendar.MINUTE) == 0 &&
        cal.get(Calendar.SECOND) == 0
}

/** A full date (in the user's preset) from the UTC calendar date of [millis]. */
private fun utcLongDate(res: Resources, millis: Long, fmt: DateFormatPrefs): String {
    val cal = Calendar.getInstance(TimeZone.getTimeZone("UTC")).apply { timeInMillis = millis }
    return presetDate(
        res,
        cal.get(Calendar.YEAR),
        cal.get(Calendar.MONTH),
        cal.get(Calendar.DAY_OF_MONTH),
        fmt.datePreset,
    )
}

/** Epoch millis → UTC ISO-8601 (`yyyy-MM-dd'T'HH:mm:ss'Z'`) for sending to the backend. */
fun millisToUtcIso(millis: Long): String =
    SimpleDateFormat("yyyy-MM-dd'T'HH:mm:ss'Z'", Locale.US)
        .apply { timeZone = TimeZone.getTimeZone("UTC") }
        .format(Date(millis))

/** A local-zone `31.12.2026, 14:30` label (date in the user's preset) for a full instant. */
fun localDateTimeLabel(res: Resources, iso: String?, fmt: DateFormatPrefs = DateFormatPrefs.Default): String {
    val millis = parseInstantMillis(iso) ?: return ""
    val cal = Calendar.getInstance().apply { timeInMillis = millis }
    val date = presetDate(
        res,
        cal.get(Calendar.YEAR),
        cal.get(Calendar.MONTH),
        cal.get(Calendar.DAY_OF_MONTH),
        fmt.datePreset,
    )
    return res.getString(
        R.string.dates_at_time,
        date,
        clock(res, cal.get(Calendar.HOUR_OF_DAY), cal.get(Calendar.MINUTE), fmt),
    )
}

/**
 * Just the time of day, in the user's `time_format` preset.
 *
 * Without the date on purpose: the caller is the in-call chat, where every line
 * was said during the same meeting and the date would repeat on all of them.
 */
fun localTimeLabel(res: Resources, iso: String?, fmt: DateFormatPrefs = DateFormatPrefs.Default): String {
    val millis = parseInstantMillis(iso) ?: return ""
    val cal = Calendar.getInstance().apply { timeInMillis = millis }
    return clock(res, cal.get(Calendar.HOUR_OF_DAY), cal.get(Calendar.MINUTE), fmt)
}
