package website.msdnna.tessera.data.model

import com.google.gson.annotations.SerializedName

/**
 * The help-centre index (#2795) — the same `helpIndex.json` the web bundles,
 * built from `docs/help` by `frontend/scripts/build-help-index.mjs` and copied
 * into the APK's assets at build time (see `app/build.gradle`, copyHelpContent).
 *
 * It carries everything except the article bodies: navigation, per-article
 * table of contents, and the search corpus ([HelpArticle.text] is the flattened
 * body). The Markdown itself is read from assets only when an article is opened.
 *
 * Lives in `data/model` on purpose: Gson fills these by reflection and the
 * package is the one `proguard-rules.pro` keeps. A model outside it survives
 * debug and comes back stripped in release — empty articles, no crash.
 */
data class HelpIndex(
    @SerializedName("articles") val articles: List<HelpArticle> = emptyList(),
)

data class HelpArticle(
    @SerializedName("slug") val slug: String = "",
    /** Path of the Markdown file relative to `docs/help` (assets: `help/<path>`). */
    @SerializedName("path") val path: String = "",
    @SerializedName("title") val title: String = "",
    @SerializedName("category") val category: String = "",
    @SerializedName("order") val order: Int = 0,
    @SerializedName("updated") val updated: String = "",
    @SerializedName("keywords") val keywords: List<String> = emptyList(),
    /** Clients the article is written for — `web`, `android`, or both. */
    @SerializedName("platforms") val platforms: List<String> = emptyList(),
    @SerializedName("headings") val headings: List<HelpHeading> = emptyList(),
    /** Flattened body, for search and result excerpts — not for rendering. */
    @SerializedName("text") val text: String = "",
    /** The mobile rewrite of this article, when one exists (`<slug>.android.md`). */
    @SerializedName("android") val android: HelpVariant? = null,
    /**
     * Translations of this article (#2809), keyed by language — `en`, and any
     * language added later. The Russian article always stays on the top level;
     * a translation lives here beside it, never in its place, so an older APK
     * whose index predates a language keeps showing the Russian help rather than
     * an empty screen.
     */
    @SerializedName("locales") val locales: Map<String, HelpLocale> = emptyMap(),
) {
    /**
     * Whether this article belongs in the app at all. An index without the field
     * (an older build's assets) counts as «both» rather than «neither»: showing
     * the desktop text beats an empty help section.
     */
    val onAndroid: Boolean
        get() = platforms.isEmpty() || platforms.contains(PLATFORM_ANDROID)

    /**
     * Collapses the article to what this client renders for [lang]: the
     * translated mobile rewrite when one exists, then the translated web body,
     * then the Russian mobile rewrite, then the Russian web body. Title and
     * category follow the same language. A language with no translation yet
     * falls back to Russian with [HelpContent.translated] false, so the screen
     * can say the article is not translated instead of hiding it.
     */
    fun content(lang: String): HelpContent {
        val loc = if (lang != DEFAULT_LANG) locales[lang] else null
        if (loc != null) {
            val v = loc.android
            return HelpContent(
                slug = slug,
                title = loc.title.ifBlank { title },
                category = loc.category.ifBlank { category },
                path = v?.path ?: loc.path,
                text = v?.text ?: loc.text,
                keywords = v?.keywords ?: loc.keywords,
                headings = v?.headings ?: loc.headings,
                updated = (v?.updated?.takeIf { it.isNotBlank() } ?: loc.updated).ifBlank { updated },
                translated = true,
                mobileRewrite = v != null,
            )
        }
        return HelpContent(
            slug = slug,
            title = title,
            category = category,
            path = android?.path ?: path,
            text = android?.text ?: text,
            keywords = android?.keywords ?: keywords,
            headings = android?.headings ?: headings,
            updated = android?.updated?.takeIf { it.isNotBlank() } ?: updated,
            translated = lang == DEFAULT_LANG,
            mobileRewrite = android != null,
        )
    }

    companion object {
        const val PLATFORM_ANDROID = "android"

        /** The language the manual is authored in; every other one is a translation. */
        const val DEFAULT_LANG = "ru"
    }
}

/**
 * The platform-specific half of an article (#2795): a different body, and with
 * it a different table of contents and search corpus. Title, category and place
 * in the navigation stay with the base article — the two platforms share one
 * manual structure and diverge only in the text.
 */
data class HelpVariant(
    /** Path of the variant's Markdown, relative to `docs/help`. */
    @SerializedName("path") val path: String = "",
    @SerializedName("updated") val updated: String = "",
    @SerializedName("keywords") val keywords: List<String> = emptyList(),
    @SerializedName("headings") val headings: List<HelpHeading> = emptyList(),
    @SerializedName("text") val text: String = "",
)

/**
 * A translation of an article (#2809). It carries the same shape as the base
 * article — its own title/category, web body and (when the mobile client has a
 * rewrite of it) an `android` half — so switching language swaps the whole
 * article, not just its prose.
 */
data class HelpLocale(
    @SerializedName("path") val path: String = "",
    @SerializedName("title") val title: String = "",
    @SerializedName("category") val category: String = "",
    @SerializedName("updated") val updated: String = "",
    @SerializedName("keywords") val keywords: List<String> = emptyList(),
    @SerializedName("headings") val headings: List<HelpHeading> = emptyList(),
    @SerializedName("text") val text: String = "",
    @SerializedName("android") val android: HelpVariant? = null,
)

/**
 * An article collapsed to a single language and platform (#2809) — the fields
 * the screen and the searcher actually use, with the locale/mobile fallbacks
 * already resolved. Not deserialized; produced by [HelpArticle.content].
 */
data class HelpContent(
    val slug: String,
    val title: String,
    val category: String,
    /** Markdown path relative to `docs/help`, unique per language and platform —
     *  so it doubles as the body cache key, and one language cannot serve the
     *  body cached for another. */
    val path: String,
    val text: String,
    val keywords: List<String>,
    val headings: List<HelpHeading>,
    val updated: String,
    /** False when a non-Russian reader is seeing the Russian original because no
     *  translation exists yet — the screen shows a note. */
    val translated: Boolean,
    /** True when [text]/[path] are the mobile rewrite; false when they are the
     *  desktop web body and the reader is warned it describes a mouse. */
    val mobileRewrite: Boolean,
)

data class HelpHeading(
    @SerializedName("id") val id: String = "",
    @SerializedName("text") val text: String = "",
    @SerializedName("level") val level: Int = 2,
)
