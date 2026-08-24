package website.msdnna.tessera.ui.screens

import android.content.Context
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithTag
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextInput
import androidx.test.core.app.ApplicationProvider
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import website.msdnna.tessera.data.repository.HelpRepository
import website.msdnna.tessera.ui.TestTags
import website.msdnna.tessera.ui.theme.TesseraTheme
import website.msdnna.tessera.util.tokenizeHelp

/**
 * The help section end to end inside the app (#2795): the manual comes out of
 * the assets, the navigation lists it, search narrows it, a row opens the
 * article. No backend is involved — that is the point of shipping the content.
 *
 * The screen size is pinned: Robolectric's default 320×470px would put the lower
 * rows off the visible area, and a tap below the edge is dropped in silence
 * rather than failing with something that names the cause.
 */
@RunWith(RobolectricTestRunner::class)
@Config(qualifiers = "w411dp-h891dp-xhdpi")
class HelpScreenTest {
    @get:Rule
    val compose = createComposeRule()

    private val repo = HelpRepository(ApplicationProvider.getApplicationContext<Context>().assets)
    private val first = repo.articles().first()

    private fun mount() {
        compose.setContent { TesseraTheme { HelpScreen() } }
    }

    @Test
    fun `the manual is listed straight from the assets`() {
        mount()
        compose.onNodeWithTag(TestTags.HELP_NAV).assertIsDisplayed()
        compose.onNodeWithTag(TestTags.helpRow(first.slug)).assertIsDisplayed()
    }

    @Test
    fun `search narrows the list to the matching article`() {
        mount()
        // A word from this article's own title — whatever the manual is renamed
        // to, the search for it has to reach it.
        val word = tokenizeHelp(first.title).maxByOrNull { it.length }!!
        compose.onNodeWithTag(TestTags.HELP_SEARCH).performTextInput(word)
        compose.onNodeWithTag(TestTags.helpRow(first.slug)).assertIsDisplayed()
    }

    @Test
    fun `a query nobody wrote about finds nothing`() {
        mount()
        compose.onNodeWithTag(TestTags.HELP_SEARCH).performTextInput("криптовалюта")
        compose.onNodeWithTag(TestTags.helpRow(first.slug)).assertDoesNotExist()
    }

    @Test
    fun `tapping a row opens the article`() {
        mount()
        compose.onNodeWithTag(TestTags.HELP_ARTICLE).assertDoesNotExist()
        compose.onNodeWithTag(TestTags.helpRow(first.slug)).performClick()
        awaitArticle()
    }

    @Test
    fun `a slug opens its article straight away`() {
        // What a contextual entry point does — land on the article, not on the
        // list with the reader one tap away.
        compose.setContent { TesseraTheme { HelpScreen(initialSlug = first.slug) } }
        awaitArticle()
    }

    private fun awaitArticle() {
        compose.waitUntil {
            compose.onAllNodesWithTag(TestTags.HELP_ARTICLE).fetchSemanticsNodes().isNotEmpty()
        }
        compose.onNodeWithTag(TestTags.HELP_ARTICLE).assertIsDisplayed()
    }
}
