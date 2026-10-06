package website.msdnna.tessera.util

import java.util.Calendar

/**
 * Разметка оси дней для таймлайна и ганта — зеркало веб-`utils/timeAxis.js`
 * (`startOfDay` / `msAtDayOffset`) и `useChartTimeline` (`dayIndex`, длина окна).
 *
 * Ось раскладывается по МЕСТНЫМ календарным суткам, а сутки с переводом часов не
 * равны 24 часам, поэтому ни один шаг здесь не считается миллисекундами:
 *
 *  * осенью воскресенье длится 25 часов, и `rangeStart + i * DAY_MS` попадает на
 *    23:00 предыдущего дня — шапка подписывала всю правую часть оси на день назад
 *    (номер дня дублировался, выходные и кружок «сегодня» уезжали), тогда как
 *    полосы задач, построенные от настоящих полуночей, оставались на месте;
 *  * весной сутки длятся 23 часа, и целочисленное деление разницы на 24 часа даёт
 *    на день меньше — полоса и линия «сегодня» вставали в соседнюю ячейку.
 *
 * Вне перевода часов оба способа совпадают ровно, поэтому в зонах без перевода
 * (та же Москва) разницы не видно вовсе.
 */
object TimeAxis {
    /** Длина суток без перевода часов. Только для дробной геометрии пикселей. */
    const val DAY_MS: Long = 86_400_000L

    /** Местная полночь тех суток, в которые попадает [ms]. */
    fun dayFloor(ms: Long): Long =
        Calendar.getInstance().apply {
            timeInMillis = ms
            set(Calendar.HOUR_OF_DAY, 0)
            set(Calendar.MINUTE, 0)
            set(Calendar.SECOND, 0)
            set(Calendar.MILLISECOND, 0)
        }.timeInMillis

    /**
     * Полночь суток, отстоящих от [rangeStart] на [days] календарных дней.
     *
     * Начало окна само по себе бывает не полночью (веб отодвигает левый край на
     * `lo - 3 * DAY_MS`, и в неделю перевода часов это 23:00 или 01:00), поэтому
     * точка отсчёта сперва опускается на свою полночь — ровно как `new Date(y, m,
     * d + i)` у веба.
     */
    fun msAtDayOffset(rangeStart: Long, days: Int): Long =
        Calendar.getInstance().apply {
            timeInMillis = dayFloor(rangeStart)
            add(Calendar.DAY_OF_MONTH, days)
        }.timeInMillis

    /**
     * Номер ячейки оси для момента [ms] при начале окна [rangeStart] — число целых
     * местных суток между ними. Округление, а не отбрасывание: сутки перевода часов
     * дают 23 или 25 часов, и обрезка увела бы день в соседнюю ячейку.
     */
    fun dayIndex(ms: Long, rangeStart: Long): Int =
        Math.round((dayFloor(ms) - dayFloor(rangeStart)).toDouble() / DAY_MS).toInt()

    /** Сколько ячеек занимает окно [lo]..[hi] включительно (минимум одна). */
    fun dayCount(lo: Long, hi: Long): Int = maxOf(1, dayIndex(hi, lo) + 1)
}
