package website.msdnna.tessera.ui.theme

import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.drawWithContent
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.BlendMode
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.CompositingStrategy
import androidx.compose.ui.graphics.LinearGradientShader
import androidx.compose.ui.graphics.Shader
import androidx.compose.ui.graphics.ShaderBrush
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.graphics.lerp

/**
 * Default gradient strength for accent fills. This is the *fraction of the way
 * to black / white* that the two corners travel — so `0.14` means the bottom-
 * left corner is 14 % darker and the top-right corner 14 % lighter than the
 * base colour, with the exact base colour landing at the element's centre.
 *
 * Tunable here in one place; previews in `AccentGradientPreview` dial it in.
 * Keep it low: the brief is "barely noticeable".
 */
const val AccentGradientStrength: Float = 0.14f

/**
 * A gentler strength for small accents (a card's thin left bar, tab underlines/
 * labels) where the full [AccentGradientStrength] reads as too contrasty next to
 * the larger filled surfaces.
 */
const val AccentGradientStrengthSubtle: Float = 0.08f

/**
 * A soft diagonal accent fill: a gradient of the same hue running from a
 * slightly darker bottom-left corner to a slightly lighter top-right corner,
 * with the **untouched base colour at the geometric centre** of the element.
 *
 * Why the centre is exact: the gradient line is the box diagonal from
 * bottom-left `(0, h)` to top-right `(w, 0)`. The box centre `(w/2, h/2)` is the
 * midpoint of that line for *any* aspect ratio, so it always sits at gradient
 * stop `0.5` — which we pin to the base colour. Pipette the middle of any
 * button and you get the same value it had when it was a flat fill.
 *
 * This replaces flat `.background(color)` on every non-neutral (primary/accent)
 * surface: filled buttons, active toggles, accent borders, initials avatars,
 * chips, etc. Neutral greys are left flat.
 *
 * @param base      the colour the element used to be filled with
 * @param strength  fraction toward black/white for the corners (see
 *                   [AccentGradientStrength]); `0f` reproduces a flat fill
 */
fun accentGradient(base: Color, strength: Float = AccentGradientStrength): Brush {
    if (strength <= 0f) return SolidColor(base)
    // Shade RGB only and keep the base's alpha, so this is safe on translucent
    // accents (muted tag fills, low-alpha borders) without re-saturating them.
    val darker = lerp(base, Color.Black, strength).copy(alpha = base.alpha)
    val lighter = lerp(base, Color.White, strength).copy(alpha = base.alpha)
    return object : ShaderBrush() {
        override fun createShader(size: Size): Shader = LinearGradientShader(
            from = Offset(0f, size.height), // bottom-left  → darkest
            to = Offset(size.width, 0f), // top-right    → lightest
            colors = listOf(darker, base, lighter),
            colorStops = listOf(0f, 0.5f, 1f),
        )
    }
}

/**
 * Vertical sibling of [accentGradient]: darker at the bottom, lighter at the top,
 * with the base colour at the vertical centre. Use it on tall-and-narrow accents
 * (e.g. a card's thin left bar) where the diagonal is too short across the width
 * to read; the bar then shows a clear bottom→top shade along its height.
 */
fun accentGradientVertical(base: Color, strength: Float = AccentGradientStrength): Brush {
    if (strength <= 0f) return SolidColor(base)
    val darker = lerp(base, Color.Black, strength).copy(alpha = base.alpha)
    val lighter = lerp(base, Color.White, strength).copy(alpha = base.alpha)
    return object : ShaderBrush() {
        override fun createShader(size: Size): Shader = LinearGradientShader(
            from = Offset(0f, size.height), // bottom → darkest
            to = Offset(0f, 0f), // top    → lightest
            colors = listOf(darker, base, lighter),
            colorStops = listOf(0f, 0.5f, 1f),
        )
    }
}

/**
 * Paints whatever this node draws — text, icons, a `border`, child shapes — with
 * **one continuous** [accentGradient] spanning the node's bounding box, so a
 * multi-part accent element reads as a single gradient unit rather than each
 * piece carrying its own little gradient.
 *
 * Use this where the *outline/foreground* is the accent (ghost buttons: border +
 * icon + label; an active underline tab: label + underline + badge), as opposed
 * to a solid accent fill (use [accentGradient] via `.background(...)` there).
 *
 * How: the node renders into an offscreen layer, then a full-box gradient is
 * composited over it with [BlendMode.SrcAtop] — the gradient lands only on the
 * pixels the content already drew, keeping their anti-aliased alpha. Same
 * bottom-left→top-right diagonal and centre-equals-base contract as
 * [accentGradient]. Put this modifier *before* the `border`/content it should
 * tint in the chain.
 */
fun Modifier.accentGradientTint(base: Color, strength: Float = AccentGradientStrength): Modifier {
    if (strength <= 0f) return this
    val darker = lerp(base, Color.Black, strength)
    val lighter = lerp(base, Color.White, strength)
    return this
        .graphicsLayer { compositingStrategy = CompositingStrategy.Offscreen }
        .drawWithContent {
            drawContent()
            drawRect(
                brush = Brush.linearGradient(
                    0f to darker, 0.5f to base, 1f to lighter,
                    start = Offset(0f, size.height), // bottom-left → darkest
                    end = Offset(size.width, 0f), // top-right    → lightest
                ),
                blendMode = BlendMode.SrcAtop,
            )
        }
}
