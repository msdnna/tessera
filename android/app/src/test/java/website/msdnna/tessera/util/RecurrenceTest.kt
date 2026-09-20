package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import java.util.Calendar
import java.util.TimeZone
import org.junit.Test
import website.msdnna.tessera.data.model.Recurrence

class RecurrenceTest {
    /** Local-zone midnight millis for the given calendar date (matches Recurrence's own zone). */
    private fun localMidnight(y: Int, mo1: Int, d: Int): Long =
        Calendar.getInstance().apply {
            clear()
            set(y, mo1 - 1, d, 0, 0, 0)
        }.timeInMillis

    private fun keyOf(millis: Long?) = millis?.let { millisDayKey(it) }

    // ── null / blank rule ────────────────────────────────────────────────────
    @Test
    fun `nextOccurrence null or blank freq is null`() {
        assertThat(nextOccurrence(null, localMidnight(2026, 6, 1))).isNull()
        assertThat(nextOccurrence(Recurrence(freq = ""), localMidnight(2026, 6, 1))).isNull()
        assertThat(nextOccurrence(Recurrence(freq = "bogus"), localMidnight(2026, 6, 1))).isNull()
    }

    // ── daily ────────────────────────────────────────────────────────────────
    @Test
    fun `daily advances by interval`() {
        val from = localMidnight(2026, 6, 10) // Wed
        assertThat(keyOf(nextOccurrence(Recurrence(freq = "daily", interval = 1), from)))
            .isEqualTo("2026-06-11")
        assertThat(keyOf(nextOccurrence(Recurrence(freq = "daily", interval = 3), from)))
            .isEqualTo("2026-06-13")
    }

    @Test
    fun `daily interval floored to 1`() {
        val from = localMidnight(2026, 6, 10)
        assertThat(keyOf(nextOccurrence(Recurrence(freq = "daily", interval = 0), from)))
            .isEqualTo("2026-06-11")
    }

    @Test
    fun `daily with skip weekends jumps over sat and sun`() {
        // 2026-06-12 is Friday → +1 = Saturday → skip to Monday 15th
        val fri = localMidnight(2026, 6, 12)
        assertThat(keyOf(nextOccurrence(Recurrence(freq = "daily", skipWeekends = true), fri)))
            .isEqualTo("2026-06-15")
    }

    // ── weekly ───────────────────────────────────────────────────────────────
    @Test
    fun `weekly without weekdays advances 7 days times interval`() {
        val from = localMidnight(2026, 6, 10)
        assertThat(keyOf(nextOccurrence(Recurrence(freq = "weekly", interval = 1), from)))
            .isEqualTo("2026-06-17")
        assertThat(keyOf(nextOccurrence(Recurrence(freq = "weekly", interval = 2), from)))
            .isEqualTo("2026-06-24")
    }

    @Test
    fun `weekly picks next selected weekday within the same week`() {
        // Wed 2026-06-10; select Mon(1),Fri(5). Next selected later this week = Fri 12th.
        val from = localMidnight(2026, 6, 10)
        val rule = Recurrence(freq = "weekly", weekdays = listOf(1, 5))
        assertThat(keyOf(nextOccurrence(rule, from))).isEqualTo("2026-06-12")
    }

    @Test
    fun `weekly wraps to next week when no later weekday this week`() {
        // Fri 2026-06-12; select only Mon(1). Next = Mon 15th.
        val from = localMidnight(2026, 6, 12)
        val rule = Recurrence(freq = "weekly", weekdays = listOf(1))
        assertThat(keyOf(nextOccurrence(rule, from))).isEqualTo("2026-06-15")
    }

    // ── monthly ──────────────────────────────────────────────────────────────
    @Test
    fun `monthly advances by interval months`() {
        val from = localMidnight(2026, 6, 15)
        assertThat(keyOf(nextOccurrence(Recurrence(freq = "monthly", interval = 1), from)))
            .isEqualTo("2026-07-15")
        assertThat(keyOf(nextOccurrence(Recurrence(freq = "monthly", interval = 8), from)))
            .isEqualTo("2027-02-15")
    }

