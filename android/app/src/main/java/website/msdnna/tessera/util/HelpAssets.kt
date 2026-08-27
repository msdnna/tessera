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

/** The English twin of a shot is `<name>-<light|dark>.en.png` next to the
 *  Russian original (#2816). A language without its own shots simply has no such
 *  file and keeps the Russian one, so the articles go on linking
 *  `board-light.png` and never mention a language. */
private val EXT_RE = Regex("(\\.[a-z0-9]+)$", RegexOption.IGNORE_CASE)

const val HELP_ASSET_DEFAULT_LANG = "ru"

private fun withLang(name: String, lang: String): String =
    if (lang.isEmpty() || lang == HELP_ASSET_DEFAULT_LANG) {
        name
    } else if (EXT_RE.containsMatchIn(name)) {
        EXT_RE.replace(name) { ".$lang" + it.groupValues[1] }
    } else {
        "$name.$lang"
    }

/**
 * Candidate file names in preference order. Theme beats language deliberately: a
 * white screenshot on a dark page hurts to look at, a Russian screenshot in an
 * English article merely reads as untranslated.
 */
private fun candidates(name: String, dark: Boolean, lang: String): List<String> {
    val out = mutableListOf<String>()
    if (dark) {
        val twin = LIGHT_RE.replace(name) { "-dark" + it.groupValues[1] }
        if (twin != name) {
            out += withLang(twin, lang)
            out += twin
        }
    }
    out += withLang(name, lang)
    out += name
    return out
}

/** Markdown image: `![alt](src …)` — only the src is rewritten. */
private val IMAGE_RE = Regex("""(!\[[^]]*]\()([^)\s]+)""")

/**
 * The asset URL for an article's image [src], or an empty string when no such
 * file is bundled. [names] is the set of file names present in `help/assets`.
 */
fun helpAssetUrl(
    src: String,
    dark: Boolean,
    names: Set<String>,
    lang: String = HELP_ASSET_DEFAULT_LANG,
): String {
    val name = src.substringBefore('?').substringBefore('#').substringAfterLast('/')
    val hit = candidates(name, dark, lang).firstOrNull { it in names }
    return if (hit != null) HELP_ASSET_BASE + hit else ""
}

/**
 * Rewrites the src of every relative image in an article's Markdown. External
 * and data URLs are left untouched; so is an unknown local file — it keeps its
 * original src rather than becoming a broken empty one, and the missing-asset
 * case is caught by `HelpIndexTest`, not by silently blanking the picture.
 */
fun resolveHelpImages(
    markdown: String,
    dark: Boolean,
    names: Set<String>,
    lang: String = HELP_ASSET_DEFAULT_LANG,
): String =
    IMAGE_RE.replace(markdown) { m ->
        val src = m.groupValues[2]
        if (EXTERNAL_RE.containsMatchIn(src) || src.startsWith("data:")) {
            m.value
        } else {
            val url = helpAssetUrl(src, dark, names, lang)
            if (url.isEmpty()) m.value else m.groupValues[1] + url
        }
    }
