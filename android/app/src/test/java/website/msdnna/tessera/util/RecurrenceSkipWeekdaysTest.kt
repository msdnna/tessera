package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import java.util.Calendar
import org.junit.Test
import website.msdnna.tessera.data.model.Recurrence

/**
 * «Не по выходным» не вправе переписывать дни недели, выбранные руками (#2932).
 *
 * Два переключателя отвечают на один и тот же вопрос — в какие дни повторять, — и
 * при расхождении побеждал «не по выходным»: `applySkip` толкал выбранную субботу
 * на понедельник, а следующий шаг считается ОТ СДВИНУТОГО дня, который лежит уже в
 * следующей понедельничной неделе. Там `nextWeekly` находит ту же субботу в пяти
 * днях, а не в четырнадцати: со второго вхождения «каждые 2 недели по субботам»
 * навсегда становилось еженедельным понедельником — не уцелели ни выбранный день,
 * ни интервал.
 *
 * Сервер теперь снимает флаг с недельного правила, назвавшего свои дни
 * (`recur.normalized`, backend/internal/recur/recur.go), и web-сетка это уже
 * зеркалит. Здесь та же арифметика для календаря формы: подсветка обязана обещать
 * ту серию, которую сервер действительно отыграет — тем более что старые правила
 * лежат в базе с обоими полями и приезжают на клиент как есть.
 */
class RecurrenceSkipWeekdaysTest {
    private val day = 24L * 60 * 60 * 1000

    /** Local-zone midnight millis (the zone Recurrence itself works in). */
    private fun localMidnight(y: Int, mo1: Int, d: Int): Long =
        Calendar.getInstance().apply {
            clear()
            set(y, mo1 - 1, d, 9, 0, 0)
        }.timeInMillis

    private fun weekdayOf(millis: Long): Int =
        Calendar.getInstance().apply { timeInMillis = millis }.get(Calendar.DAY_OF_WEEK) - Calendar.SUNDAY

    private fun sat() = localMidnight(2026, 3, 7) // Saturday 2026-03-07
    private fun fri() = localMidnight(2026, 3, 6) // Friday 2026-03-06

    @Test
    fun `keeps the chosen weekend day and the interval`() {
        val rule = Recurrence(freq = "weekly", interval = 2, weekdays = listOf(6), skipWeekends = true)
        var cur = sat()
        repeat(4) {
            val next = nextOccurrence(rule, cur)!!
            assertThat(weekdayOf(next)).isEqualTo(6) // Saturday
            assertThat(next - cur).isEqualTo(14 * day)
            cur = next
        }
    }

    @Test
    fun `still pushes off the weekend when the day is derived, not chosen`() {
        val next = nextOccurrence(Recurrence(freq = "weekly", interval = 1, skipWeekends = true), sat())!!
        assertThat(weekdayOf(next)).isEqualTo(1) // Monday
    }

    @Test
    fun `leaves a non-weekend weekday set exactly where it was`() {
        val rule = Recurrence(freq = "weekly", interval = 2, weekdays = listOf(5), skipWeekends = true)
        val next = nextOccurrence(rule, fri())!!
        assertThat(weekdayOf(next)).isEqualTo(5) // Friday
        assertThat(next - fri()).isEqualTo(14 * day)
    }

    @Test
    fun `leaves daily alone — there is no weekday set to contradict`() {
        val next = nextOccurrence(Recurrence(freq = "daily", interval = 1, skipWeekends = true), fri())!!
        assertThat(weekdayOf(next)).isEqualTo(1) // Monday
    }

    /** Подсветка календаря идёт через тот же шаг — серия обязана остаться субботней. */
    @Test
    fun `occurrence keys stay on the chosen saturday`() {
        val rule = Recurrence(freq = "weekly", interval = 2, weekdays = listOf(6), skipWeekends = true)
        val keys = occurrenceKeys(rule, sat(), 3)
        assertThat(keys).containsExactly("2026-03-07", "2026-03-21", "2026-04-04", "2026-04-18")
    }
}