    @Test
    fun `monthly clamps day to month length`() {
        // Jan 31 + 1 month → Feb 28 (2026 not leap)
        val from = localMidnight(2026, 1, 31)
        assertThat(keyOf(nextOccurrence(Recurrence(freq = "monthly", interval = 1), from)))
            .isEqualTo("2026-02-28")
    }

    @Test
    fun `monthly honours the stored day anchor after a short-month clamp`() {
        // February has parked a "31st" task on the 28th. The anchor is what keeps
        // March from inheriting the clamp.
        val from = localMidnight(2026, 2, 28)
        val rule = Recurrence(freq = "monthly", interval = 1, day = 31)
        assertThat(keyOf(nextOccurrence(rule, from))).isEqualTo("2026-03-31")
    }

    @Test
    fun `monthly without an anchor still follows the due date`() {
        val from = localMidnight(2026, 6, 15)
        assertThat(keyOf(nextOccurrence(Recurrence(freq = "monthly", interval = 1), from)))
            .isEqualTo("2026-07-15")
    }

    @Test
    fun `monthly anchor keeps the whole series on the 31st`() {
        // Without the anchor the series walks backwards: 01-31 → 02-28 → 03-28 → …
        val rule = Recurrence(freq = "monthly", interval = 1, day = 31)
        assertThat(occurrenceKeys(rule, localMidnight(2026, 1, 31), n = 3))
            .containsExactly("2026-01-31", "2026-02-28", "2026-03-31", "2026-04-30")
    }

    // ── yearly ───────────────────────────────────────────────────────────────
    @Test
    fun `yearly advances by interval years`() {
        val from = localMidnight(2026, 6, 15)
        assertThat(keyOf(nextOccurrence(Recurrence(freq = "yearly", interval = 1), from)))
            .isEqualTo("2027-06-15")
    }

    @Test
    fun `yearly leap day clamps to feb 28 in non-leap year`() {
        // 2028 is leap → Feb 29; +1 year → 2029 non-leap → Feb 28
        val from = localMidnight(2028, 2, 29)
        assertThat(keyOf(nextOccurrence(Recurrence(freq = "yearly", interval = 1), from)))
            .isEqualTo("2029-02-28")
    }

    @Test
    fun `yearly honours the stored day and month anchor`() {
        // A Feb 29 task, clamped to the 28th by the non-leap 2029. Three years on,
        // 2032 is leap again and the anchor has to bring the 29th back.
        val from = localMidnight(2029, 2, 28)
        val rule = Recurrence(freq = "yearly", interval = 3, day = 29, month = 2)
        assertThat(keyOf(nextOccurrence(rule, from))).isEqualTo("2032-02-29")
    }

    // ── withStoredAnchor ─────────────────────────────────────────────────────
    @Test
    fun `withStoredAnchor lends the anchor back to a rule rebuilt from the controls`() {
        val stored = Recurrence(freq = "monthly", interval = 1, day = 31)
        val rebuilt = Recurrence(freq = "monthly", interval = 1)
        assertThat(withStoredAnchor(rebuilt, stored, dueChanged = false)?.day).isEqualTo(31)

        val storedYearly = Recurrence(freq = "yearly", interval = 1, day = 29, month = 2)
        val rebuiltYearly = Recurrence(freq = "yearly", interval = 1)
        val out = withStoredAnchor(rebuiltYearly, storedYearly, dueChanged = false)
        assertThat(out?.day).isEqualTo(29)
        assertThat(out?.month).isEqualTo(2)
    }

