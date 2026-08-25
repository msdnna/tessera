package website.msdnna.tessera.util

/**
 * How a stored `project.icon` / `group.icon` value should be rendered.
 * Mirrors `frontend/src/utils/projectIcons.js`:
 *  - [Curated] — one of the 16 named icons (an ionicons asset name);
 *  - [Svg] — raw `<svg…>` markup (an ionicon or uploaded SVG);
 *  - [Image] — a `data:image/…` raster URL;
 *  - [None] — empty → initials (projects) or a folder (groups).
 */
sealed interface IconKind {
    data class Curated(val ion: String) : IconKind
    data class Svg(val markup: String) : IconKind
    data class Image(val data: String) : IconKind
    data object None : IconKind
}

/** Curated key to a bundled ionicons asset name (see assets/ionicons). */
private val curated: Map<String, String> = mapOf(
    "briefcase" to Ion.BRIEFCASE,
    "home" to Ion.HOME,
    "code" to Ion.CODE,
    "rocket" to Ion.ROCKET,
    "school" to Ion.SCHOOL,
    "cart" to Ion.CART,
    "heart" to Ion.HEART,
    "star" to Ion.STAR,
    "flag" to Ion.FLAG,
    "construct" to Ion.CONSTRUCT,
    "book" to Ion.BOOK,
    "flask" to Ion.FLASK,
    "bulb" to Ion.BULB,
    "game" to Ion.GAME,
    "airplane" to Ion.AIRPLANE,
    "wallet" to Ion.WALLET,
)

/** The curated icon keys (insertion order), for the project/group icon picker. */
val CuratedIconKeys: List<String> = curated.keys.toList()

/** Bundled ionicons asset names (assets/ionicons/<name>.svg). */
object Ion {
    const val MENU = "menu"
    const val CHEVRON_DOWN = "chevron_down"
    const val CHEVRON_FORWARD = "chevron_forward"
    const val ADD = "add"
    const val APPS = "apps"
    const val EXPAND = "expand"
    const val ELLIPSIS_H = "ellipsis_h"
    const val ELLIPSIS_V = "ellipsis_v"
    const val LOGOUT = "logout"
    const val PALETTE = "palette"
    const val GRID = "grid"
    const val FOLDER = "folder"
    const val DRAG = "drag"
    const val CALENDAR = "calendar"
    const val BRANCH = "branch"
    const val CHECK = "check"
    const val ELLIPSE = "ellipse"
    const val CHECK_CIRCLE = "check_circle"
    const val CONTRAST = "contrast"

    /** Column status glyphs (own icon pack, web status-* parity): empty circle /
     *  half-pie / ⅔-pie / filled-check-circle. */
    const val STATUS_TODO = "status_todo"
    const val STATUS_PROGRESS = "status_progress"
    const val STATUS_REVIEW = "status_review"
    const val STATUS_DONE = "status_done"
    const val LIST = "list"
    const val PRICETAGS = "pricetags"
    const val PRICETAG = "pricetag"
    const val SEARCH = "search"
    const val FILTER = "filter"
    const val ALBUMS = "albums"
    const val GIT_BRANCH = "git_branch"
    const val GIT_NETWORK = "git_network"
    const val SORT = "swap"
    const val REFRESH = "refresh"
    const val REPEAT = "repeat"
    const val NOTIFICATIONS = "notifications"
    const val PEOPLE = "people"
    const val PERSON = "person"
    const val PERSON_ADD = "person_add"
    const val RIBBON = "ribbon"
    const val DOCUMENT_TEXT = "document_text"
    const val ALARM = "alarm"
    const val TIME = "time"
    const val IMAGE = "image"
    const val ATTACH = "attach"
    const val DOWNLOAD = "download"
    const val TRASH = "trash"
    const val CLOSE = "close"
    const val ARCHIVE = "archive"
    const val GIT_MERGE = "git_merge"
    const val PENCIL = "pencil"
    const val EYE = "eye"
    const val SEND = "send"
    const val LINK = "link"
    const val GITLAB = "gitlab"
    const val GITHUB = "github"

