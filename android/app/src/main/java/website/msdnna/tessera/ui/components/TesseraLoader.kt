package website.msdnna.tessera.ui.components

import androidx.compose.animation.core.CubicBezierEasing
import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.rotate
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.accentGradient

/**
 * The brand loader: a single tessera tile (rounded square) spinning continuously.
 * Used on the launch splash (white tile on the purple gradient) and as the
 * in-app loading indicator for boards/notes/etc. (accent tile on the app bg) —
 * replacing the generic [androidx.compose.material3.CircularProgressIndicator].
 *
 * Motion matches the brand README: one full turn every 1.5 s on an ease-in-out
 * curve, so the tile accelerates and settles each revolution rather than
 * spinning at a flat rate. Tile shape mirrors `svg/loader-tessera-*.svg`
 * (corner radius ≈ 30 % of the side).
 *
 * @param color    the tile colour (defaults to the active accent)
 * @param gradient when true, fills the tile with the soft accent gradient;
 *                 pass `false` for a flat fill (e.g. the white splash tile)
 */
@Composable
fun TesseraLoader(
    modifier: Modifier = Modifier,
    size: Dp = 44.dp,
    color: Color = Tessera.colors.primary,
    gradient: Boolean = true,
) {
    val transition = rememberInfiniteTransition(label = "tessera-loader")
    val angle by transition.animateFloat(
        initialValue = 0f,
        targetValue = 360f,
        animationSpec = infiniteRepeatable(
            animation = tween(
                durationMillis = 1500,
                easing = CubicBezierEasing(0.68f, 0f, 0.32f, 1f),
            ),
            repeatMode = RepeatMode.Restart,
        ),
        label = "tessera-loader-angle",
    )

    val fill: Brush = if (gradient) accentGradient(color) else SolidColor(color)

    Box(
        modifier
            .size(size)
            .rotate(angle)
            .background(fill, RoundedCornerShape(size * 0.30f)),
    )
}
