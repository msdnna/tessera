package website.msdnna.tessera.util

import java.util.TimeZone
import website.msdnna.tessera.data.model.Preferences

/**
 * Формат даты и времени из профиля (#2857).
 *
 * Раньше `time_format` и `date_format` только сохранялись: [Dates] рисовал время
 * всегда в 24-часовом формате, а дату — всегда одной и той же формой. Здесь эти
 * два префа приводятся к тому, что реально нужно рендеру: флаг 12/24 и один из
 * именованных пресетов даты.
 *
 * Пресеты — веб-семантика (`frontend/src/utils/format.js`): паттерн вида
 * `dd.MM.yyyy` жёстко задаёт порядок полей и противоречит выбранному языку,
 * поэтому веб с #2798 хранит имя пресета, а порядок полей отдаёт локали. Android
 * писал паттерны, веб — имена, и оба значения ходят через один и тот же
 * `user_preferences`, так что [normalizeDatePreset] обязан принимать обе формы:
 * незнакомое значение приходит от другого клиента и должно вырождаться в
 * дефолт, а не ломать подпись.
 */
data class DateFormatPrefs(
    /** `time_format = 12h` — время рисуется как `2:30 PM`, иначе как `14:30`. */
    val time12h: Boolean = false,
    /** Один из [DatePresets], уже нормализованный. */
    val datePreset: String = DatePresets.SHORT,
    /** `week_start`: [WEEK_START_SUNDAY] или [WEEK_START_MONDAY], уже нормализованный. */
    val weekStart: Int = WEEK_START_MONDAY,
    /**
     * `timezone` — календарь, в котором клиент раскладывает даты по дням. Пустая
     * строка значит «зона устройства» (см. [resolvedTimezone]); хранится именно
     * строкой, а не [TimeZone], чтобы [Default] не застывал на зоне, действовавшей
     * в момент первого обращения к классу.
     *
     * Этот же преф уходит серверу как `?tz=` на ручках, считающих ПО ДНЯМ
     * (`AppContainer.callerZone`). Пока его знал только сервер, домашний экран
     * считал свои две половины в разных календарях: плитки «Просрочено/Сегодня/
     * На неделе» приходили посчитанными в зоне профиля, а список под ними
     * бакетился зоной устройства — у человека, чей преф разошёлся с телефоном
     * (поездка, зона выставлена вручную), плитка показывала одно число, а список
     * под ней — другой набор задач.
     */
    val zoneId: String = "",
) {
    /** Календарь, в котором считаются дни: зона префа, иначе зона устройства. */
    val zone: TimeZone
        get() = if (zoneId.isBlank()) TimeZone.getDefault() else TimeZone.getTimeZone(zoneId)

    companion object {
        /** Дефолт веба: 24 часа + `short` + неделя с понедельника + зона устройства. */
        val Default = DateFormatPrefs()

        fun of(prefs: Preferences): DateFormatPrefs = DateFormatPrefs(
            time12h = prefs.timeFormat == TIME_12H,
            datePreset = normalizeDatePreset(prefs.dateFormat),
            weekStart = normalizeWeekStart(prefs.weekStart),
            zoneId = normalizeZoneId(prefs.timezone),
        )
    }
}

/**
 * Преф зоны, суженный до того, что телефон умеет показать: пустое значение и
 * незнакомое имя одинаково означают «зона устройства».
 *
 * Незнакомое имя гасится здесь, а не отдаётся [TimeZone.getTimeZone], потому что
 * тот на мусор молча возвращает GMT: опечатка в профиле увела бы весь календарь в
 * UTC, и понять это по экрану было бы нечем. Серверу такое значение по-прежнему
 * уходит как записано — он сам решает, грузится ли зона, и падает на следующий
 * источник (см. [resolvedTimezone]).
 */
fun normalizeZoneId(value: String?): String {
    val tz = value?.trim().orEmpty()
    if (tz.isEmpty()) return ""
    return if (tz in TimeZone.getAvailableIDs()) tz else ""
}

const val TIME_12H = "12h"
const val TIME_24H = "24h"

/** `week_start = 0` — неделя начинается с воскресенья. */
const val WEEK_START_SUNDAY = 0

/** `week_start = 1` — неделя начинается с понедельника (дефолт веба). */
const val WEEK_START_MONDAY = 1

/**
 * День начала недели из хранимого значения: воскресенье — только явный ноль, всё
 * остальное — понедельник. Так же сужает значение веб (`weekStart === 0 ? 0 : 1`
 * в `utils/format.js`): преф редактируется через API, и неизвестное число не должно
 * уводить подписи в день, которого нет ни в одном из двух календарей.
 */
fun normalizeWeekStart(value: Int): Int = if (value == WEEK_START_SUNDAY) WEEK_START_SUNDAY else WEEK_START_MONDAY

/** Имена пресетов даты — те же, что пишет веб. */
object DatePresets {
    const val SHORT = "short"
    const val MEDIUM = "medium"
    const val LONG = "long"
    const val ISO = "iso"

    val ALL = listOf(SHORT, MEDIUM, LONG, ISO)
}

/**
 * Паттерны, которые Android писал в `date_format` до перехода на пресеты (и
 * которыми до сих пор могут быть заполнены профили), — на ближайший пресет.
 */
private val LEGACY_PATTERNS = mapOf(
    "dd.MM.yyyy" to DatePresets.SHORT,
    "dd/MM/yyyy" to DatePresets.SHORT,
    "MM/dd/yyyy" to DatePresets.SHORT,
    "yyyy-MM-dd" to DatePresets.ISO,
)

/** Пресет из хранимого значения: имя как есть, легаси-паттерн — в ближайший, прочее — в дефолт. */
fun normalizeDatePreset(value: String?): String {
    if (value != null && value in DatePresets.ALL) return value
    return LEGACY_PATTERNS[value] ?: DatePresets.SHORT
}

/** Формат времени из хранимого значения; всё незнакомое — 24 часа, как на вебе. */
fun normalizeTimeFormat(value: String?): String = if (value == TIME_12H) TIME_12H else TIME_24H
