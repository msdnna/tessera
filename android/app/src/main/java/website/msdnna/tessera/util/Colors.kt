package website.msdnna.tessera.util

import androidx.compose.ui.graphics.Color

/** Parses a `#rrggbb` / `#aarrggbb` hex string; returns [fallback] on failure. */
fun parseHexColor(hex: String?, fallback: Color): Color {
    if (hex.isNullOrBlank()) return fallback
    val s = hex.trim().removePrefix("#")
    return try {
        when (s.length) {
            6 -> Color(0xFF000000 or s.toLong(16))
            8 -> Color(s.toLong(16))
            else -> fallback
        }
    } catch (_: NumberFormatException) {
        fallback
    }
}
