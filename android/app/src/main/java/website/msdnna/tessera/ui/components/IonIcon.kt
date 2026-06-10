package website.msdnna.tessera.ui.components

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.ColorFilter
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import coil.compose.AsyncImage
import coil.request.ImageRequest
import website.msdnna.tessera.ui.theme.RadiusSm
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.accentGradientTint

/**
 * Renders a bundled ionicons-5 outline SVG (assets/ionicons/<name>.svg) — the
 * same icon set the web frontend uses — tinted to the current theme. Decoded
 * + cached by Coil's SVG decoder.
 *
 * Set [gradient] when the [tint] is an accent/entity colour: the glyph then
 * carries the soft diagonal accent gradient (same contract as `accentGradient`)
 * instead of a flat tint. Leave it off for neutral (text2/text3) icons.
 */
@Composable
fun IonIcon(
    name: String,
    modifier: Modifier = Modifier,
    size: Dp = 20.dp,
    tint: Color = Tessera.colors.text2,
    gradient: Boolean = false,
) {
    val ctx = LocalContext.current
    val px = with(LocalDensity.current) { size.roundToPx().coerceAtLeast(1) }
    val request = remember(name, px) {
        ImageRequest.Builder(ctx)
            .data("file:///android_asset/ionicons/$name.svg")
            .size(px)
            .build()
    }
    AsyncImage(
        model = request,
        contentDescription = null,
        colorFilter = ColorFilter.tint(tint),
        contentScale = ContentScale.Fit,
        modifier = modifier
            .then(if (gradient) Modifier.accentGradientTint(tint) else Modifier)
            .size(size),
    )
}

/** Tappable ionicon inside a square hit-target (no ripple, flat like the web). */
@Composable
fun IonIconButton(
    name: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    boxSize: Dp = 36.dp,
    iconSize: Dp = 20.dp,
    tint: Color = Tessera.colors.text2,
) {
    Box(
        modifier.size(boxSize).clip(RoundedCornerShape(RadiusSm)).clickableNoRipple(onClick = onClick),
        contentAlignment = Alignment.Center,
    ) {
        IonIcon(name, size = iconSize, tint = tint)
    }
}
