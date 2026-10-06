package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import java.util.Calendar
import java.util.TimeZone
import org.junit.Test
import website.msdnna.tessera.ui.screens.buildDayCells
import website.msdnna.tessera.ui.screens.buildMonthBands

/**
 * Ось дней таймлайна/ганта. Зона фиксируется в самом тесте: хост сидит в UTC, где
 * перевода часов нет вовсе, и любая проверка этого класса ошибок выродилась бы в
 * тавтологию. Europe/Berlin переводит часы назад 2026-10-25 (в сутках 25 часов) и
 * вперёд 2026-03-29 (23 часа).
 */
class TimeAxisTest {
    /** Выполняет [body] с подменённой зоной устройства, возвращая прежнюю. */
    private fun inZone(id: String, body: () -> Unit) {
        val prev = TimeZone.getDefault()
        TimeZone.setDefault(TimeZone.getTimeZone(id))
        try {
            body()
        } finally {
            TimeZone.setDefault(prev)
        }
    }

    private fun localMidnight(y: Int, mo1: Int, d: Int): Long =
        Calendar.getInstance().apply {
            clear()
            set(y, mo1 - 1, d, 0, 0, 0)
        }.timeInMillis

    private fun domOf(ms: Long): Int =
        Calendar.getInstance().apply { timeInMillis = ms }.get(Calendar.DAY_OF_MONTH)

    private val weekdays = listOf("Пн", "Вт", "Ср", "Чт", "Пт", "Сб", "Вс")
    private val months =
        listOf("янв", "фев", "мар", "апр", "май", "июн", "июл", "авг", "сен", "окт", "ноя", "дек")

    // ── шаг по оси ───────────────────────────────────────────────────────────

    @Test
    fun `ячейки оси идут календарными сутками, когда часы переводят назад`() = inZone("Europe/Berlin") {
        val start = localMidnight(2026, 10, 23) // пятница до перевода
        val walk = (0..4).map { domOf(TimeAxis.msAtDayOffset(start, it)) }
        assertThat(walk).containsExactly(23, 24, 25, 26, 27).inOrder()

        // Ровно та арифметика, от которой уходим: 25-е длится 25 часов, поэтому
        // «+ 3 × 24 часа» попадает на 23:00 всё того же 25-го.
        assertThat(domOf(start + 3 * TimeAxis.DAY_MS)).isEqualTo(25)
    }

    @Test
    fun `номер ячейки округляется, когда сутки короче 24 часов`() = inZone("Europe/Berlin") {
        val start = localMidnight(2026, 3, 27) // пятница до перевода вперёд
        val target = localMidnight(2026, 3, 30) // понедельник после него

        assertThat(TimeAxis.dayIndex(target, start)).isEqualTo(3)
        assertThat(TimeAxis.dayCount(start, target)).isEqualTo(4)

        // Прежнее целочисленное деление: 71 час / 24 = 2, полоса и линия «сегодня»
        // вставали в соседнюю ячейку.
        assertThat(((target - start) / TimeAxis.DAY_MS).toInt()).isEqualTo(2)
    }

    @Test
    fun `начало окна опускается на свою полночь`() = inZone("Europe/Berlin") {
        // Левый край окна отодвинут на трое суток миллисекундами (как в веб-оригинале),
        // поэтому сам по себе он не полночь — 23-е, 01:00.
        val lo = localMidnight(2026, 10, 26) - 3 * TimeAxis.DAY_MS
        assertThat(TimeAxis.msAtDayOffset(lo, 0)).isEqualTo(localMidnight(2026, 10, 23))
        assertThat(TimeAxis.dayIndex(localMidnight(2026, 10, 26), lo)).isEqualTo(3)
    }

    @Test
    fun `в зоне без перевода часов ничего не меняется`() = inZone("Europe/Moscow") {
        val start = localMidnight(2026, 10, 23)
        for (i in 0..9) {
            assertThat(TimeAxis.msAtDayOffset(start, i)).isEqualTo(start + i * TimeAxis.DAY_MS)
        }
        assertThat(TimeAxis.dayCount(start, localMidnight(2026, 11, 1))).isEqualTo(10)
    }

    // ── шапка оси ────────────────────────────────────────────────────────────

    @Test
    fun `подписи дней переживают перевод часов назад`() = inZone("Europe/Berlin") {
        val start = localMidnight(2026, 10, 23)
        val cells = buildDayCells(start, 5, localMidnight(2026, 10, 26), weekdays)

        assertThat(cells.map { it.dom }).containsExactly(23, 24, 25, 26, 27).inOrder()
        assertThat(cells.map { it.weekday }).containsExactly("Пт", "Сб", "Вс", "Пн", "Вт").inOrder()
        assertThat(cells.map { it.weekend }).containsExactly(false, true, true, false, false).inOrder()
        assertThat(cells.map { it.isToday }).containsExactly(false, false, false, true, false).inOrder()
    }

    @Test
    fun `полоса месяца рвётся на настоящей границе месяца`() = inZone("Europe/Berlin") {
        // 23 октября … 3 ноября: девять октябрьских ячеек и три ноябрьских. Пока шаг
        // был миллисекундным, вся часть оси правее 25-го числа съезжала на день назад
        // и октябрю доставалась лишняя ячейка.
        val bands = buildMonthBands(localMidnight(2026, 10, 23), 12, months)

        assertThat(bands.map { it.label }).containsExactly("окт 2026", "ноя 2026").inOrder()
        assertThat(bands.map { it.span }).containsExactly(9, 3).inOrder()
        assertThat(bands.map { it.startIdx }).containsExactly(0, 9).inOrder()
    }
}