    @Test
    fun `withStoredAnchor drops the anchor when the due date or frequency changed`() {
        val stored = Recurrence(freq = "monthly", interval = 1, day = 31)
        // Moving the due date re-derives the anchor server-side — don't pin the old one.
        assertThat(withStoredAnchor(Recurrence(freq = "monthly"), stored, dueChanged = true)?.day)
            .isEqualTo(0)
        // A different frequency has a different anchor entirely.
        assertThat(withStoredAnchor(Recurrence(freq = "yearly"), stored, dueChanged = false)?.day)
            .isEqualTo(0)
        // Nothing to lend from.
        assertThat(withStoredAnchor(Recurrence(freq = "monthly"), null, dueChanged = false)?.day)
            .isEqualTo(0)
        // Frequencies without an anchor pass through untouched.
        val weekly = Recurrence(freq = "weekly", weekdays = listOf(1))
        assertThat(withStoredAnchor(weekly, stored, dueChanged = false)).isEqualTo(weekly)
        assertThat(withStoredAnchor(null, stored, dueChanged = false)).isNull()
    }

    // ── custom ───────────────────────────────────────────────────────────────
    @Test
    fun `custom picks first date strictly after from`() {
        val from = localMidnight(2026, 6, 10)
        val rule = Recurrence(freq = "custom", dates = listOf("2026-06-10", "2026-06-20", "2026-07-01"))
        assertThat(keyOf(nextOccurrence(rule, from))).isEqualTo("2026-06-20")
    }

    @Test
    fun `custom returns null when no later date`() {
        val from = localMidnight(2026, 8, 1)
        val rule = Recurrence(freq = "custom", dates = listOf("2026-06-20"))
        assertThat(nextOccurrence(rule, from)).isNull()
    }

    // ── occurrenceKeys ───────────────────────────────────────────────────────
    @Test
    fun `occurrenceKeys null inputs empty`() {
        assertThat(occurrenceKeys(null, 123L)).isEmpty()
        assertThat(occurrenceKeys(Recurrence(freq = "daily"), null)).isEmpty()
    }

    @Test
    fun `occurrenceKeys custom returns its dates`() {
        val rule = Recurrence(freq = "custom", dates = listOf("2026-06-10", "2026-06-20"))
        assertThat(occurrenceKeys(rule, localMidnight(2026, 6, 1)))
            .containsExactly("2026-06-01", "2026-06-10", "2026-06-20")
    }

    // occurrenceKeys отвечает на вопрос «в какие дни задача сработает», а не «что
    // выбрано в списке»: сервер берёт первую дату строго позже срока, так что
    // пройденные даты не сработают уже никогда, а `once` оставляет ровно одну.
    // Отдавать сюда весь выбранный список — обещать срабатывания, которых не будет;
    // сам список подсвечивает редактор отдельно (#2932).
    @Test
    fun `occurrenceKeys custom drops the dates the series has walked past`() {
        val rule = Recurrence(
            freq = "custom",
            dates = listOf("2026-03-01", "2026-03-08", "2026-03-15", "2026-03-22"),
        )
        assertThat(occurrenceKeys(rule, localMidnight(2026, 3, 15)))
            .containsExactly("2026-03-15", "2026-03-22")
    }

    @Test
    fun `occurrenceKeys custom once stops after the single firing`() {
        val rule = Recurrence(
            freq = "custom",
            once = true,
            dates = listOf("2026-03-01", "2026-03-08", "2026-03-15", "2026-03-22"),
        )
        assertThat(occurrenceKeys(rule, localMidnight(2026, 3, 1)))
            .containsExactly("2026-03-01", "2026-03-08")
    }

    @Test
    fun `occurrenceKeys once yields current plus one`() {
        val from = localMidnight(2026, 6, 10)
        val rule = Recurrence(freq = "daily", once = true)
        assertThat(occurrenceKeys(rule, from)).containsExactly("2026-06-10", "2026-06-11")
    }

    @Test
    fun `occurrenceKeys generates n occurrences`() {
        val from = localMidnight(2026, 6, 10)
        val rule = Recurrence(freq = "daily")
        val keys = occurrenceKeys(rule, from, n = 3)
        assertThat(keys).containsExactly("2026-06-10", "2026-06-11", "2026-06-12", "2026-06-13")
    }

