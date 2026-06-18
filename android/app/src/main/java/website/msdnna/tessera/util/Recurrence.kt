package website.msdnna.tessera.util

import java.util.Calendar
import website.msdnna.tessera.data.model.Recurrence

/**
 * Client-side mirror of the backend `internal/recur` advance logic, used only to
 * preview a recurrence on the due-date calendar (highlight upcoming occurrences).
 * The backend remains the source of truth. Works in the device's local zone, on
 * epoch-millis instants (matching the due-date picker).
 */

private fun cal(millis: Long): Calendar = Calendar.getInstance().apply { timeInMillis = millis }

private fun dayKeyOf(c: Calendar): String =
    "%04d-%02d-%02d".format(c.get(Calendar.YEAR), c.get(Calendar.MONTH) + 1, c.get(Calendar.DAY_OF_MONTH))

fun millisDayKey(millis: Long): String = dayKeyOf(cal(millis))

private fun daysInMonth(year: Int, month: Int): Int =
    Calendar.getInstance().apply {
        clear()
        set(year, month, 1)
    }.getActualMaximum(Calendar.DAY_OF_MONTH)

private fun dateOn(base: Calendar, year: Int, month: Int, day: Int): Long {
    val d = minOf(day, daysInMonth(year, month))
    return Calendar.getInstance().apply {
        clear()
        set(year, month, d, base.get(Calendar.HOUR_OF_DAY), base.get(Calendar.MINUTE), 0)
    }.timeInMillis
}

private fun applySkip(millis: Long, skip: Boolean): Long {
    if (!skip) return millis
    val c = cal(millis)
    return when (c.get(Calendar.DAY_OF_WEEK)) {
        Calendar.SATURDAY -> millis + 2 * 86_400_000L
        Calendar.SUNDAY -> millis + 86_400_000L
        else -> millis
    }
}

/** weekday 0=Sun..6=Sat (matches the rule's encoding). */
private fun weekday0(c: Calendar): Int = c.get(Calendar.DAY_OF_WEEK) - Calendar.SUNDAY

private fun nextWeekly(from: Calendar, rule: Recurrence): Long {
    val days = rule.weekdays?.toSortedSet() ?: sortedSetOf()
    val iv = maxOf(1, rule.interval)
    if (days.isEmpty()) {
        return (from.clone() as Calendar).apply { add(Calendar.DAY_OF_MONTH, 7 * iv) }.timeInMillis
    }
    // Later selected weekday this Monday-week?
    val monday = (from.clone() as Calendar).apply {
        add(Calendar.DAY_OF_MONTH, -((weekday0(from) + 6) % 7))
    }
    for (i in 1..6) {
        val d = (from.clone() as Calendar).apply { add(Calendar.DAY_OF_MONTH, i) }
        val weekEnd = (monday.clone() as Calendar).apply { add(Calendar.DAY_OF_MONTH, 7) }
        if (d.before(weekEnd) && days.contains(weekday0(d))) return d.timeInMillis
    }
    val target = (monday.clone() as Calendar).apply { add(Calendar.DAY_OF_MONTH, 7 * iv) }
    for (i in 0..6) {
        val d = (target.clone() as Calendar).apply { add(Calendar.DAY_OF_MONTH, i) }
        if (days.contains(weekday0(d))) return d.timeInMillis
    }
    return (from.clone() as Calendar).apply { add(Calendar.DAY_OF_MONTH, 7 * iv) }.timeInMillis
}

/** Next occurrence after [fromMillis], or null when the recurrence has ended. */
fun nextOccurrence(rule: Recurrence?, fromMillis: Long): Long? {
    if (rule == null || rule.freq.isBlank()) return null
    val from = cal(fromMillis)
    val iv = maxOf(1, rule.interval)
    return when (rule.freq) {
        "daily" -> applySkip((from.clone() as Calendar).apply { add(Calendar.DAY_OF_MONTH, iv) }.timeInMillis, rule.skipWeekends)

        "weekly" -> applySkip(nextWeekly(from, rule), rule.skipWeekends)

        "monthly" -> {
            val total = from.get(Calendar.MONTH) + iv
            val y = from.get(Calendar.YEAR) + Math.floorDiv(total, 12)
            val m = Math.floorMod(total, 12)
            dateOn(from, y, m, from.get(Calendar.DAY_OF_MONTH))
        }

        "yearly" -> dateOn(from, from.get(Calendar.YEAR) + iv, from.get(Calendar.MONTH), from.get(Calendar.DAY_OF_MONTH))

        "custom" -> {
            val key = dayKeyOf(from)
            rule.dates?.sorted()?.firstOrNull { it > key }?.let { s ->
                val (yy, mm, dd) = s.split("-").map { it.toInt() }
                Calendar.getInstance().apply {
                    clear()
                    set(yy, mm - 1, dd, from.get(Calendar.HOUR_OF_DAY), from.get(Calendar.MINUTE), 0)
                }.timeInMillis
            }
        }

        else -> null
    }
}

/**
 * Up to [n] upcoming occurrence day-keys (yyyy-MM-dd) starting at [fromMillis].
 * A one-off rule (`once`) shows just the current due + the single next occurrence.
 */
fun occurrenceKeys(rule: Recurrence?, fromMillis: Long?, n: Int = 24): Set<String> {
    val keys = mutableSetOf<String>()
    if (fromMillis == null || rule == null || rule.freq.isBlank()) return keys
    if (rule.freq == "custom") {
        rule.dates?.forEach { keys.add(it) }
        return keys
    }
    val limit = if (rule.once) 1 else n
    var cur: Long = fromMillis
    keys.add(millisDayKey(cur))
    repeat(limit) {
        val nx = nextOccurrence(rule, cur) ?: return keys
        keys.add(millisDayKey(nx))
        cur = nx
    }
    return keys
}
