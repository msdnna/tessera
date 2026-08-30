package website.msdnna.tessera.ui.screens

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import website.msdnna.tessera.R
import website.msdnna.tessera.data.model.HelpContent
import website.msdnna.tessera.data.repository.HelpRepository
import website.msdnna.tessera.ui.TestTags
import website.msdnna.tessera.ui.components.IonIcon
import website.msdnna.tessera.ui.components.IonIconButton
import website.msdnna.tessera.ui.components.RichContent
import website.msdnna.tessera.ui.components.TTextField
import website.msdnna.tessera.ui.components.clickableNoRipple
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.util.HELP_ASSET_DEFAULT_LANG
import website.msdnna.tessera.util.HelpHit
import website.msdnna.tessera.util.HelpSearcher
import website.msdnna.tessera.util.Ion
import website.msdnna.tessera.util.normalizeLanguage
import website.msdnna.tessera.util.resolveHelpImages

/**
 * Help centre (web `HelpView`, #2795) — the same manual the site shows, read
 * from the APK's assets. Categories over articles; tapping one slides the
 * reader over them, the master/detail shape [DocumentsScreen] uses. Search runs
 * on the device against the bundled index, so it answers while offline.
 *
 * The web page's right-hand table of contents is deliberately not ported: on a
 * phone the article is a single scrolling column, and a floating TOC over it
 * costs more room than it saves.
 */
@Composable
fun HelpScreen(initialSlug: String? = null, onSlugConsumed: () -> Unit = {}) {
    val c = Tessera.colors
    val assets = LocalContext.current.assets
    val repo = remember(assets) { HelpRepository(assets) }
    // The manual follows the profile language (#2809): AppLocale has already put
    // it into the configuration, so the effective locale is the source of truth
    // here too. Titles, categories, the search corpus and the body path are all
    // resolved for it — recomputed when it changes, so switching language in
    // settings re-renders the help without leaving the screen.
    val lang = normalizeLanguage(LocalConfiguration.current.locales[0].language)
    val baseArticles = remember(repo) { repo.articles() }
    val articles = remember(baseArticles, lang) { baseArticles.map { it.content(lang) } }
    val searcher = remember(articles, lang) { HelpSearcher(articles, lang) }
    val assetNames = remember(repo) { repo.assetNames() }

    var query by remember { mutableStateOf("") }
    // The open article is held by slug, not by value, so a language switch while
    // one is open re-resolves it to the new language instead of leaving the old
    // one on screen.
    var openSlug by remember { mutableStateOf<String?>(null) }
    val open = remember(openSlug, articles) { articles.firstOrNull { it.slug == openSlug } }
    var body by remember { mutableStateOf<String?>(null) }
    var loading by remember { mutableStateOf(false) }

    // A deep link (the sidebar's «Помощь» carries none, contextual entry points
    // do) opens straight into an article.
    LaunchedEffect(initialSlug) {
        val slug = initialSlug ?: return@LaunchedEffect
        onSlugConsumed()
        if (articles.any { it.slug == slug }) openSlug = slug
    }

    // Reading an article is a few kilobytes off the asset manager — off the main
    // thread anyway, since it happens on every open of a cold article. Keyed by
    // path, which carries the language, so switching language reloads the body.
    LaunchedEffect(open?.path) {
        val article = open
        if (article == null) {
            body = null
            return@LaunchedEffect
        }
        loading = true
        body = withContext(Dispatchers.IO) { repo.body(article.path) }
        loading = false
    }

    // The reader is an inline overlay, not a Dialog, so Back would otherwise
    // fall through to the nav back-stack.
    BackHandler(enabled = open != null) { openSlug = null }

    Box(Modifier.fillMaxSize().background(c.bg).testTag(TestTags.HELP_NAV)) {
        Column(Modifier.fillMaxSize()) {
            Row(
                Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 10.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                TTextField(
                    value = query,
                    onValueChange = { query = it },
                    placeholder = stringResource(R.string.help_search_placeholder),
                    modifier = Modifier.weight(1f),
                    fieldTag = TestTags.HELP_SEARCH,
                )
            }

            val hits = remember(query, searcher) { searcher.search(query) }
            when {
                articles.isEmpty() -> Empty(stringResource(R.string.help_not_built))

                query.isNotBlank() && hits.isEmpty() -> Empty(stringResource(R.string.common_nothing_found))

                query.isNotBlank() -> LazyColumn(Modifier.fillMaxSize().padding(horizontal = 8.dp)) {
                    items(hits, key = { it.slug }) { hit ->
                        HitRow(hit, onClick = { openSlug = hit.slug })
                    }
                    item { Spacer(Modifier.height(16.dp)) }
                }

                else -> LazyColumn(Modifier.fillMaxSize().padding(horizontal = 8.dp)) {
                    // Categories in index order — the builder already sorted the
                    // articles by category, then by `order`, so the nav follows
                    // the reading order the manual was written in.
                    var previous: String? = null
                    for (article in articles) {
                        if (article.category != previous) {
                            previous = article.category
                            item(key = "cat-${article.category}") { SectionLabel(article.category) }
                        }
                        item(key = article.slug) {
                            ArticleRow(article, onClick = { openSlug = article.slug })
                        }
                    }
                    item { Spacer(Modifier.height(16.dp)) }
                }
            }
        }

        open?.let { article ->
            HelpArticleReader(
                article = article,
                body = body,
                loading = loading,
                dark = c.isDark,
                assetNames = assetNames,
                slugs = articles.map { it.slug },
                onBack = { openSlug = null },
                onOpenSlug = { slug -> if (articles.any { it.slug == slug }) openSlug = slug },
            )
        }
    }
}

