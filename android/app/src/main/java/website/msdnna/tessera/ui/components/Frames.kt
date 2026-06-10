package website.msdnna.tessera.ui.components

import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.draw.shadow
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.geometry.RoundRect
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.PathEffect
import androidx.compose.ui.graphics.Shape
import androidx.compose.ui.graphics.drawscope.DrawScope
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.drawscope.clipRect
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import website.msdnna.tessera.ui.theme.AccentGradientStrengthSubtle
import website.msdnna.tessera.ui.theme.accentGradient
import website.msdnna.tessera.ui.theme.accentGradientVertical

/**
 * Soft, mostly-downward drop shadow for cards. Low-alpha spot/ambient colours
 * keep it subtle (vs the default which reads too dark); the elevation biases it
 * downward so it falls onto the card/space below.
 */
fun Modifier.softShadow(shape: Shape, elevation: Dp = 3.dp): Modifier = this.shadow(
    elevation = elevation,
    shape = shape,
    ambientColor = Color.Black.copy(alpha = 0.05f),
    spotColor = Color.Black.copy(alpha = 0.16f),
)

/**
 * Cascade effect for a multi-tag pill: paints [colors] (the extra tags) as
 * rounded rects peeking to the right behind the front pill — the web's stacked
 * tag look. Place BEFORE `clip`/`background` so the peeks aren't clipped.
 */
fun Modifier.stackedTagShadow(colors: List<Color>, cornerRadius: Dp): Modifier = drawBehind {
    // [colors] must be OPAQUE (pre-blended with the surface) so the layers don't
    // bleed through each other or the front pill. Draw farthest-first so each
    // nearer layer paints over the previous → a clean right-stepping stack.
    for (i in colors.indices.reversed()) {
        val off = (i + 1) * 5.dp.toPx()
        // Each peeking layer is a touch shorter than the front pill; offset its
        // top by half that shrink so it sits vertically CENTRED behind the pill
        // (otherwise it's top-aligned and the gap shows only at the bottom).
        val shrink = (i + 1) * 3.dp.toPx()
        // Shrink the corner radius by the same inset so the layer's rounded
        // corner stays concentric with the pill's (constant gap along the arc).
        val r = (cornerRadius.toPx() - shrink / 2f).coerceAtLeast(0f)
        drawRoundRect(
            brush = accentGradient(colors[i]),
            topLeft = Offset(off, shrink / 2f),
            size = Size(size.width, size.height - shrink),
            cornerRadius = CornerRadius(r, r),
        )
    }
}

/** A dashed rounded-rect border (for empty/unset quick-action pills). */
fun Modifier.dashedBorder(color: Color, cornerRadius: Dp, strokeWidth: Dp = 1.dp): Modifier = drawBehind {
    val stroke = strokeWidth.toPx()
    drawRoundRect(
        color = color,
        cornerRadius = CornerRadius(cornerRadius.toPx(), cornerRadius.toPx()),
        topLeft = Offset(stroke / 2, stroke / 2),
        size = Size(size.width - stroke, size.height - stroke),
        style = Stroke(width = stroke, pathEffect = PathEffect.dashPathEffect(floatArrayOf(6f, 5f), 0f)),
    )
}

/** A rounded-rect [Path] with independent top/bottom corner radii. */
internal fun DrawScope.framePath(left: Float, top: Float, right: Float, bottom: Float, topR: Float, bottomR: Float): Path =
    Path().apply {
        addRoundRect(
            RoundRect(
                rect = Rect(left, top, right, bottom),
                topLeft = CornerRadius(topR, topR),
                topRight = CornerRadius(topR, topR),
                bottomRight = CornerRadius(bottomR, bottomR),
                bottomLeft = CornerRadius(bottomR, bottomR),
            ),
        )
    }

/**
 * Draws a card frame in one pass (the "two rounded rects" technique):
 *  - a full rounded rect in [border] → the thin neutral frame;
 *  - that rect clipped to the LEFT, in [accent] → the thick left bar that
 *    tapers smoothly into the 1px frame along the corner radius;
 *  - an inset rounded rect in [surface] → the card face.
 * Per-corner [topRadius]/[bottomRadius] let callers square one edge. Use with a
 * matching `clip` and a transparent background.
 */
fun Modifier.leftAccentFrame(
    accent: Color,
    surface: Color,
    border: Color,
    barWidth: Dp,
    topRadius: Dp,
    bottomRadius: Dp,
    gradient: Boolean = true,
): Modifier = drawBehind {
    val topR = topRadius.toPx()
    val botR = bottomRadius.toPx()
    val bar = barWidth.toPx()
    val t = 1.dp.toPx()
    drawPath(framePath(0f, 0f, size.width, size.height, topR, botR), border)
    // Paint the accent a bit wider than [barWidth] so the colour reaches up to
    // where the top/bottom 1px frame begins → smooth taper at the corners. The
    // bar is tall and narrow, so use the VERTICAL gradient (dark bottom → light
    // top) which actually reads along its height; neutral (non-accent) cards opt
    // out via [gradient].
    clipRect(right = bar * 4) {
        val barPath = framePath(0f, 0f, size.width, size.height, topR, botR)
        if (gradient) drawPath(barPath, accentGradientVertical(accent, AccentGradientStrengthSubtle)) else drawPath(barPath, accent)
    }
    val topInset = if (topR > 0f) t else 0f
    val botInset = if (botR > 0f) t else 0f
    drawPath(
        framePath(bar, topInset, size.width - t, size.height - botInset, (topR - t).coerceAtLeast(0f), (botR - t).coerceAtLeast(0f)),
        surface,
    )
}

/**
 * Same idea as [leftAccentFrame] but the coloured bar runs along the TOP edge —
 * used by kanban columns so the column colour tapers smoothly into the 1px
 * frame at the top corners (instead of a hard 4dp strip).
 */
fun Modifier.topAccentFrame(
    accent: Color,
    surface: Color,
    border: Color,
    barHeight: Dp,
    radius: Dp,
    gradient: Boolean = true,
): Modifier = drawBehind {
    val r = radius.toPx()
    val bar = barHeight.toPx()
    val t = 1.dp.toPx()
    drawPath(framePath(0f, 0f, size.width, size.height, r, r), border)
    clipRect(bottom = bar * 4) {
        val barPath = framePath(0f, 0f, size.width, size.height, r, r)
        if (gradient) drawPath(barPath, accentGradient(accent)) else drawPath(barPath, accent)
    }
    drawPath(
        framePath(t, bar, size.width - t, size.height - t, (r - t).coerceAtLeast(0f), (r - t).coerceAtLeast(0f)),
        surface,
    )
}
