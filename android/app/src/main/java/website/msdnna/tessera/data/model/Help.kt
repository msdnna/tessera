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
    @SerializedName("headings") val headings: List<HelpHeading> = emptyList(),
    /** Flattened body, for search and result excerpts — not for rendering. */
    @SerializedName("text") val text: String = "",
)

data class HelpHeading(
    @SerializedName("id") val id: String = "",
    @SerializedName("text") val text: String = "",
    @SerializedName("level") val level: Int = 2,
)
