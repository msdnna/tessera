package website.msdnna.tessera.util

import java.util.Calendar

/*
 * Где рвётся неделя в календарных сетках — преф `week_start`, а не понедельник.
 * Порт того, что на вебе делают `BoardCalendarView.vue` (firstDow), `utils/timeAxis.js`
 * (buildWeekBands) и `MilestoneManager.vue` (firstDayOfWeek у пикеров).
 *
 * Причина вынесена в отдельный util, а не размазана по Compose-файлам: сеток три
 * (месячный вид доски, недельные полосы Ганта, попап даты вехи), они живут в разных
 * экранах, и до этого каждая считала разрыв недели своей формулой — жёстко на
 * понедельнике. У пользователя с воскресной неделей это значило, что пикер срока,
 * открытый с карточки, начинался с воскресенья, а календарь под ним — с понедельника.
 *
 * Чистые функции: ни Compose, ни часов — поэтому разрыв недели покрывается тестами.
 */
private const val DAYS_IN_WEEK = 7

object WeekGrid {
    /** [Calendar.SUNDAY] или [Calendar.MONDAY] — день, которым открывается неделя. */
    fun firstDayOfWeek(weekStart: Int): Int =
        if (normalizeWeekStart(weekStart) == WEEK_START_SUNDAY) Calendar.SUNDAY else Calendar.MONDAY

    /**
     * Сколько пустых ячеек идёт перед днём с [dayOfWeek] ([Calendar.DAY_OF_WEEK],
     * воскресенье = 1) в сетке, открывающейся на [weekStart].
     */
    fun lead(dayOfWeek: Int, weekStart: Int): Int =
        (dayOfWeek - firstDayOfWeek(weekStart) + DAYS_IN_WEEK) % DAYS_IN_WEEK

    /** True, когда [dayOfWeek] открывает неделю — на нём рвутся полосы недель на Ганте. */
    fun opensWeek(dayOfWeek: Int, weekStart: Int): Boolean = dayOfWeek == firstDayOfWeek(weekStart)

    /**
     * Подписи дней недели в порядке самой сетки. На вход — массив ресурсов
     * `calendar_weekdays_short`, который всегда идёт с понедельника; при воскресной
     * неделе воскресенье переезжает в начало.
     *
     * Массив не из семи элементов (перевод потеряли) возвращается как есть: пусть
     * подписи будут не в том порядке, чем упадёт экран на чужой локали.
     */
    fun labels(mondayFirst: List<String>, weekStart: Int): List<String> =
        if (mondayFirst.size == DAYS_IN_WEEK && normalizeWeekStart(weekStart) == WEEK_START_SUNDAY) {
            listOf(mondayFirst.last()) + mondayFirst.dropLast(1)
        } else {
            mondayFirst
        }

    /**
     * Индекс дня с [dayOfWeek] в НЕповёрнутом массиве `calendar_weekdays_short`
     * (понедельник = 0). Нужен там, где подпись берут по конкретному дню, а не по
     * колонке сетки, — порядок префа на такую выборку не влияет.
     */
    fun labelIndex(dayOfWeek: Int): Int = (dayOfWeek + 5) % DAYS_IN_WEEK
}