@Composable
private fun HelpArticleReader(
    article: HelpContent,
    body: String?,
    loading: Boolean,
    dark: Boolean,
    assetNames: Set<String>,
    slugs: List<String>,
    onBack: () -> Unit,
    onOpenSlug: (String) -> Unit,
) {
    val c = Tessera.colors
    Column(Modifier.fillMaxSize().background(c.surface).testTag(TestTags.HELP_ARTICLE)) {
        Row(
            Modifier.fillMaxWidth().padding(horizontal = 8.dp, vertical = 8.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            IonIconButton(
                Ion.CHEVRON_FORWARD,
                onClick = onBack,
                boxSize = 40.dp,
                modifier = Modifier.graphicsLayer { scaleX = -1f },
            )
            Spacer(Modifier.width(4.dp))
            Text(
                article.title,
                color = c.text1,
                fontSize = 16.sp,
                fontWeight = FontWeight.SemiBold,
                maxLines = 1,
                modifier = Modifier.weight(1f),
            )
        }
        HorizontalDivider(color = c.border)

        // The renderer sizes itself to its content and never scrolls internally
        // (#2781), so the scrolling belongs to the column around it.
        Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(horizontal = 16.dp)) {
            Spacer(Modifier.height(12.dp))
            // A language with no translation yet shows the Russian original with
            // a note (#2809) — an honest label beats a blank page or a silent
            // language switch the reader did not ask for.
            if (!article.translated) {
                HelpNotTranslatedNote()
                Spacer(Modifier.height(12.dp))
            }
            // An article with no mobile rewrite is shown as its desktop text,
            // with a note (#2795). Hiding it would be worse: search still finds
            // it by title, and «found but won't open» reads as a bug where an
            // honest label reads as a gap in the manual.
            if (!article.mobileRewrite) {
                HelpDesktopTextNote()
                Spacer(Modifier.height(12.dp))
            }
            when {
                loading -> Text(stringResource(R.string.common_loading), color = c.text3, fontSize = 14.sp)

                body == null -> Text(
                    stringResource(R.string.help_article_missing),
                    color = c.text3,
                    fontSize = 14.sp,
                )

                else -> {
                    // Screenshots follow the language of the *text* (#2816): on
                    // the Russian fallback the shots stay Russian, so the
                    // pictures never disagree with the prose next to them.
                    val shotLang = if (article.translated) {
                        normalizeLanguage(LocalConfiguration.current.locales[0].language)
                    } else {
                        HELP_ASSET_DEFAULT_LANG
                    }
                    val md = remember(body, dark, assetNames, shotLang) {
                        resolveHelpImages(body, dark, assetNames, shotLang)
                    }
                    RichContent(
                        source = md,
                        helpLinks = true,
                        helpSlugs = slugs,
                        onHelpLink = onOpenSlug,
                    )
                }
            }
            if (article.updated.isNotBlank()) {
                Spacer(Modifier.height(16.dp))
                Text(
                    stringResource(R.string.help_updated, article.updated),
                    color = c.placeholder,
                    fontSize = 12.sp,
                )
            }
            Spacer(Modifier.height(24.dp))
        }
    }
}

