package website.msdnna.tessera.util

import java.util.TimeZone
import website.msdnna.tessera.data.model.Conference

/**
 * A meeting's plan beyond its start (#3034, web `utils/confPlan.js`): its length
 * and repeat (#3052) and the stage it is tied to (#3053).
 *
 * The server owns the arithmetic of a repeat — it answers `next_at`, the next
 * occurrence not over yet, in the meeting's own zone. The client names the rule
 * and sends the reader's zone with it.
 */
data class ConferencePlan(
    val durationMinutes: Int?,
    /** "" (no repeat) or one of [CONF_RECURRENCES]. */
    val recurrence: String,
    val timezone: String,
    val milestoneId: String?,
)

/** The repeat presets, in the order the picker offers them; "" is «не повторять». */
val CONF_RECURRENCES = listOf("", "daily", "weekdays", "weekly", "biweekly", "monthly")

/** The lengths the form offers, in minutes; 30 is the default. */
val CONF_DURATIONS = listOf(15, 30, 45, 60, 90, 120)
const val CONF_DEFAULT_DURATION = 30

/**
 * The plan a form sends. A repeat needs a start (the server refuses one
 * without), so it is dropped when no time is picked; the zone rides along only
 * with a repeat — a one-off has no clock to keep.
 */
fun conferencePlan(
    scheduledAtIso: String?,
    durationMinutes: Int?,
    recurrence: String,
    milestoneId: String?,
    zone: String = TimeZone.getDefault().id,
): ConferencePlan {
    val rule = if (scheduledAtIso.isNullOrBlank()) "" else recurrence.takeIf { it in CONF_RECURRENCES }.orEmpty()
    return ConferencePlan(
        durationMinutes = durationMinutes,
        recurrence = rule,
        timezone = if (rule.isEmpty()) "" else zone,
        milestoneId = milestoneId?.takeIf { it.isNotBlank() },
    )
}

/** The instant a row is filed by: the next occurrence of a repeat, else the start. */
fun conferencePlanAt(c: Conference): String? = c.nextAt?.takeIf { it.isNotBlank() } ?: c.scheduledAt

/** Hours and minutes of a length, for «45 мин» / «1 ч» / «1 ч 30 мин»; null without one. */
fun conferenceDurationParts(minutes: Int?): Pair<Int, Int>? {
    val n = minutes ?: return null
    if (n < 1) return null
    return n / 60 to n % 60
}
