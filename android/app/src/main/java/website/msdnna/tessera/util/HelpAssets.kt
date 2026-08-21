package website.msdnna.tessera.util

/**
 * Resolves the screenshots referenced by help articles (#2795) — the Android
 * half of the web `utils/helpAssets.js`.
 *
 * Articles link their shots relatively (`![…](../assets/board-light.png)`), and
 * [website.msdnna.tessera.ui.components.RichContent] renders the Markdown in a
 * WebView based at `file:///android_asset/richcontent/` — so a relative link
 * would resolve to `file:///android_asset/assets/…` and show nothing at all.
 * The rewrite happens on the Markdown here, before it reaches the renderer.
 *
 * Articles live at different depths under `docs/help`, so their relative paths
 * differ («../assets/x.png» vs «../../assets/x.png») while the target is the
 * same single directory — only the file name is used.
 */
const val HELP_ASSET_BASE = "file:///android_asset/help/assets/"

/** A light shot is written as `<name>-light.png` and its dark twin as
 *  `<name>-dark.png` (see `frontend/e2e/shots/help-shots.spec.js`). Articles
 *  link the light one; a reader in the dark theme gets the dark twin, so the
 *  manual never shows a blinding white board on a dark page. Articles with only
 *  one variant (a diagram, say) are left alone. */
private val LIGHT_RE = Regex("-light(\\.[a-z0-9]+)$", RegexOption.IGNORE_CASE)

private val EXTERNAL_RE = Regex("^(https?:)?//")

/** Markdown image: `![alt](src …)` — only the src is rewritten. */
private val IMAGE_RE = Regex("""(!\[[^]]*]\()([^)\s]+)""")

/**
 * The asset URL for an article's image [src], or an empty string when no such
 * file is bundled. [names] is the set of file names present in `help/assets`.
 */
fun helpAssetUrl(src: String, dark: Boolean, names: Set<String>): String {
    val name = src.substringBefore('?').substringBefore('#').substringAfterLast('/')
    if (dark) {
        val twin = LIGHT_RE.replace(name) { "-dark" + it.groupValues[1] }
        if (twin != name && twin in names) return HELP_ASSET_BASE + twin
    }
    return if (name in names) HELP_ASSET_BASE + name else ""
}

/**
 * Rewrites the src of every relative image in an article's Markdown. External
 * and data URLs are left untouched; so is an unknown local file — it keeps its
 * original src rather than becoming a broken empty one, and the missing-asset
 * case is caught by `HelpIndexTest`, not by silently blanking the picture.
 */
fun resolveHelpImages(markdown: String, dark: Boolean, names: Set<String>): String =
    IMAGE_RE.replace(markdown) { m ->
        val src = m.groupValues[2]
        if (EXTERNAL_RE.containsMatchIn(src) || src.startsWith("data:")) {
            m.value
        } else {
            val url = helpAssetUrl(src, dark, names)
            if (url.isEmpty()) m.value else m.groupValues[1] + url
        }
    }
