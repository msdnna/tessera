package website.msdnna.tessera.data.repository

import android.content.res.AssetManager
import com.google.gson.Gson
import website.msdnna.tessera.data.model.HelpArticle
import website.msdnna.tessera.data.model.HelpIndex

/**
 * The help centre's content (#2795), read from the APK's assets — no API, no
 * database, works offline. `app/build.gradle` copies `docs/help` and the web's
 * `helpIndex.json` into `assets/help/` at build time, so the manual has exactly
 * one source and the app cannot drift from the site.
 *
 * Everything is cached after the first read: the index is parsed once and the
 * articles are a few kilobytes each, so re-opening one costs nothing.
 */
class HelpRepository(private val assets: AssetManager) {
    /** Frontmatter is metadata for the index, not content — strip it before render. */
    private val frontmatter = Regex("^---\\r?\\n[\\s\\S]*?\\r?\\n---\\r?\\n?")

    @Volatile
    private var indexCache: HelpIndex? = null

    @Volatile
    private var namesCache: Set<String>? = null

    private val bodies = HashMap<String, String>()

    /** The parsed index, or an empty one if the assets are missing (a build that
     *  skipped the copy task) — an empty help section beats a crash on open. */
    fun index(): HelpIndex {
        indexCache?.let { return it }
        val parsed = runCatching {
            assets.open("$HELP_DIR/$INDEX_FILE").bufferedReader().use { Gson().fromJson(it, HelpIndex::class.java) }
        }.getOrNull() ?: HelpIndex()
        indexCache = parsed
        return parsed
    }

    fun articles(): List<HelpArticle> = index().articles

    fun bySlug(slug: String): HelpArticle? = articles().firstOrNull { it.slug == slug }

    /** File names bundled under `help/assets` — what [website.msdnna.tessera.util.helpAssetUrl]
     *  checks a screenshot (and its dark twin) against. */
    fun assetNames(): Set<String> {
        namesCache?.let { return it }
        val names = runCatching { assets.list("$HELP_DIR/assets")?.toSet() }.getOrNull().orEmpty()
        namesCache = names
        return names
    }

    /**
     * An article's Markdown, frontmatter stripped. Returns null when the index
     * and the files disagree (the index was not rebuilt, or a file was deleted)
     * so the screen can say so instead of rendering a blank page.
     */
    fun body(article: HelpArticle): String? {
        synchronized(bodies) { bodies[article.slug] }?.let { return it }
        val raw = runCatching {
            assets.open("$HELP_DIR/${article.path}").bufferedReader().use { it.readText() }
        }.getOrNull() ?: return null
        val md = frontmatter.replace(raw, "")
        synchronized(bodies) { bodies[article.slug] = md }
        return md
    }

    private companion object {
        const val HELP_DIR = "help"
        const val INDEX_FILE = "helpIndex.json"
    }
}