    const val BRIEFCASE = "briefcase"
    const val HOME = "home"
    const val CODE = "code"
    const val ROCKET = "rocket"
    const val SCHOOL = "school"
    const val CART = "cart"
    const val HEART = "heart"
    const val STAR = "star"
    const val FLAG = "flag"
    const val CONSTRUCT = "construct"
    const val BOOK = "book"
    const val FLASK = "flask"
    const val BULB = "bulb"
    const val GAME = "game"
    const val AIRPLANE = "airplane"
    const val WALLET = "wallet"
    const val SETTINGS = "settings"
    const val SHIELD_CHECKMARK = "shield_checkmark"
    const val HELP_CIRCLE = "help_circle"
}

fun classifyIcon(icon: String?): IconKind {
    if (icon.isNullOrBlank()) return IconKind.None
    val trimmed = icon.trimStart()
    // Strip a leading XML prolog / DOCTYPE (present in some exports, e.g. draw.io)
    // so the `<svg…>` root is detected regardless of the preamble.
    val svgBody = stripSvgPrologue(trimmed)
    return when {
        icon.startsWith("data:image") -> IconKind.Image(icon)
        svgBody.startsWith("<svg") -> IconKind.Svg(sanitizeSvgForAndroid(svgBody))
        curated[icon] != null -> IconKind.Curated(curated.getValue(icon))
        else -> IconKind.None
    }
}

/** Drop a leading `<?xml…?>` declaration and `<!DOCTYPE…>` (+ whitespace) so the
 *  markup starts at the `<svg` root — Coil's SVG sniffer needs `<` first, and our
 *  classifier keys off `<svg`. */
private fun stripSvgPrologue(markup: String): String {
    var s = markup.trimStart()
    while (s.startsWith("<?") || s.startsWith("<!")) {
        val end = s.indexOf('>')
        if (end < 0) break
        s = s.substring(end + 1).trimStart()
    }
    return s
}

private val DRAWIO_HELP_SWITCH_RE =
    Regex("""<switch>(?:(?!</switch>).)*?Text is not SVG(?:(?!</switch>).)*?</switch>""", RegexOption.DOT_MATCHES_ALL)

/**
 * Make browser-oriented SVG markup render faithfully through Coil's AndroidSVG
 * decoder, which lacks features a browser has:
 *  - CSS `light-dark(a, b)` colour function → keep the light variant `a`
 *    (AndroidSVG can't parse it and falls back to black);
 *  - `Helvetica` font-family → `sans-serif` (no Helvetica on Android → serif
 *    fallback; Helvetica is a sans face, so `sans-serif` maps to `Typeface.SANS_SERIF`);
 *  - drop the draw.io "Text is not SVG - cannot display" `<switch>` fallback so it
 *    doesn't render at small sizes.
 */
fun sanitizeSvgForAndroid(markup: String): String =
    replaceLightDark(markup)
        .replace("Helvetica", "sans-serif")
        .replace(DRAWIO_HELP_SWITCH_RE, "")

/** Replace every `light-dark(light, dark)` with its light variant. Paren-aware so
 *  a nested `rgb(…, …, …)` argument (which contains commas) is handled correctly. */
private fun replaceLightDark(s: String): String {
    val marker = "light-dark("
    var idx = s.indexOf(marker)
    if (idx < 0) return s
    val out = StringBuilder()
    var cursor = 0
    while (idx >= 0) {
        out.append(s, cursor, idx)
        val open = idx + marker.length
        // Scan to the matching close paren, tracking depth and the top-level comma.
        var depth = 1
        var i = open
        var commaAt = -1
        while (i < s.length && depth > 0) {
            when (s[i]) {
                '(' -> depth++
                ')' -> depth--
                ',' -> if (depth == 1 && commaAt < 0) commaAt = i
            }
            if (depth == 0) break
            i++
        }
        if (i >= s.length || commaAt < 0) {
            // Malformed — leave the literal untouched and move on.
            out.append(s, idx, open)
            cursor = open
        } else {
            out.append(s.substring(open, commaAt).trim())
            cursor = i + 1 // past the closing ')'
        }
        idx = s.indexOf(marker, cursor)
    }
    out.append(s, cursor, s.length)
    return out.toString()
}