/**
 * Shown above an article that has no mobile rewrite: the reader is looking at
 * the desktop manual, and the wording («слева в боковой панели», «перетащите
 * мышью») will not match what is under their thumb.
 *
 * Internal rather than private so the suite can mount it: every article in
 * `docs/help` has a mobile rewrite today, so there is no way to reach this note
 * through the screen itself.
 */
@Composable
internal fun HelpDesktopTextNote() {
    val c = Tessera.colors
    Row(
        Modifier.fillMaxWidth()
            .border(1.dp, c.border, RoundedCornerShape(8.dp))
            .background(c.surfaceAlt, RoundedCornerShape(8.dp))
            .padding(horizontal = 12.dp, vertical = 10.dp)
            .testTag(TestTags.HELP_DESKTOP_NOTE),
        verticalAlignment = Alignment.Top,
    ) {
        IonIcon(Ion.HELP_CIRCLE, size = 16.dp, tint = c.text3)
        Spacer(Modifier.width(8.dp))
        Text(
            stringResource(R.string.help_desktop_note),
            color = c.text3,
            fontSize = 12.sp,
            modifier = Modifier.weight(1f),
        )
    }
}

/**
 * Shown above an article that has no translation for the reader's language yet
 * (#2809): they are looking at the Russian original, and saying so beats a blank
 * page or a language they did not choose. In practice the parity test keeps
 * every shipped article translated, so this is reached only by an older index —
 * hence internal, so the suite can still mount it.
 */
@Composable
internal fun HelpNotTranslatedNote() {
    val c = Tessera.colors
    Row(
        Modifier.fillMaxWidth()
            .border(1.dp, c.border, RoundedCornerShape(8.dp))
            .background(c.surfaceAlt, RoundedCornerShape(8.dp))
            .padding(horizontal = 12.dp, vertical = 10.dp)
            .testTag(TestTags.HELP_NOT_TRANSLATED_NOTE),
        verticalAlignment = Alignment.Top,
    ) {
        IonIcon(Ion.HELP_CIRCLE, size = 16.dp, tint = c.text3)
        Spacer(Modifier.width(8.dp))
        Text(
            stringResource(R.string.help_not_translated),
            color = c.text3,
            fontSize = 12.sp,
            modifier = Modifier.weight(1f),
        )
    }
}

@Composable
private fun SectionLabel(label: String) {
    Text(
        label.uppercase(),
        color = Tessera.colors.text3,
        fontSize = 11.sp,
        fontWeight = FontWeight.SemiBold,
        modifier = Modifier.padding(start = 8.dp, top = 14.dp, bottom = 4.dp),
    )
}

@Composable
private fun ArticleRow(article: HelpContent, onClick: () -> Unit) {
    val c = Tessera.colors
    Row(
        Modifier.fillMaxWidth().clickableNoRipple(onClick = onClick)
            .padding(horizontal = 8.dp, vertical = 12.dp)
            .testTag(TestTags.helpRow(article.slug)),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        IonIcon(Ion.HELP_CIRCLE, size = 18.dp, tint = c.text3)
        Spacer(Modifier.width(10.dp))
        Text(article.title, color = c.text1, fontSize = 15.sp, maxLines = 1, modifier = Modifier.weight(1f))
    }
}

@Composable
private fun HitRow(hit: HelpHit, onClick: () -> Unit) {
    val c = Tessera.colors
    Column(
        Modifier.fillMaxWidth().clickableNoRipple(onClick = onClick)
            .padding(horizontal = 8.dp, vertical = 10.dp)
            .testTag(TestTags.helpRow(hit.slug)),
        verticalArrangement = Arrangement.spacedBy(3.dp),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            IonIcon(Ion.HELP_CIRCLE, size = 18.dp, tint = c.text3)
            Spacer(Modifier.width(10.dp))
            Text(hit.title, color = c.text1, fontSize = 15.sp, maxLines = 1, modifier = Modifier.weight(1f))
        }
        if (hit.excerpt.isNotBlank()) {
            Text(hit.excerpt, color = c.text3, fontSize = 12.sp, maxLines = 2, modifier = Modifier.padding(start = 28.dp))
        }
    }
}

@Composable
private fun Empty(text: String) {
    Box(Modifier.fillMaxSize().padding(32.dp), contentAlignment = Alignment.Center) {
        Text(text, color = Tessera.colors.text3, fontSize = 14.sp)
    }
}
