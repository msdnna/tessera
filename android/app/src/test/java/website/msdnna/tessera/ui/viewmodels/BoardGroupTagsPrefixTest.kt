package website.msdnna.tessera.ui.viewmodels

import com.google.common.truth.Truth.assertThat
import org.junit.Test
import website.msdnna.tessera.data.model.Tag

/**
 * Какие теги становятся дорожками, когда доска сужена одним пространством имён (#2932).
 *
 * Меню префиксов строится из `buildTagGroups`, а тот раскладывает теги по КАНОНИЧЕСКОМУ
 * ключу (`canonPrefix(tagNamespace(name))`). Сужение же сравнивало сырую строку —
 * `name.startsWith(tagPrefix)`, — и две половины расходились: «Сложность::высокая» и
 * «сложность::низкая» — одна группа в меню и два разных пространства для дорожек, так
 * что выбор префикса рисовал только теги совпавшего регистра, а карточки остальных
 * уезжали в «Без тегов». Веб это уже починил (`utils/tagGroups.js tagInPrefix`).
 */
class BoardGroupTagsPrefixTest {
    private val hardUpper = Tag(id = "t-1", name = "Сложность::высокая")
    private val hardLower = Tag(id = "t-2", name = "сложность::низкая")
    private val effort = Tag(id = "t-3", name = "effort::small")
    private val effortless = Tag(id = "t-4", name = "effortless::да")
    private val plain = Tag(id = "t-5", name = "urgent")

    private fun state(tagPrefix: String) = BoardUiState(
        tags = listOf(hardUpper, hardLower, effort, effortless, plain).associateBy { it.id },
        groupMode = "tag",
        tagPrefix = tagPrefix,
    )

    private fun names(tagPrefix: String) = state(tagPrefix).groupTags.map { it.name }

    @Test
    fun `no prefix keeps every tag`() {
        assertThat(names("")).containsExactly(
            "Сложность::высокая", "сложность::низкая", "effort::small", "effortless::да", "urgent",
        )
    }

    @Test
    fun `a prefix takes both spellings of its namespace`() {
        assertThat(names("сложность::")).containsExactly("Сложность::высокая", "сложность::низкая")
        // И с другой стороны: выбранный в меню пункт мог прийти в любом написании.
        assertThat(names("Сложность::")).containsExactly("Сложность::высокая", "сложность::низкая")
    }

    @Test
    fun `a prefix does not take a namespace that merely starts the same`() {
        assertThat(names("effort::")).containsExactly("effort::small")
    }

    @Test
    fun `a prefix does not take tags without a namespace`() {
        assertThat(names("urgent")).isEmpty()
    }
}
