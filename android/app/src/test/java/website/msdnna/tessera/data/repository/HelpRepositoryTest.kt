package website.msdnna.tessera.data.repository

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import com.google.common.truth.Truth.assertThat
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import website.msdnna.tessera.data.model.HelpArticle

/**
 * Which half of an article the app reads (#2795, #2809). The manual ships every
 * article several ways: the desktop text the site renders, the mobile rewrite
 * next to it, and — since #2809 — the English translation of each. Picking the
 * wrong one is not a visible failure: the screen fills with perfectly readable
 * instructions in the wrong language, or about a mouse the reader does not have.
 */
@RunWith(RobolectricTestRunner::class)
class HelpRepositoryTest {
    private val assets = ApplicationProvider.getApplicationContext<Context>().assets
    private val repo = HelpRepository(assets)

    private fun asset(path: String) = assets.open("help/$path").bufferedReader().use { it.readText() }

    private val languages = listOf("ru", "en")

    @Test
    fun `a mobile rewrite is read from its own file, in every language`() {
        for (lang in languages) {
            val rewritten = repo.articles().map { it.content(lang) }.filter { it.mobileRewrite }
            assertThat(rewritten).isNotEmpty()
            for (c in rewritten) {
                // Compared against the file itself rather than a phrase from the
                // text: the wording is meant to be edited, the choice of file is
                // not. The path carries the language, so this also proves the
                // English body is bundled and reached.
                assertThat(repo.body(c.path)).isEqualTo(stripFrontmatter(asset(c.path)))
            }
        }
    }

    @Test
    fun `an article without a mobile rewrite falls back to the desktop text`() {
        // No such article in the manual today — every topic got a mobile rewrite
        // — so the fallback is asserted against a hand-made entry. It is the path
        // the next article added to docs/help takes before anyone rewrites it.
        val base = repo.articles().first()
        val plain = HelpArticle(slug = "plain-${base.slug}", path = base.path, title = base.title)
        val content = plain.content("ru")

        assertThat(content.mobileRewrite).isFalse()
        assertThat(repo.body(content.path)).isEqualTo(stripFrontmatter(asset(base.path)))
    }

    @Test
    fun `a language with no translation falls back to Russian, flagged`() {
        // An older index, or a language added before the article was translated:
        // the reader gets the Russian original, not a blank page, and the flag is
        // what the screen turns into a note.
        val base = repo.articles().first()
        val ruOnly = HelpArticle(slug = "x", path = base.path, title = "Только по-русски", category = "Раздел")
        val en = ruOnly.content("en")

        assertThat(en.translated).isFalse()
        assertThat(en.title).isEqualTo("Только по-русски")
        assertThat(en.path).isEqualTo(base.path)
    }

    @Test
    fun `a real article is fully translated into English`() {
        // Parity (#2809): every article shown in the app has an English
        // translation, body and all. Switching the language swaps the whole
        // article — title, search text and the file behind it.
        val article = repo.articles().first { it.locales.containsKey("en") }
        val ru = article.content("ru")
        val en = article.content("en")

        assertThat(en.translated).isTrue()
        assertThat(en.text).isNotEqualTo(ru.text)
        assertThat(repo.body(en.path)).isNotNull()
        assertThat(repo.body(en.path)).isNotEqualTo(repo.body(ru.path))
    }

    @Test
    fun `an article scoped to the web is not shown at all`() {
        val webOnly = HelpArticle(slug = "shortcuts", path = "start/first-steps.md", platforms = listOf("web"))
        assertThat(webOnly.onAndroid).isFalse()
        // Everything actually bundled for the app is meant for the app.
        assertThat(repo.articles().none { !it.onAndroid }).isTrue()
    }

    @Test
    fun `an index without the platform field is treated as «both»`() {
        // Assets from an older build: an empty list has to read as «shown
        // everywhere», or an app updated ahead of its content shows an empty
        // help section instead of a slightly stale one.
        assertThat(HelpArticle(slug = "x").onAndroid).isTrue()
    }

    @Test
    fun `a missing file reads as null rather than an empty article`() {
        assertThat(repo.body("start/ghost.md")).isNull()
    }

    private fun stripFrontmatter(raw: String) = Regex("^---\\r?\\n[\\s\\S]*?\\r?\\n---\\r?\\n?").replace(raw, "")
}
