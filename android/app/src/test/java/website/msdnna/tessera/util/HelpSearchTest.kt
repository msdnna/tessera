package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import org.junit.Test
import website.msdnna.tessera.data.model.HelpArticle
import website.msdnna.tessera.data.model.HelpHeading
import website.msdnna.tessera.data.model.HelpLocale
import website.msdnna.tessera.data.model.HelpVariant

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

    // The searcher works on articles already collapsed to one language (#2809);
    // `content("ru")` reproduces the Russian rendering these cases assert.
    private val search = HelpSearcher(articles.map { it.content("ru") }, "ru")

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

/**
 * Search over an article that has a mobile rewrite (#2795). The corpus has to
 * follow the body this client renders: indexing the desktop text would make the
 * app find articles by words about a mouse and a sidebar — and miss the words
 * the reader can actually see on the screen in front of them.
 */
class HelpSearchVariantTest {
    private val boards = HelpArticle(
        slug = "boards",
        title = "Доски и задачи",
        category = "Работа",
        keywords = listOf("канбан", "мышь"),
        headings = listOf(HelpHeading("перенос", "Перенос мышью", 2)),
        text = "карточка переносится между колонками перетаскиванием мышью",
        android = HelpVariant(
            path = "boards/boards.android.md",
            keywords = listOf("канбан", "палец"),
            headings = listOf(HelpHeading("perenos", "Как перенести задачу", 2)),
            text = "карточка переносится долгим тапом или сменой колонки в экране задачи",
        ),
        locales = mapOf(
            "en" to HelpLocale(
                title = "Boards and tasks",
                category = "Working with tasks",
                android = HelpVariant(
                    path = "boards/boards.android.en.md",
                    keywords = listOf("kanban", "finger"),
                    headings = listOf(HelpHeading("move", "How to move a task", 2)),
                    text = "a card moves with a long press of a finger or by changing its column",
                ),
            ),
        ),
    )

    private val search = HelpSearcher(listOf(boards).map { it.content("ru") }, "ru")

    @Test
    fun `a word only the mobile text uses is findable`() {
        assertThat(search.search("тапом").map { it.slug }).containsExactly("boards")
    }

    @Test
    fun `a word only the desktop text uses is not`() {
        assertThat(search.search("перетаскиванием")).isEmpty()
    }

    @Test
    fun `keywords and headings come from the variant too`() {
        assertThat(search.search("палец").map { it.slug }).containsExactly("boards")
        assertThat(search.search("перенести").map { it.slug }).containsExactly("boards")
        assertThat(search.search("мышь")).isEmpty()
    }

    @Test
    fun `the excerpt quotes the text the reader will see`() {
        assertThat(search.search("тапом").first().excerpt).contains("тапом")
    }

    @Test
    fun `an article without a variant keeps its own text`() {
        val plain = HelpArticle(slug = "faq", title = "Частые вопросы", text = "версия видна в настройках")
        assertThat(HelpSearcher(listOf(plain).map { it.content("ru") }).search("версия").map { it.slug })
            .containsExactly("faq")
    }

    @Test
    fun `the English rendering is indexed and searched in English`() {
        // A translated article (#2809): searching the English body finds it, and
        // the Russian body no longer does — the corpus follows the language the
        // reader is in.
        val en = HelpSearcher(listOf(boards.content("en")), "en")
        assertThat(en.search("finger").map { it.slug }).containsExactly("boards")
        assertThat(en.search("тапом")).isEmpty()
    }
}
