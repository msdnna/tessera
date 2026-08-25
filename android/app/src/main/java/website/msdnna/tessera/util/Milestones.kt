package website.msdnna.tessera.util

import android.content.res.Resources
import website.msdnna.tessera.R
import website.msdnna.tessera.data.model.Milestone

/**
 * Shared milestone helpers — the Android mirror of the web `utils/milestones.js`.
 * Milestone dates are date-only (stored at UTC midnight), so we read the raw UTC
 * calendar date via [longDate] (which avoids the +03:00 offset trap).
 *
 * Подписи окна берут [Resources] явно, как и сами даты (#2803, волна 11).
 */
object Milestones {
    /** Human-readable start–due window (either side may be missing). "" when neither. */
    fun range(res: Resources, start: String?, due: String?): String {
        val s = longDate(res, start)
        val d = longDate(res, due)
        return when {
            s.isNotEmpty() && d.isNotEmpty() -> res.getString(R.string.milestones_range, s, d)
            d.isNotEmpty() -> res.getString(R.string.milestones_range_until, d)
            s.isNotEmpty() -> res.getString(R.string.milestones_range_from, s)
            else -> ""
        }
    }

    fun range(res: Resources, m: Milestone?): String =
        if (m == null) "" else range(res, m.startDate, m.dueDate)
}
