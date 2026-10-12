package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import org.junit.Test

/**
 * Killer-фича «колонки = теги»: заголовок дорожки — то же, что показывает веб
 * (`tagColumnName`), а не сырое «сложность::высокая» (#2932).
 */
class TagLaneTitleTest {
    @Test
    fun `scoped tag spells its scope out when no prefix is chosen`() {
        assertThat(tagLaneTitle("сложность::высокая")).isEqualTo("сложность: высокая")
        assertThat(tagLaneTitle("T: bug")).isEqualTo("T: bug")
    }

    @Test
    fun `chosen prefix leaves the value alone`() {
        // Пространство имён уже названо подписью группировки — дорожки различает значение.
        assertThat(tagLaneTitle("сложность::высокая", prefixSelected = true)).isEqualTo("высокая")
        assertThat(tagLaneTitle("T: bug", prefixSelected = true)).isEqualTo("bug")
    }

    @Test
    fun `friendly prefix name titles the lane`() {
        val names = mapOf("сложность::" to "Сложность")
        assertThat(tagLaneTitle("сложность::высокая", names)).isEqualTo("Сложность: высокая")
        // «Короткие префиксы» — тот же тумблер, что у пилюль на карточках.
        assertThat(tagLaneTitle("сложность::высокая", names, raw = true)).isEqualTo("сложность: высокая")
    }

    @Test
    fun `a tag without a usable scope keeps its whole name`() {
        assertThat(tagLaneTitle("срочно")).isEqualTo("срочно")
        // Половинчатые имена разбору не поддаются — теряться им нельзя.
        assertThat(tagLaneTitle("сложность::")).isEqualTo("сложность::")
        assertThat(tagLaneTitle("::высокая")).isEqualTo("::высокая")
        assertThat(tagLaneTitle("срочно", prefixSelected = true)).isEqualTo("срочно")
    }
}
