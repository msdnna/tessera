package website.msdnna.tessera.util

import java.text.SimpleDateFormat
import java.util.Calendar
import java.util.Date
import java.util.Locale
import java.util.TimeZone

private val months = listOf(
    "янв", "фев", "мар", "апр", "май", "июн", "июл", "авг", "сен", "окт", "ноя", "дек",
)

// Short weekday names, indexed by Calendar.DAY_OF_WEEK - 1 (1 = Sunday … 7 = Saturday).
private val weekdaysShort = listOf("вс", "пн", "вт", "ср", "чт", "пт", "сб")

/**
 * A relative day label for near-future/near-past dues, mirroring the web
 * `formatDue`: «Сегодня»/«Завтра»/«Вчера», or a capitalised weekday when the date
 * is elsewhere in the current (Monday-anchored) week. Returns "" to fall back to an
 * absolute date. [y]/[mo0]/[d] are the due's calendar components (mo0 zero-based).
 */
private fun relativeDue(y: Int, mo0: Int, d: Int): String {
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
        0 -> return "сегодня"
        1 -> return "завтра"
        -1 -> return "вчера"
    }
    if (Math.abs(diff) <= 6 && sameWeek(due, today)) {
        // weekdaysShort is already lowercase ("пн") — keep it lowercase, web parity.
        return weekdaysShort[due.get(Calendar.DAY_OF_WEEK) - 1]
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
fun shortDate(iso: String?): String {
    if (iso.isNullOrBlank() || iso.length < 10) return ""
    return try {
        val day = iso.substring(8, 10).trimStart('0')
        val month = iso.substring(5, 7).toInt()
        val year = iso.substring(0, 4)
        // Include the year only when it isn't the current one, so a far-off (or
        // stale) date isn't mistaken for one this year.
        val suffix = if (year == currentYear()) "" else " $year"
        "$day ${months[month - 1]}$suffix"
    } catch (_: Exception) {
        ""
    }
}

private fun currentYear(): String = Calendar.getInstance().get(Calendar.YEAR).toString()

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
 * A fuller `4 июн. 2026 г.` label for the task modal's due-date row. Returns ""
 * for blank/unparseable input. String-based (no java.time on minSdk 24).
 */
fun longDate(iso: String?): String {
    if (iso.isNullOrBlank() || iso.length < 10) return ""
    return try {
        val year = iso.substring(0, 4)
        val month = iso.substring(5, 7).toInt()
        val day = iso.substring(8, 10).trimStart('0')
        "$day ${months[month - 1]}. $year г."
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
fun dueLabel(iso: String?, withTime: Boolean = true): String {
    if (iso.isNullOrBlank() || iso.length < 10) return ""
    val millis = parseInstantMillis(iso) ?: return longDate(iso)
    // A pure UTC-midnight instant is a date-only due (GitLab/legacy) — render the
    // UTC calendar date, no time, so a server that serialises with a +03:00 offset
    // (e.g. `…T03:00:00+03:00`) doesn't surface a phantom "03:00". Mirrors web.
    if (isUtcMidnight(millis)) return utcLongDate(millis)
    val cal = Calendar.getInstance().apply { timeInMillis = millis }
    val day = cal.get(Calendar.DAY_OF_MONTH)
    val month = months[cal.get(Calendar.MONTH)]
    val year = cal.get(Calendar.YEAR)
    val date = "$day $month. $year г."
    val hh = cal.get(Calendar.HOUR_OF_DAY)
    val mm = cal.get(Calendar.MINUTE)
    return if (withTime && (hh != 0 || mm != 0)) "$date, %02d:%02d".format(hh, mm) else date
}

/**
 * A compact due label for cards. Near dates read as relative shorthand
 * («Завтра», or a weekday within the current week); otherwise `10 июн` (+ year
 * when not current), plus `14:30` when a time is set. Mirrors the web `formatDue`.
 */
fun dueShort(iso: String?): String {
    if (iso.isNullOrBlank() || iso.length < 10) return ""
    val millis = parseInstantMillis(iso) ?: return shortDate(iso)
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
        if (hh != 0 || mm != 0) "%02d:%02d".format(hh, mm) else ""
    } else {
        ""
    }
    val rel = relativeDue(year, mo0, day)
    if (rel.isNotEmpty()) return if (time.isNotEmpty()) "$rel $time" else rel
    val suffix = if (year.toString() == currentYear()) "" else " $year"
    val date = "$day ${months[mo0]}$suffix"
    return if (time.isNotEmpty()) "$date $time" else date
}

/**
 * A `4 июн., 14:30` timestamp for comments / journal entries. The time is the
 * raw ISO (UTC) clock — good enough for an at-a-glance "when".
 */
fun whenLabel(iso: String?): String {
    val date = shortDate(iso)
    if (date.isEmpty()) return ""
    val time = if (iso != null && iso.length >= 16) iso.substring(11, 16) else ""
    return if (time.isEmpty()) date else "$date, $time"
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

/** `4 июн. 2026 г.` from the UTC calendar date of [millis]. */
private fun utcLongDate(millis: Long): String {
    val cal = Calendar.getInstance(TimeZone.getTimeZone("UTC")).apply { timeInMillis = millis }
    val day = cal.get(Calendar.DAY_OF_MONTH)
    val month = months[cal.get(Calendar.MONTH)]
    val year = cal.get(Calendar.YEAR)
    return "$day $month. $year г."
}

/** Epoch millis → UTC ISO-8601 (`yyyy-MM-dd'T'HH:mm:ss'Z'`) for sending to the backend. */
fun millisToUtcIso(millis: Long): String =
    SimpleDateFormat("yyyy-MM-dd'T'HH:mm:ss'Z'", Locale.US)
        .apply { timeZone = TimeZone.getTimeZone("UTC") }
        .format(Date(millis))

/** A local-zone `15 июн 2026, 14:30` label for a full instant. "" if unparseable. */
fun localDateTimeLabel(iso: String?): String {
    val millis = parseInstantMillis(iso) ?: return ""
    val cal = Calendar.getInstance().apply { timeInMillis = millis }
    val day = cal.get(Calendar.DAY_OF_MONTH)
    val month = months[cal.get(Calendar.MONTH)]
    val year = cal.get(Calendar.YEAR)
    val hh = cal.get(Calendar.HOUR_OF_DAY).toString().padStart(2, '0')
    val mm = cal.get(Calendar.MINUTE).toString().padStart(2, '0')
    return "$day $month $year, $hh:$mm"
}
