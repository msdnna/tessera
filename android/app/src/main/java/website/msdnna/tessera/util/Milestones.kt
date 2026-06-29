package website.msdnna.tessera.util

import website.msdnna.tessera.data.model.Milestone

/**
 * Shared milestone helpers — the Android mirror of the web `utils/milestones.js`.
 * Milestone dates are date-only (stored at UTC midnight), so we read the raw UTC
 * calendar date via [longDate] (which avoids the +03:00 offset trap).
 */
object Milestones {
    /** Human-readable start–due window (either side may be missing). "" when neither. */
    fun range(start: String?, due: String?): String {
        val s = longDate(start)
        val d = longDate(due)
        return when {
            s.isNotEmpty() && d.isNotEmpty() -> "$s – $d"
            d.isNotEmpty() -> "до $d"
            s.isNotEmpty() -> "с $s"
            else -> ""
        }
    }

    fun range(m: Milestone?): String = if (m == null) "" else range(m.startDate, m.dueDate)
}
