package website.msdnna.tessera.ui.components

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import com.google.common.truth.Truth.assertThat
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import website.msdnna.tessera.util.withLanguage

/**
 * Строка интервала повтора собирается из трёх кусков: [префикс] [число] [единица].
 * Единица склонялась с самого начала (`recur_unit_*`), а префикс был обычной
 * строкой «каждые» — при интервале 1, то есть сразу после выбора любой частоты,
 * выходило «каждые 1 день» / «каждые 1 неделю». Русскому нужны и число, и род:
 * «каждый день», но «каждую неделю». Английский инвариантен — это негативный
 * контроль, он не должен начать шевелиться заодно с русским.
 *
 * Тест ходит через сами карты пикера ([EveryPlurals] / [UnitPlurals]), а не через
 * `R.plurals.*` напрямую: так он ловит и подмену ресурса в самой раскладке.
 */
@RunWith(RobolectricTestRunner::class)
class RecurrenceLabelsTest {
    private val appContext: Context = ApplicationProvider.getApplicationContext()

    private fun res(language: String) = appContext.withLanguage(language).resources

    private fun prefix(language: String, freq: String, n: Int) =
        res(language).getQuantityString(EveryPlurals.getValue(freq), n)

    private fun phrase(language: String, freq: String, n: Int): String {
        val r = res(language)
        val unit = r.getQuantityString(UnitPlurals.getValue(freq), n)
        return "${prefix(language, freq, n)} $n $unit"
    }

    @Test
    fun `russian prefix agrees with the count`() {
        assertThat(prefix("ru", "daily", 1)).isEqualTo("каждый")
        assertThat(prefix("ru", "daily", 2)).isEqualTo("каждые")
        assertThat(prefix("ru", "daily", 5)).isEqualTo("каждые")
        assertThat(prefix("ru", "daily", 11)).isEqualTo("каждые")
        assertThat(prefix("ru", "daily", 21)).isEqualTo("каждый")
    }

    @Test
    fun `russian prefix agrees with the gender of the unit`() {
        assertThat(prefix("ru", "weekly", 1)).isEqualTo("каждую")
        assertThat(prefix("ru", "weekly", 2)).isEqualTo("каждые")
        assertThat(prefix("ru", "monthly", 1)).isEqualTo("каждый")
        assertThat(prefix("ru", "yearly", 1)).isEqualTo("каждый")
    }

    @Test
    fun `russian interval row reads as a sentence`() {
        assertThat(phrase("ru", "daily", 1)).isEqualTo("каждый 1 день")
        assertThat(phrase("ru", "weekly", 1)).isEqualTo("каждую 1 неделю")
        assertThat(phrase("ru", "weekly", 3)).isEqualTo("каждые 3 недели")
        assertThat(phrase("ru", "monthly", 5)).isEqualTo("каждые 5 месяцев")
        assertThat(phrase("ru", "yearly", 21)).isEqualTo("каждый 21 год")
    }

    @Test
    fun `english prefix stays invariant`() {
        for (freq in listOf("daily", "weekly", "monthly", "yearly")) {
            for (n in listOf(1, 2, 5, 21)) {
                assertThat(prefix("en", freq, n)).isEqualTo("every")
            }
        }
        assertThat(phrase("en", "daily", 1)).isEqualTo("every 1 day")
        assertThat(phrase("en", "weekly", 3)).isEqualTo("every 3 weeks")
    }

    @Test
    fun `both maps cover every frequency with an interval row`() {
        val withInterval = listOf("daily", "weekly", "monthly", "yearly")
        assertThat(EveryPlurals.keys).containsExactlyElementsIn(withInterval)
        assertThat(UnitPlurals.keys).containsExactlyElementsIn(withInterval)
    }
}
