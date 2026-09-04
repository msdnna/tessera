package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import org.junit.Test

/**
 * Сборка строк «какой билд запущен» для истории изменений (#2859).
 *
 * Проверяем ровно то, что делает функция решением, а не рендером: чего в списке
 * НЕ должно быть. Коммита и даты у dev-бэкенда нет, и пустая строка «коммит »
 * выглядела бы поломкой сильнее, чем её отсутствие.
 */
class VersionStampTest {
    private fun lines(stamp: VersionStamp?, formatDate: (String) -> String = { "02.09.2026" }) = versionLines(
        label = "Клиент",
        stamp = stamp,
        commitLabel = { "коммит $it" },
        builtLabel = { "сборка $it" },
        formatDate = formatDate,
    )

    @Test
    fun `a full stamp gives version, commit and build date in that order`() {
        assertThat(lines(VersionStamp("0.75.0", "abc1234", "2026-09-02T01:02:03Z")))
            .containsExactly("Клиент 0.75.0", "коммит abc1234", "сборка 02.09.2026")
            .inOrder()
    }

    @Test
    fun `missing commit and build date drop their lines, not print empty ones`() {
        assertThat(lines(VersionStamp("0.85.1"))).containsExactly("Клиент 0.85.1")
    }

    /** Дату, которую форматтер не разобрал, показываем не сырой ISO-строкой, а никак. */
    @Test
    fun `an unparseable build date is dropped rather than printed raw`() {
        val out = lines(VersionStamp("0.75.0", builtAt = "не-дата"), formatDate = { "" })
        assertThat(out).containsExactly("Клиент 0.75.0")
    }

    /** Сервер не ответил (или ответил офлайн) — блока нет вовсе, а не «Сервер ». */
    @Test
    fun `no stamp and a blank version both yield nothing`() {
        assertThat(lines(null)).isEmpty()
        assertThat(lines(VersionStamp("", "abc1234"))).isEmpty()
    }
}
