package website.msdnna.tessera.data.repository

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import com.google.common.truth.Truth.assertThat
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import website.msdnna.tessera.util.helpAssetUrl

/**
 * Guards the help content as the app actually receives it (#2795, #2809):
 * through the asset manager, after `HelpAssetsTask` has copied `docs/help` into
 * the APK — in both languages the app ships.
 *
 * That is the point of running it under Robolectric rather than reading the repo
 * off disk — the interesting failure is not a typo in an article (the web suite
 * covers the content itself) but the bundling silently not happening: no assets,
 * no crash, just an empty «Помощь» section, or an English reader falling back to
 * Russian because the translation never made it into the APK.
 */
@RunWith(RobolectricTestRunner::class)
class HelpIndexTest {
    private val repo = HelpRepository(ApplicationProvider.getApplicationContext<Context>().assets)

    private val linkRe = Regex("""]\((/help/[^)#\s]+)""")
    private val imageRe = Regex("""!\[[^]]*]\(([^)\s]+)""")
    private val languages = listOf("ru", "en")

    /**
     * Markdown as the reader sees it — code stripped out.
     *
     * An article about the editor quotes the syntax it teaches: `![имя](адрес)`
     * in a backtick span is a specimen, not a picture, and «адрес» is not a file
     * anyone can bundle. The same holds for a fenced block. Only what renders as
     * a real image or a real link is this suite's business.
     */
    private fun prose(markdown: String): String =
        markdown
            .replace(Regex("""(?s)```.*?```"""), "")
            .replace(Regex("""`[^`\n]*`"""), "")

    @Test
    fun `the index is bundled and not empty`() {
        assertThat(repo.articles()).isNotEmpty()
    }

    @Test
    fun `every article has a title, a category and a file behind it, in each language`() {
        for (lang in languages) {
            for (a in repo.articles()) {
                val c = a.content(lang)
                assertThat(c.title).isNotEmpty()
                assertThat(c.category).isNotEmpty()
                assertThat(repo.body(c.path)).isNotNull()
            }
        }
    }

    @Test
    fun `slugs are unique`() {
        val slugs = repo.articles().map { it.slug }
        assertThat(slugs.toSet()).hasSize(slugs.size)
    }

    @Test
    fun `heading anchors are unique inside an article, in each language`() {
        for (lang in languages) {
            for (a in repo.articles()) {
                val ids = a.content(lang).headings.map { it.id }
                assertThat(ids.toSet()).hasSize(ids.size)
            }
        }
    }

    @Test
    fun `frontmatter is stripped from the rendered body`() {
        for (lang in languages) {
            for (a in repo.articles()) {
                assertThat(repo.body(a.content(lang).path)!!.trimStart()).doesNotContain("---\ntitle:")
            }
        }
    }

    @Test
    fun `every screenshot an article references is bundled`() {
        val names = repo.assetNames()
        assertThat(names).isNotEmpty()
        for (lang in languages) {
            for (a in repo.articles()) {
                val body = prose(repo.body(a.content(lang).path) ?: continue)
                for (m in imageRe.findAll(body)) {
                    val src = m.groupValues[1]
                    if (src.startsWith("http") || src.startsWith("//") || src.startsWith("data:")) continue
                    assertThat(helpAssetUrl(src, dark = false, names = names)).isNotEmpty()
                }
            }
        }
    }

    @Test
    fun `the mobile rewrite is bundled, not just listed in the index`() {
        // The copy task takes `docs/help/**`, so a variant file that never made
        // it into the APK is the same class of failure as a missing article:
        // nothing crashes, the reader just gets the desktop wording back.
        for (lang in languages) {
            val rewritten = repo.articles().map { it.content(lang) }.filter { it.mobileRewrite }
            assertThat(rewritten).isNotEmpty()
            for (c in rewritten) {
                assertThat(repo.body(c.path)).isNotNull()
            }
        }
    }

    @Test
    fun `every article shown in the app is translated into English`() {
        // The parity ratchet, Android side (#2809): an article that ships without
        // an English translation would silently fall back to Russian for an
        // English reader. If it has a mobile rewrite, so must its translation.
        for (a in repo.articles()) {
            val en = a.locales["en"]
            assertThat(en).isNotNull()
            if (a.android != null) {
                assertThat(en!!.android).isNotNull()
            }
        }
    }

    @Test
    fun `cross-links between articles point at slugs that exist`() {
        // The manual is one text for both clients, so an article the app ships can
        // legitimately link to one it does not (the admin topics are web-only).
        // What must never happen is a link to nothing at all: a typo'd slug is
        // dead on the site too. `RichContent` keeps the web-only ones as ordinary
        // server links — a tap opens the manual on the site.
        val all = repo.index().articles.map { it.slug }.toSet()
        assertThat(all).isNotEmpty()
        for (lang in languages) {
            for (a in repo.articles()) {
                val body = prose(repo.body(a.content(lang).path) ?: continue)
                for (m in linkRe.findAll(body)) {
                    val target = m.groupValues[1].removePrefix("/help/")
                    assertThat(all).contains(target)
                }
            }
        }
    }
}
