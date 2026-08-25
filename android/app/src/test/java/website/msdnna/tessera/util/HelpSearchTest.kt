package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import org.junit.Test
import website.msdnna.tessera.data.model.HelpArticle
import website.msdnna.tessera.data.model.HelpHeading

/**
 * The Android half of the help search (#2795), asserted against the same cases
 * as the web suite (`frontend/tests/cx-help-search.spec.js`) — the two
 * implementations have to agree, or a word that finds an article in the browser
 * silently finds nothing in the app.
 *
 * A fixture rather than the real index: these are assertions about ranking
 * rules, and pinning them to article text that is meant to be rewritten would
 * fail the suite on every edit to the docs.
 */
class HelpSearchTest {
    private val articles = listOf(
        HelpArticle(
            slug = "boards",
            title = "Доски и задачи",
            category = "Работа",
            keywords = listOf("канбан", "колонки"),
            headings = listOf(HelpHeading("группировка", "Группировка по тегам", 2)),
            text = "задача переносится между колонками перетаскиванием мышью",
        ),
        HelpArticle(
            slug = "faq",
            title = "Частые вопросы",
            category = "Частые вопросы",
            keywords = listOf("faq"),
            headings = listOf(HelpHeading("тема", "Как включить тёмную тему", 2)),
            text = "доски настраиваются в настройках доски; тёмная тема включается там же",
        ),
    )

    private val search = HelpSearcher(articles)

    @Test
    fun `tokenize splits Cyrillic and digits and lowercases`() {
        assertThat(tokenizeHelp("Доски И Задачи 2026"))
            .containsExactly("доски", "и", "задачи", "2026").inOrder()
    }

    @Test
    fun `tokenize cuts on hyphens and punctuation`() {
        assertThat(tokenizeHelp("QR-код, тест.")).containsExactly("qr", "код", "тест").inOrder()
    }

    @Test
    fun `empty input tokenizes to nothing, null included`() {
        assertThat(tokenizeHelp("")).isEmpty()
        assertThat(tokenizeHelp(null)).isEmpty()
    }

    @Test
    fun `an empty query returns no results`() {
        assertThat(search.search("")).isEmpty()
        assertThat(search.search("   ")).isEmpty()
    }

    @Test
    fun `a prefix is enough — «доск» reaches «Доски и задачи»`() {
        assertThat(search.search("доск").map { it.slug }).contains("boards")
    }

    @Test
    fun `a title match outranks the same word in the body`() {
        // «доски» sits in the first article's title and in the second's text.
        assertThat(search.search("доски").first().slug).isEqualTo("boards")
    }

    @Test
    fun `several words — only articles where all of them matched`() {
        assertThat(search.search("тёмная тема").map { it.slug }).containsExactly("faq")
        assertThat(search.search("тёмная перетаскиванием")).isEmpty()
    }

    @Test
    fun `frontmatter keywords are searchable`() {
        assertThat(search.search("канбан").map { it.slug }).containsExactly("boards")
    }

    @Test
    fun `subheadings are searchable`() {
        assertThat(search.search("группировка").map { it.slug }).containsExactly("boards")
    }

    @Test
    fun `a word nobody wrote finds nothing`() {
        assertThat(search.search("криптовалюта")).isEmpty()
    }

    @Test
    fun `a hit carries title, category and an excerpt`() {
        val hit = search.search("колонками").first()
        assertThat(hit.slug).isEqualTo("boards")
        assertThat(hit.title).isEqualTo("Доски и задачи")
        assertThat(hit.category).isEqualTo("Работа")
        assertThat(hit.excerpt).contains("колонками")
    }

    @Test
    fun `limit is respected`() {
        assertThat(search.search("доски", 1)).hasSize(1)
    }

    @Test
    fun `excerpt lifts the neighbourhood of the match`() {
        val long = "а".repeat(200) + " искомое " + "б".repeat(200)
        val out = helpExcerpt(long, listOf("искомое"))
        assertThat(out).contains("искомое")
        assertThat(out.startsWith("…")).isTrue()
        assertThat(out.endsWith("…")).isTrue()
    }

    @Test
    fun `no match in the body falls back to the opening`() {
        assertThat(helpExcerpt("короткий текст", listOf("заголовочное"))).isEqualTo("короткий текст")
    }

    @Test
    fun `empty text excerpts to an empty string`() {
        assertThat(helpExcerpt("", listOf("что-то"))).isEmpty()
    }
}
