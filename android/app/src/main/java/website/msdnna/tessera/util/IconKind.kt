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
    const val LIST = "list"
    const val PRICETAGS = "pricetags"
    const val PRICETAG = "pricetag"
    const val SEARCH = "search"
    const val FILTER = "filter"
    const val ALBUMS = "albums"
    const val GIT_BRANCH = "git_branch"
    const val SORT = "swap"
    const val REFRESH = "refresh"
    const val NOTIFICATIONS = "notifications"
    const val PEOPLE = "people"
    const val PERSON_ADD = "person_add"
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
}

fun classifyIcon(icon: String?): IconKind {
    if (icon.isNullOrBlank()) return IconKind.None
    val trimmed = icon.trimStart()
    return when {
        icon.startsWith("data:image") -> IconKind.Image(icon)
        trimmed.startsWith("<svg") -> IconKind.Svg(icon)
        curated[icon] != null -> IconKind.Curated(curated.getValue(icon))
        else -> IconKind.None
    }
}
