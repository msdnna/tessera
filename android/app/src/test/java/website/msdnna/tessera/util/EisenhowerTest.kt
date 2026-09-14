package website.msdnna.tessera.util

import java.text.SimpleDateFormat
import java.util.Locale
import java.util.TimeZone
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import website.msdnna.tessera.data.model.Task

/**
 * Ось «Срочно» матрицы Эйзенхауэра. Случаи зеркалят
 * `frontend/tests/ut-taskFilter.spec.js` (describe «isDueWithinDays»).
 *
 * До этого окно считалось миллисекундами (`due <= now + 7 * 86_400_000L`), и оно
 * спорило с фасетом «Срок: неделя» на той же доске: у задачи со сроком ровно через
 * семь дней время суток обычно раньше текущего — у date-only срока (UTC-полночь)
 * всегда, — и инстант проскакивал под границу.
 *
 * Каждый случай проверяется в двух зонах по разные стороны от любого раннера —
 * Камчатка (+12) и Гонолулу (−10), — чтобы ответ нельзя было получить, вовсе не
 * читая зону, в какой бы TZ ни шёл прогон.
 */
class EisenhowerTest {
    private fun zone(id: String) = DateFormatPrefs(zoneId = id)
    private val east = zone("Asia/Kamchatka")
    private val west = zone("Pacific/Honolulu")

    private fun millis(iso: String): Long =
        SimpleDateFormat("yyyy-MM-dd'T'HH:mm:ss", Locale.ROOT)
            .apply { timeZone = TimeZone.getTimeZone("UTC") }
            .parse(iso)!!
            .time

    // now = 14:00Z 13 сентября: 14-е 02:00 на Камчатке, 13-е 04:00 в Гонолулу.
    private val nowZ = millis("2026-09-13T14:00:00")

    private fun task(due: String?, priority: Int = 0, pinned: Int? = null) =
        Task(id = "t", title = "t", dueDate = due, priority = priority, eisenhowerQuadrant = pinned)

    @Test
    fun `срок на восьмой день не срочный, хотя инстант влезает в 7 суток`() {
        val due = "2026-09-20T13:00:00Z"
        // Инстант укладывается в now + 7×86_400_000 — старая формула сказала бы «срочно».
        assertTrue(millis("2026-09-20T13:00:00") <= nowZ + 7 * 86_400_000L)
        for (z in listOf(east, west)) {
            assertFalse(isDueWithinDays(due, EISENHOWER_URGENT_DAYS, z, nowZ))
            // И фасет «неделя» рядом на доске отвечает так же — в этом весь смысл.
            val clock = FilterClock(dayKeyIn(z.zone, 0, nowZ), dayKeyIn(z.zone, 7, nowZ), z)
            assertFalse(matchesDue(task(due), DueFilter.Week, clock))
        }
    }

    @Test
    fun `седьмой день окна срочный, и фасет «неделя» с ним согласен`() {
        val due = "2026-09-19T13:00:00Z"
        for (z in listOf(east, west)) {
            assertTrue(isDueWithinDays(due, EISENHOWER_URGENT_DAYS, z, nowZ))
            val clock = FilterClock(dayKeyIn(z.zone, 0, nowZ), dayKeyIn(z.zone, 7, nowZ), z)
            assertTrue(matchesDue(task(due), DueFilter.Week, clock))
        }
    }

    @Test
    fun `date-only срок меряется от «сегодня» в зоне пользователя`() {
        // UTC-полночь — день один и тот же везде, а вот «сегодня» у двоих разное.
        val due = "2026-09-20T00:00:00Z"
        assertTrue(isDueWithinDays(due, EISENHOWER_URGENT_DAYS, east, nowZ)) // сегодня 14-е
        assertFalse(isDueWithinDays(due, EISENHOWER_URGENT_DAYS, west, nowZ)) // сегодня 13-е
    }

    @Test
    fun `перевод часов не сдвигает границу окна`() {
        // 26-е 23_30Z — это уже 27-е 00_30 в Берлине (CET с 25 октября), восьмой день.
        val berlin = zone("Europe/Berlin")
        val now = millis("2026-10-20T08:00:00") // 10:00 по Берлину, CEST
        assertFalse(isDueWithinDays("2026-10-26T23:30:00Z", 7, berlin, now))
        assertTrue(isDueWithinDays("2026-10-26T22:30:00Z", 7, berlin, now))
    }

    @Test
    fun `просроченное срочно, а задача без срока — нет`() {
        for (z in listOf(east, west)) {
            assertTrue(isDueWithinDays("2026-09-01T10:00:00Z", 7, z, nowZ))
            assertFalse(isDueWithinDays(null, 7, z, nowZ))
            assertFalse(isDueWithinDays("", 7, z, nowZ))
            // Ширину окна задаёт вызывающий: с однодневным окном тот же срок не срочен.
            assertFalse(isDueWithinDays("2026-09-19T13:00:00Z", 1, z, nowZ))
        }
    }

    @Test
    fun `квадрант складывается из важности и срочности, закрепление важнее вывода`() {
        val soon = "2026-09-19T13:00:00Z" // внутри окна в обеих зонах
        val later = "2026-09-20T13:00:00Z" // восьмой день в обеих зонах
        for (z in listOf(east, west)) {
            assertEquals(0, quadrantOf(task(soon, priority = 3), z, nowZ))
            assertEquals(1, quadrantOf(task(later, priority = 3), z, nowZ))
            assertEquals(2, quadrantOf(task(soon, priority = 1), z, nowZ))
            assertEquals(3, quadrantOf(task(later, priority = 1), z, nowZ))
            assertEquals(3, quadrantOf(task(null, priority = 0), z, nowZ))
            // Закреплённый квадрант вывод не пересчитывает.
            assertEquals(3, quadrantOf(task(soon, priority = 3, pinned = 3), z, nowZ))
        }
    }
}
