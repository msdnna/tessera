package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import org.junit.Test

/**
 * Что рисует единственная карточка изменений (#2858) — карточку обновления,
 * историю целиком или ничего.
 *
 * Логика вынесена из ViewModel сюда именно ради этих трёх случаев: приоритет
 * между непрочитанным обновлением и открытой вручную историей — решение
 * поведенческое (кто чей ack пишет), а не вопрос вёрстки.
 */
class ChangelogSheetTest {
    private val all = listOf(
        WhatsNewEntry("0.69.0", "2026-08-17", titleRes = 31, itemsRes = 32),
        WhatsNewEntry("0.70.0", "2026-08-20", titleRes = 21, itemsRes = 22),
        WhatsNewEntry("0.9.0", "2026-05-01", titleRes = 41, itemsRes = 42),
    )
    private val pending = listOf(all[1])

    @Test
    fun `closed history and nothing pending means no sheet at all`() {
        assertThat(changelogSheet(emptyList(), historyOpen = false, all = all)).isNull()
    }

    @Test
    fun `history shows every release newest first, sorted numerically`() {
        val sheet = changelogSheet(emptyList(), historyOpen = true, all = all)!!
        assertThat(sheet.history).isTrue()
        assertThat(sheet.entries.map { it.version }).containsExactly("0.70.0", "0.69.0", "0.9.0").inOrder()
    }

    /**
     * Непрочитанное обновление важнее: именно оно пишет ack'и, и если бы история
     * рисовалась поверх, «Закрыть» на ней человек прочитал бы как «карточку видел» —
     * а следующий запуск показал бы её снова.
     */
    @Test
    fun `a pending update wins over a hand-opened history`() {
        val sheet = changelogSheet(pending, historyOpen = true, all = all)!!
        assertThat(sheet.history).isFalse()
        assertThat(sheet.entries.map { it.version }).containsExactly("0.70.0")
    }

    /**
     * История — это changelog, а не «что вы пропустили»: релиз новее установленной
     * сборки и уже прочитанный релиз в ней остаются (в отличие от [planWhatsNew]).
     */
    @Test
    fun `history ignores acks and the running build`() {
        val sheet = changelogSheet(emptyList(), historyOpen = true, all = all)!!
        assertThat(sheet.entries).hasSize(all.size)
    }
}
