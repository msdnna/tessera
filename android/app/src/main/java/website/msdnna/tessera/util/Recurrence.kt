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

/**
 * Толкает субботу/воскресенье вперёд на понедельник.
 *
 * Шаг — календарный (`add(DAY_OF_MONTH, …)`, как `AddDate` у бэкендового оригинала),
 * а не `+ n * 86_400_000`: в неделю перевода часов сутки не равны 24 часам. Осенью
 * воскресенье длится 25 часов, поэтому +48 ч от субботы попадали на ВОСКРЕСЕНЬЕ 23:00,
 * а +24 ч от воскресенья — на то же воскресенье: календарь подсвечивал ровно тот
 * выходной, который правило запрещает, и день, на который сервер не сработает.
 * Весной те же +48 ч уводили время суток на час вперёд.
 */
private fun applySkip(millis: Long, skip: Boolean): Long {
    if (!skip) return millis
    val c = cal(millis)
    val days = when (c.get(Calendar.DAY_OF_WEEK)) {
        Calendar.SATURDAY -> 2
        Calendar.SUNDAY -> 1
        else -> return millis
    }
    return c.apply { add(Calendar.DAY_OF_MONTH, days) }.timeInMillis
}

/** Anchored day-of-month (1..31), falling back to the due date's own day. */
private fun anchorDay(rule: Recurrence, from: Calendar): Int =
    if (rule.day in 1..31) rule.day else from.get(Calendar.DAY_OF_MONTH)

/** Anchored month as a 0-based Calendar month, falling back to the due date's. */
private fun anchorMonth(rule: Recurrence, from: Calendar): Int =
    if (rule.month in 1..12) rule.month - 1 else from.get(Calendar.MONTH)

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

/**
 * Lends a rule rebuilt from the editor's controls the anchor the server keeps on
 * our behalf — mirrors web `withStoredAnchor` and the backend's `recurrenceToStore`
 * (handlers/tasks.go).
 *
 * A monthly/yearly rule carries a server-managed day-of-month anchor ([Recurrence.day],
 * plus [Recurrence.month] for yearly). The picker's controls never hold it, so without
 * this a task that February has parked on the 28th would be previewed a March 28th it
 * is never going to fire on. Borrowed on the same terms the server uses: same
 * frequency, due date untouched. Otherwise the server re-derives the anchor from the
 * due date, which is what [nextOccurrence] falls back to on its own.
 */
fun withStoredAnchor(rule: Recurrence?, stored: Recurrence?, dueChanged: Boolean): Recurrence? {
    if (rule == null || (rule.freq != "monthly" && rule.freq != "yearly")) return rule
    if (dueChanged || stored == null || stored.freq != rule.freq) return rule
    return rule.copy(
        day = if (stored.day > 0) stored.day else rule.day,
        month = if (rule.freq == "yearly" && stored.month > 0) stored.month else rule.month,
    )
}

/**
 * True for a rule that can never fire because the task has no due date.
 *
 * Every one of the three triggers reads the due date and only the due date:
 * `recurAdvance` (backend/handlers/recurrence.go) returns a no-op while
 * `t.DueDate == nil` before it even looks at the trigger, so neither ticking the
 * task nor dragging it into the trigger column advances anything, and the
 * schedule worker's own query filters `due_date IS NOT NULL` besides.
 *
 * The rule is still accepted and stored, and the card draws its repeat badge, so
 * «каждый понедельник» on a task nobody dated looks live and is not (#2932). On
 * this form the door is wide open: picking a frequency chip sets the frequency
 * and nothing else, so a task without a due date commits the rule as it stands
 * (web's own editor dates the task for you, then lets «очистить срок» take it
 * back). Same shape as the dead trigger column — the panel has to say so.
 *
 * [dueMillis] is the picker's selected due instant, null when unset.
 */
fun dueDateMissing(rule: Recurrence?, dueMillis: Long?): Boolean {
    if (rule == null || rule.freq.isBlank()) return false
    return dueMillis == null
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
            // The anchor, not the (possibly clamped) due date: see Recurrence.day.
            dateOn(from, y, m, anchorDay(rule, from))
        }

        "yearly" -> dateOn(from, from.get(Calendar.YEAR) + iv, anchorMonth(rule, from), anchorDay(rule, from))

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
 *
 * Правило по списку дат идёт через тот же цикл, что и остальные частоты, и по той
 * же причине: сервер берёт первую дату СТРОГО ПОЗЖЕ срока и на `once` роняет
 * правило после одного срабатывания. Отдача всего выбранного списка обещала
 * срабатывания, которых не будет, и вдобавок подсвечивала даты, мимо которых серия
 * уже прошла (#2932). Сам выбранный список — дело редактора, здесь ответ только на
 * вопрос «в какие дни задача сработает».
 */
fun occurrenceKeys(rule: Recurrence?, fromMillis: Long?, n: Int = 24): Set<String> {
    val keys = mutableSetOf<String>()
    if (fromMillis == null || rule == null || rule.freq.isBlank()) return keys
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
