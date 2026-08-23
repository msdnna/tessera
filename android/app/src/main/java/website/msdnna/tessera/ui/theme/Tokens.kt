package website.msdnna.tessera.ui.theme

import androidx.annotation.StringRes
import androidx.compose.ui.graphics.Color
import website.msdnna.tessera.R

/**
 * Neutral palette tokens, ported 1:1 from the web frontend
 * (`frontend/src/styles/tokens.js`). These drive every plain surface — the
 * accent colour is layered on top via [AccentTheme].
 */
data class TesseraColors(
    val bg: Color,
    val surface: Color,
    val surfaceAlt: Color,
    val cardSurface: Color,
    val hover: Color,
    val border: Color,
    val text1: Color,
    val text2: Color,
    val text3: Color,
    val placeholder: Color,
    // Accent — overlaid from the active AccentTheme so components can read a
    // single colour bundle.
    val primary: Color,
    val onPrimary: Color,
    val isDark: Boolean,
)

val LightPalette = TesseraColors(
    bg = Color(0xFFF6F7F9),
    surface = Color(0xFFFFFFFF),
    surfaceAlt = Color(0xFFF3F4F6),
    cardSurface = Color(0xFFFFFFFF),
    hover = Color(0xFFEEF0F3),
    border = Color(0xFFE6E8EC),
    text1 = Color(0xFF1F2329),
    text2 = Color(0xFF4A5059),
    text3 = Color(0xFF868D96),
    placeholder = Color(0xFFB8BDC4),
    primary = Color(0xFF7C5CFF),
    onPrimary = Color(0xFFFFFFFF),
    isDark = false,
)

val DarkPalette = TesseraColors(
    bg = Color(0xFF16161A),
    surface = Color(0xFF1E1E24),
    surfaceAlt = Color(0xFF26262E),
    cardSurface = Color(0xFF1E1E24),
    hover = Color(0xFF2D2D36),
    border = Color(0xFF33333D),
    text1 = Color(0xFFF0F0F3),
    text2 = Color(0xFFD6D6DD),
    text3 = Color(0xFF9A9AA6),
    placeholder = Color(0xFF5C5C66),
    primary = Color(0xFF7C5CFF),
    onPrimary = Color(0xFFFFFFFF),
    isDark = true,
)

/**
 * Accent scheme. Ported from `frontend/src/stores/theme.js`
 * `COLOR_THEMES`. Default = purple.
 */
data class AccentTheme(
    /** Подпись схемы — ресурс, а не строка: список ниже вычисляется один раз при
     *  загрузке класса, готовый текст застыл бы на языке первого обращения. */
    @StringRes val nameRes: Int,
    val key: String,
    val primary: Color,
    val hover: Color,
    val pressed: Color,
    val suppl: Color,
) {
    /** WCAG-relative-luminance pick of readable text on the accent surface. */
    val onPrimary: Color get() = if (relativeLuminance(primary) > 0.3) Color(0xFF1F1F1F) else Color.White
}

val AccentThemes = listOf(
    AccentTheme(R.string.accent_purple, "purple", Color(0xFF7C5CFF), Color(0xFF9277FF), Color(0xFF6344E0), Color(0xFF9277FF)),
    AccentTheme(R.string.accent_blue, "blue", Color(0xFF2F80ED), Color(0xFF4F97F5), Color(0xFF1F64C7), Color(0xFF4F97F5)),
    AccentTheme(R.string.accent_teal, "teal", Color(0xFF0EB0A9), Color(0xFF2CC1BA), Color(0xFF07877F), Color(0xFF2CC1BA)),
    AccentTheme(R.string.accent_green, "green", Color(0xFF18A058), Color(0xFF36AD6A), Color(0xFF0C7A43), Color(0xFF36AD6A)),
    AccentTheme(R.string.accent_orange, "orange", Color(0xFFF0A020), Color(0xFFFCB040), Color(0xFFC97C10), Color(0xFFFCB040)),
    AccentTheme(R.string.accent_red, "red", Color(0xFFE0533D), Color(0xFFEA6E5A), Color(0xFFC23C28), Color(0xFFEA6E5A)),
    AccentTheme(R.string.accent_pink, "pink", Color(0xFFEB2F96), Color(0xFFF759AB), Color(0xFFC41D7F), Color(0xFFF759AB)),
)

fun accentByKey(key: String): AccentTheme = AccentThemes.find { it.key == key } ?: AccentThemes[0]

/** Task priority palette (index = priority 0..4), ported from tokens.js. */
val PriorityColors = listOf(
    Color(0xFF9AA0AA), // 0 none
    Color(0xFF5B9BD5), // 1 low
    Color(0xFF3AA675), // 2 normal
    Color(0xFFE0A418), // 3 high
    Color(0xFFE0533D), // 4 urgent
)

/* Подписи приоритета — в `R.array.task_priority_labels` (тот же индекс, что здесь);
 * доступ через util/Priority.kt. Списком в коде их держать нельзя: он вычислился бы
 * один раз при загрузке класса и не пережил бы смену языка в профиле. */

/** Destructive-action red (web `--t-danger`). Theme-neutral; readable on both. */
val TesseraDanger = Color(0xFFE0533D)

/** Dangerous-but-not-destructive orange (web warnIcon `#f0a020`) — e.g. transfer. */
val TesseraWarning = Color(0xFFF0A020)

/** Warning amber for GitLab write-back conflicts (web `#e0922f`). */
val ConflictAmber = Color(0xFFE0922F)

private fun relativeLuminance(c: Color): Double {
    fun chan(v: Float): Double {
        val d = v.toDouble()
        return if (d <= 0.03928) d / 12.92 else Math.pow((d + 0.055) / 1.055, 2.4)
    }
    return 0.2126 * chan(c.red) + 0.7152 * chan(c.green) + 0.0722 * chan(c.blue)
}
