package website.msdnna.tessera.util

import java.util.Calendar
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertSame
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Разрыв недели в календарных сетках — зеркало веба (`BoardCalendarView.vue` firstDow,
 * `utils/timeAxis.js` buildWeekBands, `first-day-of-week` у пикеров вехи).
 *
 * До #2932 три сетки Android считали разрыв жёстко на понедельнике, поэтому каждый
 * воскресный кейс здесь красный на старой формуле `(dow + 5) % 7` / `== Calendar.MONDAY`.
 */
class WeekGridTest {

    private val mondayFirst = listOf("Пн", "Вт", "Ср", "Чт", "Пт", "Сб", "Вс")

    @Test
    fun `week opens on Monday by default`() {
        assertEquals(Calendar.MONDAY, WeekGrid.firstDayOfWeek(WEEK_START_MONDAY))
        assertEquals(Calendar.SUNDAY, WeekGrid.firstDayOfWeek(WEEK_START_SUNDAY))
    }

    @Test
    fun `an unknown preference degrades to Monday, like the web`() {
        // Преф правится через API: число, которого нет ни в одном календаре, не должно
        // уводить сетку в несуществующий день — веб сужает его тем же `=== 0 ? 0 : 1`.
        assertEquals(Calendar.MONDAY, WeekGrid.firstDayOfWeek(5))
        assertEquals(Calendar.MONDAY, WeekGrid.firstDayOfWeek(-1))
    }

    @Test
    fun `lead counts the blanks before the first of the month`() {
        // 1 сентября 2026 — вторник: при неделе с понедельника одна пустая ячейка,
        // при неделе с воскресенья — две.
        assertEquals(1, WeekGrid.lead(Calendar.TUESDAY, WEEK_START_MONDAY))
        assertEquals(2, WeekGrid.lead(Calendar.TUESDAY, WEEK_START_SUNDAY))
    }

    @Test
    fun `a Sunday first-of-month leads a full row on a Monday week and none on a Sunday one`() {
        // Худший случай старой формулы: воскресенье уезжало в конец недели, и месяц,
        // начинающийся с воскресенья, открывался шестью пустыми ячейками.
        assertEquals(6, WeekGrid.lead(Calendar.SUNDAY, WEEK_START_MONDAY))
        assertEquals(0, WeekGrid.lead(Calendar.SUNDAY, WEEK_START_SUNDAY))
    }

    @Test
    fun `opensWeek marks the day the Gantt band breaks on`() {
        assertTrue(WeekGrid.opensWeek(Calendar.MONDAY, WEEK_START_MONDAY))
        assertFalse(WeekGrid.opensWeek(Calendar.SUNDAY, WEEK_START_MONDAY))
        assertTrue(WeekGrid.opensWeek(Calendar.SUNDAY, WEEK_START_SUNDAY))
        assertFalse(WeekGrid.opensWeek(Calendar.MONDAY, WEEK_START_SUNDAY))
    }

    @Test
    fun `labels rotate Sunday to the front on a Sunday week`() {
        assertEquals(mondayFirst, WeekGrid.labels(mondayFirst, WEEK_START_MONDAY))
        assertEquals(
            listOf("Вс", "Пн", "Вт", "Ср", "Чт", "Пт", "Сб"),
            WeekGrid.labels(mondayFirst, WEEK_START_SUNDAY),
        )
    }

    @Test
    fun `labels leave a broken resource array alone`() {
        // Потерянный перевод не должен ронять экран — подписи уходят как есть.
        val short = listOf("Пн", "Вт")
        assertSame(short, WeekGrid.labels(short, WEEK_START_SUNDAY))
    }

    @Test
    fun `labelIndex stays Monday-based, whatever the preference`() {
        // Подпись КОНКРЕТНОГО дня берётся из неповёрнутого массива: на оси Ганта
        // ячейка знает свой день недели, а не колонку сетки.
        assertEquals(0, WeekGrid.labelIndex(Calendar.MONDAY))
        assertEquals(6, WeekGrid.labelIndex(Calendar.SUNDAY))
    }
}
