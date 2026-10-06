package website.msdnna.tessera.util

import android.content.res.Resources
import kotlin.math.roundToInt
import website.msdnna.tessera.R
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
 *
 * Форматирующие функции берут [Resources] явно: объект живёт вне Compose, а язык
 * приходит из профиля (#2803, волна 10). Разбор ввода, наоборот, локали не знает —
 * он принимает и русские, и английские буквы единиц, поэтому «3d 4h», набранное
 * на английском интерфейсе, читается и после переключения языка.
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

    /**
     * Что значит число без суффикса, если у предыдущего токена суффикс был:
     * единица на шаг меньше — так пишут «1ч 30» и «1д 4». Минуты — дно лестницы.
     *
     * Раньше часами читалось КАЖДОЕ такое число, и «1ч 30» сохранялось как 31 час —
     * молча, во всех трёх разборах сразу (web `utils/estimation.js`, backend
     * `internal/quickact`). Единственный разобранный случай, «1d 4» = 720, — это
     * то же правило: для дня шаг вниз как раз и даёт часы.
     */
    private val SMALLER = mapOf('w' to 'd', 'd' to 'h', 'h' to 'm', 'm' to 'm')

    /**
     * Потолок разобранной оценки: минуты во временном пространстве, просто счёт —
     * в points/custom. Та же граница, что у веба (`utils/estimation.js`
     * MAX_ESTIMATE) и у бэкенда (`quickact.MaxEstimate`), и по той же причине —
     * число, которого не держит ни одна колонка, приезжает сюда обычной опечаткой:
     * вставленный id, зажатая клавиша (#2932).
     *
     * Здесь отсутствие потолка пряталось за `roundToInt`, который зажимает по
     * `Int.MAX_VALUE`: «999999999999» часов уходило в задачу как 2147483647 минут —
     * около четырёх тысяч лет, — и поле молча принимало то, от чего popover на вебе
     * и `/estimate` в комментарии отказываются с текстом. А в points/custom
     * достаточно длинный прогон цифр переполняет double до +Inf, и проверка
     * `n > 0` его пропускала: в PATCH уходило число, которого в JSON нет.
     */
    const val MAX_ESTIMATE = 1.0e9

    /** Единственная проверка выхода: конечное, больше нуля и внутри [MAX_ESTIMATE]. */
    private fun bounded(v: Double?): Double? =
        if (v != null && v.isFinite() && v > 0 && v <= MAX_ESTIMATE) v else null

    private fun minutesPerDay(cfg: EstimationConfig): Double = (cfg.hoursPerDay ?: 8.0) * 60.0
    private fun minutesPerWeek(cfg: EstimationConfig): Double =
        minutesPerDay(cfg) * (cfg.daysPerWeek ?: 5.0)

    /**
     * Число в НАЧАЛЕ строки, как его читает `parseFloat` в веб-оригинале: хвост
     * после числа игнорируется.
     *
     * Строгий `toDoubleOrNull` здесь стирал оценку. Диалог ввода (TaskModal
     * `EstimateValue`) предзаполняется тем, что вернул [format], а у единицы
     * `custom` это «12 тикетов» — число и название единицы. Человек открывал
     * оценку посмотреть, жал «ОК», ничего не поменяв, и получал null: значение
     * пропадало. Веб на том же сценарии (popover префилится той же подписью)
     * возвращает 12 и значение сохраняет.
     */
    private fun leadingNumber(s: String): Double? =
        LEADING_NUMBER.find(s)?.value?.toDoubleOrNull()

    private val LEADING_NUMBER = Regex("""^[+-]?(?:\d+(?:\.\d*)?|\.\d+)""")

    private fun trimNum(v: Double): String =
        if (v == v.toLong().toDouble()) v.toLong().toString() else ((v * 100).roundToInt() / 100.0).toString()

    /**
     * Parse free-text into the canonical value, or null when unparseable. Time
     * accepts mixed tokens ("1н 2д 3ч 30м"), English (w/d/h/m) or Russian
     * (н/д/ч/м) units, decimals, and a bare number (taken as hours). Points/custom
     * take a number — anything after it (the unit's own name, «12 тикетов») is
     * ignored, see [leadingNumber]; a T-shirt size label is accepted too.
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
            return bounded(leadingNumber(s))
        }

        return parseTime(s, cfg)
    }

    /**
     * Разбор времени: смешанные токены («1н 2д 3ч 30м») в канонические минуты.
     * Отдельной функцией, а не веткой [parse] — у той иначе восемь выходов на
     * один вход, и detekt (ReturnCount) прав: читать это как одну функцию нельзя.
     */
    private fun parseTime(s: String, cfg: EstimationConfig): Double? {
        val mpd = minutesPerDay(cfg)
        val unitMin = mapOf('w' to mpd * (cfg.daysPerWeek ?: 5.0), 'd' to mpd, 'h' to 60.0, 'm' to 1.0)
        var total = 0.0
        var matched = false
        // Кириллица здесь — разбор ввода, а не интерфейс: буквы единиц принимаются
        // в обоих алфавитах независимо от языка профиля.
        val re = Regex("""(\d+(?:\.\d+)?)\s*([a-zа-яё]*)""") // i18n-data
        // Единица предыдущего токена (null — у него её не было): число без
        // суффикса читается относительно неё, см. [SMALLER].
        var prev: Char? = null
        var first = true
        for (mr in re.findAll(s)) {
            val num = mr.groupValues[1].toDoubleOrNull() ?: continue
            val u = mr.groupValues[2]
            val suffix = when (u.firstOrNull()) {
                'w', 'н' -> 'w'

                'd', 'д' -> 'd'

                'h', 'ч' -> 'h'

                'm', 'м' -> 'm'

                null -> null

                // Незнакомый суффикс — не часы: «5 попугаев», сохранённые как
                // пять часов, хуже отказа (так же читает бэкенд, quickact).
                else -> return null
            }
            // «1ч 30» — полтора часа, не 31: у числа без суффикса единица на шаг
            // ниже предыдущей. Одиночное число впереди — часы; второе
            // безъединичное число подряд не значит ничего.
            val key = when {
                suffix != null -> suffix
                first -> 'h'
                prev != null -> SMALLER[prev] ?: 'm'
                else -> return null
            }
            prev = suffix
            first = false
            total += num * (unitMin[key] ?: 0.0)
            matched = true
        }
        if (!matched) return null
        // Потолок — ДО округления: `roundToInt` зажимает по `Int.MAX_VALUE` и тем
        // превращает переполнение в правдоподобное число, см. [MAX_ESTIMATE].
        val mins = bounded(total)?.roundToInt() ?: 0
        return if (mins > 0) mins.toDouble() else null
    }

    /**
     * Format a canonical value for display. Time compresses minutes to working
     * weeks/days/hours/minutes (30h with an 8h day → "3д 6ч"). Empty for null/≤0.
     */
    fun format(res: Resources, value: Double?, cfg: EstimationConfig): String {
        if (value == null || value <= 0) return ""
        when (cfg.unit.ifEmpty { "time" }) {
            "points" -> {
                if (cfg.pointsScale == "tshirt") {
                    return TSHIRT.firstOrNull { it.second.toDouble() == value }?.first ?: trimNum(value)
                }
                return res.getString(R.string.est_points_value, trimNum(value))
            }

            "custom" -> {
                val label = cfg.customLabel?.trim().orEmpty()
                return if (label.isNotEmpty()) {
                    res.getString(R.string.est_custom_value, trimNum(value), label)
                } else {
                    trimNum(value)
                }
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
            if (w > 0) add(res.getString(R.string.est_unit_week, w))
            if (d > 0) add(res.getString(R.string.est_unit_day, d))
            if (h > 0) add(res.getString(R.string.est_unit_hour, h))
            if (min > 0) add(res.getString(R.string.est_unit_minute, min))
        }
        // Ноль рисуется тем же суффиксом минут — отдельной строки на «0м» не нужно.
        return if (parts.isEmpty()) res.getString(R.string.est_unit_minute, 0) else parts.joinToString(" ")
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

    /** Human name of the unit, for settings labels and aggregates. Название своей
     *  единицы (`custom`) задал пользователь — оно остаётся как введено. */
    fun unitName(res: Resources, cfg: EstimationConfig): String = when (cfg.unit.ifEmpty { "time" }) {
        "points" -> res.getString(R.string.est_unit_points)
        "custom" -> cfg.customLabel?.trim()?.ifEmpty { null } ?: res.getString(R.string.est_unit_custom)
        else -> res.getString(R.string.est_unit_time)
    }

    /** Input placeholder hinting the accepted syntax for the resolved unit. */
    fun placeholder(res: Resources, cfg: EstimationConfig): String = when (cfg.unit.ifEmpty { "time" }) {
        "points" -> res.getString(R.string.est_hint_points)
        "custom" -> res.getString(R.string.est_hint_custom)
        else -> res.getString(R.string.est_hint_time)
    }

    /**
     * Convert a canonical estimate into a calendar-day span for the timeline/Gantt
     * "ghost" envelope. Only the time unit has a duration meaning (points/custom are
     * dimensionless → null). The estimate is taken as a fraction of a working week
     * and mapped onto 7 calendar days, so 4 working weeks of effort reads as ~4
     * calendar weeks on the axis. Mirrors web `estimateToDays`.
     */
    fun toDays(value: Double?, cfg: EstimationConfig): Double? {
        if (value == null || value <= 0) return null
        if (cfg.unit.ifEmpty { "time" } != "time") return null
        return value / minutesPerWeek(cfg) * 7.0
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

    /**
     * Сколько стоит ОДНА карточка: своя оценка, а если её нет — свёртка подзадач.
     * Ровно то число, что печатает сама карточка (TaskCard `EstimatePill`: `own ?: rollup`,
     * с приставкой «Σ»), поэтому всё, что суммирует набор карточек, обязано читать их так же.
     *
     * Зеркало веба `utils/estimation.js effectiveEstimate`.
     */
    fun effective(own: Double?, childEstimates: Iterable<Double?>): Double? =
        own ?: sum(childEstimates)

    /**
     * Трудозатраты дорожки — «Σ» в шапке этапа на доске и в дорожке таймлайна (#2932).
     *
     * Раньше это была `sum(lane.tasks.map { it.estimate })`, то есть СВОИ оценки строк, а
     * дорожка держит только верхнеуровневые карточки (подзадачи рисуются под родителем:
     * на доске — раскрытым списком, в таймлайне — тонкими суб-полосами). Задача, работа
     * которой расписана целиком по подзадачам — своей оценки нет, у каждого ребёнка есть, —
     * показывала на карточке «Σ 5д» и не давала в общий итог ровно ничего. По шапке читают
     * загрузку спринта, а молча выпадали из неё именно декомпозированные, то есть крупные.
     *
     * [children] обязан отдавать тот же список детей, из которого нарисована карточка
     * (BoardUiState `visibleSubtasksOf`), иначе шапка разойдётся с карточками под собой —
     * только с другой стороны. Зеркало веба `sumLaneEffort`.
     */
    fun <T> laneEffort(
        rows: Iterable<T>,
        own: (T) -> Double?,
        children: (T) -> Iterable<Double?>,
    ): Double? = sum(rows.map { effective(own(it), children(it)) })
}
