package website.msdnna.tessera.data.repository

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import com.google.common.truth.Truth.assertThat
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import website.msdnna.tessera.util.helpAssetUrl

/**
 * Guards the help content as the app actually receives it (#2795): through the
 * asset manager, after `HelpAssetsTask` has copied `docs/help` into the APK.
 *
 * That is the point of running it under Robolectric rather than reading the repo
 * off disk — the interesting failure is not a typo in an article (the web suite
 * covers the content itself) but the bundling silently not happening: no assets,
 * no crash, just an empty «Помощь» section.
 */
@RunWith(RobolectricTestRunner::class)
class HelpIndexTest {
    private val repo = HelpRepository(ApplicationProvider.getApplicationContext<Context>().assets)

    private val linkRe = Regex("""]\((/help/[^)#\s]+)""")
    private val imageRe = Regex("""!\[[^]]*]\(([^)\s]+)""")

    @Test
    fun `the index is bundled and not empty`() {
        assertThat(repo.articles()).isNotEmpty()
    }

    @Test
    fun `every article has a title, a category and a file behind it`() {
        for (a in repo.articles()) {
            assertThat(a.title).isNotEmpty()
            assertThat(a.category).isNotEmpty()
            assertThat(repo.body(a)).isNotNull()
        }
    }

    @Test
    fun `slugs are unique`() {
        val slugs = repo.articles().map { it.slug }
        assertThat(slugs.toSet()).hasSize(slugs.size)
    }

    @Test
    fun `heading anchors are unique inside an article`() {
        for (a in repo.articles()) {
            val ids = a.headings.map { it.id }
            assertThat(ids.toSet()).hasSize(ids.size)
        }
    }

    @Test
    fun `frontmatter is stripped from the rendered body`() {
        for (a in repo.articles()) {
            assertThat(repo.body(a)!!.trimStart()).doesNotContain("---\ntitle:")
        }
    }

    @Test
    fun `every screenshot an article references is bundled`() {
        val names = repo.assetNames()
        assertThat(names).isNotEmpty()
        for (a in repo.articles()) {
            val body = repo.body(a) ?: continue
            for (m in imageRe.findAll(body)) {
                val src = m.groupValues[1]
                if (src.startsWith("http") || src.startsWith("//") || src.startsWith("data:")) continue
                assertThat(helpAssetUrl(src, dark = false, names = names)).isNotEmpty()
            }
        }
    }

    @Test
    fun `the mobile rewrite is bundled, not just listed in the index`() {
        // The copy task takes `docs/help/**`, so a variant file that never made
        // it into the APK is the same class of failure as a missing article:
        // nothing crashes, the reader just gets the desktop wording back.
        val withVariant = repo.articles().filter { it.android != null }
        assertThat(withVariant).isNotEmpty()
        for (a in withVariant) {
            assertThat(a.androidPath).isEqualTo(a.android!!.path)
            assertThat(repo.body(a)).isNotNull()
        }
    }

    @Test
    fun `cross-links between articles point at slugs that exist`() {
        val slugs = repo.articles().map { it.slug }.toSet()
        for (a in repo.articles()) {
            val body = repo.body(a) ?: continue
            for (m in linkRe.findAll(body)) {
                val target = m.groupValues[1].removePrefix("/help/")
                assertThat(slugs).contains(target)
            }
        }
    }
}
