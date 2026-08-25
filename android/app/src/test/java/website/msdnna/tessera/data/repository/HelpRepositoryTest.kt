package website.msdnna.tessera.data.repository

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import com.google.common.truth.Truth.assertThat
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import website.msdnna.tessera.data.model.HelpArticle

/**
 * Which half of an article the app reads (#2795). The manual ships both: the
 * desktop text the site renders and, next to it, the mobile rewrite. Picking the
 * wrong one is not a visible failure — the screen fills with perfectly readable
 * instructions about a mouse and a left-hand sidebar the reader does not have.
 */
@RunWith(RobolectricTestRunner::class)
class HelpRepositoryTest {
    private val assets = ApplicationProvider.getApplicationContext<Context>().assets
    private val repo = HelpRepository(assets)

    private fun asset(path: String) = assets.open("help/$path").bufferedReader().use { it.readText() }

    @Test
    fun `an article with a mobile rewrite is read from it`() {
        val withVariant = repo.articles().filter { it.android != null }
        assertThat(withVariant).isNotEmpty()
        for (a in withVariant) {
            val body = repo.body(a)!!
            // Compared against the files themselves rather than a phrase from the
            // text: the wording is meant to be edited, the choice of file is not.
            assertThat(body).isEqualTo(stripFrontmatter(asset(a.android!!.path)))
            assertThat(body).isNotEqualTo(stripFrontmatter(asset(a.path)))
        }
    }

    @Test
    fun `an article without one falls back to the desktop text`() {
        // No such article in the manual today — every topic got a mobile rewrite
        // — so the fallback is asserted against a hand-made entry. It is the path
        // the next article added to docs/help takes before anyone rewrites it.
        val base = repo.articles().first()
        val plain = HelpArticle(slug = "plain-${base.slug}", path = base.path, title = base.title)

        assertThat(plain.desktopOnlyText).isTrue()
        assertThat(repo.body(plain)).isEqualTo(stripFrontmatter(asset(base.path)))
    }

    @Test
    fun `an article scoped to the web is not shown at all`() {
        val webOnly = HelpArticle(slug = "shortcuts", path = "start/first-steps.md", platforms = listOf("web"))
        assertThat(webOnly.onAndroid).isFalse()
        // Everything actually bundled is meant for the app.
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
        assertThat(repo.body(HelpArticle(slug = "ghost", path = "start/ghost.md"))).isNull()
    }

    @Test
    fun `updated date follows the text being shown`() {
        for (a in repo.articles().filter { it.android != null }) {
            assertThat(a.androidUpdated).isEqualTo(a.android!!.updated.ifBlank { a.updated })
        }
    }

    private fun stripFrontmatter(raw: String) = Regex("^---\\r?\\n[\\s\\S]*?\\r?\\n---\\r?\\n?").replace(raw, "")
}
