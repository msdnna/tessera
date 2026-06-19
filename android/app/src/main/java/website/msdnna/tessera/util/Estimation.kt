package website.msdnna.tessera.util

import kotlin.math.roundToInt
import website.msdnna.tessera.data.model.EstimationConfig
import website.msdnna.tessera.data.model.Project
import website.msdnna.tessera.data.model.Workspace

/**
 * Task-estimation helpers — the Android mirror of the web `utils/estimation.js`.
 * The backend stores one canonical `estimate` number per task; its unit lives in
 * a two-level config (project override → workspace default → the built-in
 * default below). This object resolves that config and does all input parsing /
 * output formatting:
 *   • time   → canonical minutes ("3д 4ч"/"1н"/"90м" parse via the working day,
 *              output compresses back so "3 дня" = 24 working hours);
 *   • points → the point number on a scale (Fibonacci / T-shirt / linear);
 *   • custom → a count of a named unit.
 */
object Estimation {
    val DEFAULT = EstimationConfig(unit = "time", hoursPerDay = 8.0, daysPerWeek = 5.0)

    val FIBONACCI = listOf(1, 2, 3, 5, 8, 13, 21)
    val LINEAR = (1..10).toList()

    // T-shirt sizes → canonical number (label shown, number stored).
    val TSHIRT = listOf("XS" to 1, "S" to 2, "M" to 3, "L" to 5, "XL" to 8, "XXL" to 13)

    /** Effective config for a project: its override, else the workspace default, else built-in. */
    fun resolve(project: Project?, workspace: Workspace?): EstimationConfig =
        project?.estimation ?: workspace?.estimation ?: DEFAULT

    private fun minutesPerDay(cfg: EstimationConfig): Double = (cfg.hoursPerDay ?: 8.0) * 60.0
    private fun minutesPerWeek(cfg: EstimationConfig): Double =
        minutesPerDay(cfg) * (cfg.daysPerWeek ?: 5.0)

    private fun trimNum(v: Double): String =
        if (v == v.toLong().toDouble()) v.toLong().toString() else ((v * 100).roundToInt() / 100.0).toString()

    /**
     * Parse free-text into the canonical value, or null when unparseable. Time
     * accepts mixed tokens ("1н 2д 3ч 30м"), English (w/d/h/m) or Russian
     * (н/д/ч/м) units, decimals, and a bare number (taken as hours). Points/custom
     * take a plain number; a T-shirt size label is accepted too.
     */
    fun parse(input: String?, cfg: EstimationConfig): Double? {
        if (input == null) return null
        val s = input.trim().lowercase().replace(',', '.')
        if (s.isEmpty()) return null
        val unit = cfg.unit.ifEmpty { "time" }

        if (unit != "time") {
            if (unit == "points" && cfg.pointsScale == "tshirt") {
                TSHIRT.firstOrNull { it.first.lowercase() == s }?.let { return it.second.toDouble() }
            }
            val n = s.toDoubleOrNull()
            return if (n != null && n > 0) n else null
        }

        val mpd = minutesPerDay(cfg)
        val unitMin = mapOf('w' to mpd * (cfg.daysPerWeek ?: 5.0), 'd' to mpd, 'h' to 60.0, 'm' to 1.0)
        var total = 0.0
        var matched = false
        val re = Regex("""(\d+(?:\.\d+)?)\s*([a-zа-яё]*)""")
        for (mr in re.findAll(s)) {
            val num = mr.groupValues[1].toDoubleOrNull() ?: continue
            val u = mr.groupValues[2]
            val key = when (u.firstOrNull()) {
                'w', 'н' -> 'w'

                'd', 'д' -> 'd'

                'h', 'ч' -> 'h'

                'm', 'м' -> 'm'

                null -> 'h'

                // bare number → hours
                else -> 'h'
            }
            total += num * (unitMin[key] ?: 0.0)
            matched = true
        }
        if (!matched) return null
        val mins = total.roundToInt()
        return if (mins > 0) mins.toDouble() else null
    }

    /**
     * Format a canonical value for display. Time compresses minutes to working
     * weeks/days/hours/minutes (30h with an 8h day → "3д 6ч"). Empty for null/≤0.
     */
    fun format(value: Double?, cfg: EstimationConfig): String {
        if (value == null || value <= 0) return ""
        when (cfg.unit.ifEmpty { "time" }) {
            "points" -> {
                if (cfg.pointsScale == "tshirt") {
                    return TSHIRT.firstOrNull { it.second.toDouble() == value }?.first ?: trimNum(value)
                }
                return "${trimNum(value)} SP"
            }

            "custom" -> {
                val label = cfg.customLabel?.trim().orEmpty()
                return if (label.isNotEmpty()) "${trimNum(value)} $label" else trimNum(value)
            }
        }
        val mpd = minutesPerDay(cfg).roundToInt()
        val mpw = minutesPerWeek(cfg).roundToInt()
        var rem = value.roundToInt()
        val w = rem / mpw
        rem -= w * mpw
        val d = rem / mpd
        rem -= d * mpd
        val h = rem / 60
        rem -= h * 60
        val min = rem
        val parts = buildList {
            if (w > 0) add("${w}н")
            if (d > 0) add("${d}д")
            if (h > 0) add("${h}ч")
            if (min > 0) add("${min}м")
        }
        return if (parts.isEmpty()) "0м" else parts.joinToString(" ")
    }

    /** Discrete options for a point picker (empty for time/custom). Pairs of (label, value). */
    fun scaleOptions(cfg: EstimationConfig): List<Pair<String, Double>> {
        if (cfg.unit != "points") return emptyList()
        return when (cfg.pointsScale) {
            "tshirt" -> TSHIRT.map { it.first to it.second.toDouble() }
            "linear" -> LINEAR.map { it.toString() to it.toDouble() }
            else -> FIBONACCI.map { it.toString() to it.toDouble() }
        }
    }

    /** Human name of the unit, for settings labels and aggregates. */
    fun unitName(cfg: EstimationConfig): String = when (cfg.unit.ifEmpty { "time" }) {
        "points" -> "Стори-поинты"
        "custom" -> cfg.customLabel?.trim()?.ifEmpty { null } ?: "Единицы"
        else -> "Время"
    }

    /** Input placeholder hinting the accepted syntax for the resolved unit. */
    fun placeholder(cfg: EstimationConfig): String = when (cfg.unit.ifEmpty { "time" }) {
        "points" -> "напр. 5"
        "custom" -> "напр. 8"
        else -> "напр. 3д 4ч, 90м, 1н"
    }

    /** Sum a task list's estimates (rollup / lane total). Null when none are set. */
    fun sum(values: Iterable<Double?>): Double? {
        var total = 0.0
        var any = false
        for (e in values) {
            if (e != null && e > 0) {
                total += e
                any = true
            }
        }
        return if (any) total else null
    }
}
