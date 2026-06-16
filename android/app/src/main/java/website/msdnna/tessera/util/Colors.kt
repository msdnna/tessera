package website.msdnna.tessera.util

import androidx.compose.ui.graphics.Color
import kotlin.math.pow

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

/**
 * Clamp a colour's lightness into a legible band for the active theme, keeping
 * its hue/saturation — the Kotlin port of the web `readableHue` (gradient.js).
 * A tag colour from GitLab can be too dark for the dark theme or too light for
 * the light theme, killing contrast when used as *text*; this keeps it on-brand
 * but readable on either background. Use it on tag/label text colours.
 */
fun readableHue(base: Color, isDark: Boolean): Color {
    val r = base.red
    val g = base.green
    val b = base.blue
    val max = maxOf(r, g, b)
    val min = minOf(r, g, b)
    val d = max - min
    val l = (max + min) / 2f
    var h = 0f
    var s = 0f
    if (d != 0f) {
        s = if (l > 0.5f) d / (2f - max - min) else d / (max + min)
        h = when (max) {
            r -> (g - b) / d + (if (g < b) 6f else 0f)
            g -> (b - r) / d + 2f
            else -> (r - g) / d + 4f
        } / 6f
    }
    val nl = if (isDark) maxOf(l, 0.62f) else minOf(l, 0.42f)
    return hslToColor(h, s, nl, base.alpha)
}

/**
 * Readable text/icon colour to lay *on top of* a solid fill of [base] — picks
 * near-black or white by the fill's relative luminance, so a label on a solid
 * bright tag (e.g. yellow) stays legible. Unlike the theme's `onPrimary` (keyed
 * to the accent), this keys to the tag's own colour. Web parity: `onColor`.
 */
fun onColor(base: Color): Color {
    fun chan(v: Float): Float = if (v <= 0.03928f) v / 12.92f else ((v + 0.055f) / 1.055f).pow(2.4f)
    val lum = 0.2126f * chan(base.red) + 0.7152f * chan(base.green) + 0.0722f * chan(base.blue)
    return if (lum > 0.45f) Color(0xFF1F1F1F) else Color.White
}

private fun hslToColor(h: Float, s: Float, l: Float, alpha: Float): Color {
    if (s == 0f) return Color(l, l, l, alpha)
    val q = if (l < 0.5f) l * (1f + s) else l + s - l * s
    val p = 2f * l - q
    fun chan(t0: Float): Float {
        var t = t0
        if (t < 0f) t += 1f
        if (t > 1f) t -= 1f
        return when {
            t < 1f / 6f -> p + (q - p) * 6f * t
            t < 1f / 2f -> q
            t < 2f / 3f -> p + (q - p) * (2f / 3f - t) * 6f
            else -> p
        }
    }
    return Color(chan(h + 1f / 3f), chan(h), chan(h - 1f / 3f), alpha)
}