    // ── перевод часов ────────────────────────────────────────────────────────
    // Правило — календарное утверждение, а сутки с переводом часов не равны 24
    // часам. Зона фиксируется в самом тесте: хост сидит в UTC, где перевода нет
    // вовсе, и любая проверка этого класса ошибок выродилась бы в тавтологию.

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

    private fun hourOf(millis: Long?): Int? =
        millis?.let { Calendar.getInstance().apply { timeInMillis = it }.get(Calendar.HOUR_OF_DAY) }

    @Test
    fun `skip weekends steps calendar days when clocks go back`() = inZone("Europe/Berlin") {
        // 2026-10-25 — воскресенье перевода часов назад, в нём 25 часов.
        val rule = Recurrence(freq = "daily", skipWeekends = true)

        // Пятница 23-го → суббота 24-го → понедельник 26-го (миллисекундами выходило
        // воскресенье 25-е 23:00 — выходной, запрещённый самим правилом).
        val fromFri = nextOccurrence(rule, localMidnight(2026, 10, 23))
        assertThat(keyOf(fromFri)).isEqualTo("2026-10-26")
        assertThat(hourOf(fromFri)).isEqualTo(0)

        // Суббота 24-го → воскресенье 25-е → понедельник 26-й (было: то же 25-е).
        val fromSat = nextOccurrence(rule, localMidnight(2026, 10, 24))
        assertThat(keyOf(fromSat)).isEqualTo("2026-10-26")
        assertThat(hourOf(fromSat)).isEqualTo(0)
    }

    @Test
    fun `skip weekends keeps the clock when clocks go forward`() = inZone("Europe/Berlin") {
        // 2026-03-29 — воскресенье перевода часов вперёд, в нём 23 часа: +48 ч от
        // субботы 28-го уводили занятие с 22:00 на 23:00.
        val fri = Calendar.getInstance().apply {
            clear()
            set(2026, Calendar.MARCH, 27, 22, 0, 0)
        }.timeInMillis
        val next = nextOccurrence(Recurrence(freq = "daily", skipWeekends = true), fri)
        assertThat(keyOf(next)).isEqualTo("2026-03-30")
        assertThat(hourOf(next)).isEqualTo(22)
    }

    // ── мёртвая колонка-триггер (#2932) ──
    // Сервер сверяет id дословно, так что не совпадает ни пустая колонка, ни
    // выбывшая с доски: правило хранится, бейдж рисуется, срабатываний нет.

    private val board = listOf("col-1", "col-2")

    @Test
    fun `the column trigger without a column can never fire`() {
        val rule = Recurrence(freq = "daily", trigger = "column")
        assertThat(triggerColumnMissing(rule, board)).isTrue()
        assertThat(triggerColumnMissing(rule.copy(triggerColumn = ""), board)).isTrue()
    }

    @Test
    fun `a column that has left the board can never fire`() {
        val rule = Recurrence(freq = "daily", trigger = "column", triggerColumn = "col-gone")
        assertThat(triggerColumnMissing(rule, board)).isTrue()
        assertThat(triggerColumnMissing(rule.copy(triggerColumn = "col-2"), board)).isFalse()
    }

    @Test
    fun `the other triggers are not judged by a column`() {
        assertThat(triggerColumnMissing(Recurrence(freq = "daily"), board)).isFalse()
        assertThat(triggerColumnMissing(Recurrence(freq = "daily", trigger = "schedule"), board)).isFalse()
        assertThat(triggerColumnMissing(Recurrence(freq = "", trigger = "column"), board)).isFalse()
        assertThat(triggerColumnMissing(null, board)).isFalse()
    }

    @Test
    fun `an unknown board only judges the unset column`() {
        // Пустой список колонок значит «доску не видно» — это не «колонки больше нет».
        val gone = Recurrence(freq = "daily", trigger = "column", triggerColumn = "col-gone")
        assertThat(triggerColumnMissing(gone, emptyList())).isFalse()
        assertThat(triggerColumnMissing(gone.copy(triggerColumn = null), emptyList())).isTrue()
    }
}
