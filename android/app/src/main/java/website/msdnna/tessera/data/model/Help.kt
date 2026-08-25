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
) {
    /**
     * Whether this article belongs in the app at all. An index without the field
     * (an older build's assets) counts as «both» rather than «neither»: showing
     * the desktop text beats an empty help section.
     */
    val onAndroid: Boolean
        get() = platforms.isEmpty() || platforms.contains(PLATFORM_ANDROID)

    /** True when the app has to fall back to the desktop text for this article. */
    val desktopOnlyText: Boolean
        get() = android == null

    /** The Markdown this client renders — the mobile rewrite when there is one. */
    val androidPath: String
        get() = android?.path ?: path

    // The search corpus follows the body, not the base article: indexing the
    // desktop text under a mobile article would make words findable that the
    // reader never sees on screen.
    val androidText: String
        get() = android?.text ?: text

    val androidKeywords: List<String>
        get() = android?.keywords ?: keywords

    val androidHeadings: List<HelpHeading>
        get() = android?.headings ?: headings

    val androidUpdated: String
        get() = android?.updated?.takeIf { it.isNotBlank() } ?: updated

    private companion object {
        const val PLATFORM_ANDROID = "android"
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

data class HelpHeading(
    @SerializedName("id") val id: String = "",
    @SerializedName("text") val text: String = "",
    @SerializedName("level") val level: Int = 2,
)
