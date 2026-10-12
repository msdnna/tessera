package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import org.junit.Test

/**
 * Порядок пользовательского текста ([UserTextOrder]) — зеркало веб-коллятора
 * `Intl.Collator(lang, { numeric: true, sensitivity: 'base' })`
 * (`frontend/src/utils/format.js`).
 */
class CollationTest {
    private fun sorted(vararg items: String): List<String> = items.toList().sortedWith(UserTextOrder)

    /** `numeric: true` на вебе: число в имени сравнивается по величине, а не посимвольно.
     *  Голый java.text.Collator ставит «спринт/10» перед «спринт/2». */
    @Test
    fun `digits compare by value, not character by character`() {
        assertThat(sorted("спринт/10", "спринт/2", "спринт/1"))
            .containsExactly("спринт/1", "спринт/2", "спринт/10")
            .inOrder()
        assertThat(sorted("Sprint 100", "Sprint 20", "Sprint 3"))
            .containsExactly("Sprint 3", "Sprint 20", "Sprint 100")
            .inOrder()
    }

    /** Число в середине имени, а не в хвосте: общий префикс сравнивается коллятором,
     *  число — численно, остаток снова коллятором. */
    @Test
    fun `digits inside a name`() {
        assertThat(sorted("этап 10 бета", "этап 2 альфа"))
            .containsExactly("этап 2 альфа", "этап 10 бета")
            .inOrder()
    }

    /** «ё» — U+0451, то есть ПОЗЖЕ «я» (U+044F): сортировка по code unit уводит её
     *  в самый конец списка, коллятор ставит рядом с «е». */
    @Test
    fun `yo sorts next to ye, not after ya`() {
        assertThat(sorted("яблоко", "ёмкость", "апельсин"))
            .containsExactly("апельсин", "ёмкость", "яблоко")
            .inOrder()
    }

    /** Регистр не разрывает алфавит (веб `sensitivity: 'base'`): заглавная кириллица
     *  в code-unit-порядке идёт перед всей строчной. */
    @Test
    fun `case does not split the alphabet`() {
        assertThat(sorted("срочно", "Баг", "апи"))
            .containsExactly("апи", "Баг", "срочно")
            .inOrder()
    }

    /** Латиница перед кириллицей — как у веб-коллятора. */
    @Test
    fun `latin precedes cyrillic`() {
        assertThat(sorted("бэкенд", "Android", "frontend"))
            .containsExactly("Android", "frontend", "бэкенд")
            .inOrder()
    }

    /** Порядок тотальный: строки, различимые только ведущими нулями или длиной,
     *  не считаются равными — иначе сортировка перестаёт быть детерминированной. */
    @Test
    fun `total order for numerically equal names`() {
        assertThat(UserTextOrder.compare("этап 02", "этап 2")).isNotEqualTo(0)
        assertThat(sorted("этап 2 бис", "этап 2")).containsExactly("этап 2", "этап 2 бис").inOrder()
    }

    /** Цифры не из ASCII BigInteger не разбирает — такое имя должно сортироваться,
     *  а не ронять экран. */
    @Test
    fun `non-ascii digits do not crash the sort`() {
        val arabicIndic = "этап ٢"
        assertThat(sorted(arabicIndic, "этап 2")).hasSize(2)
    }

    /** Пустая строка — обычный минимум, без исключений. */
    @Test
    fun `empty strings are ordered, not special-cased`() {
        assertThat(sorted("b", "", "a")).containsExactly("", "a", "b").inOrder()
    }
}
