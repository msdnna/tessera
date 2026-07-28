package website.msdnna.tessera.ui.components

import androidx.compose.foundation.Image
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.size
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.ColorFilter
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import website.msdnna.tessera.R
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.accentGradientTint

/**
 * The in-app tessera "t" mark (single glyph + corner tile). [size] is the glyph
 * HEIGHT; the mark is taller than wide (~69×99), so width follows that aspect.
 * When [gradient] is on (the accent-tinted default) it carries the soft accent
 * gradient; pass `gradient = false` for a neutral (grey) tint so it stays flat.
 */
@Composable
fun MtLogo(
    modifier: Modifier = Modifier,
    size: Dp = 48.dp,
    tint: Color = Tessera.colors.primary,
    gradient: Boolean = true,
) {
    Image(
        painter = painterResource(R.drawable.mt_logo),
        contentDescription = "Tessera",
        colorFilter = ColorFilter.tint(tint),
        modifier = modifier
            .then(if (gradient) Modifier.accentGradientTint(tint) else Modifier)
            .size(width = size * (69.224f / 99.008f), height = size),
    )
}

/**
 * The full brand lockup: the "t" mark + the "tessera" wordmark, both accent-tinted
 * (with the soft gradient by default). [height] is the mark height; the wordmark is
 * optically smaller so its cap aligns with the glyph body. Mirrors the web BrandLogo.
 */
@Composable
fun BrandLockup(
    modifier: Modifier = Modifier,
    height: Dp = 22.dp,
    tint: Color = Tessera.colors.primary,
    gradient: Boolean = true,
) {
    Row(
        modifier = modifier,
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(height * 0.28f),
    ) {
        MtLogo(size = height, tint = tint, gradient = gradient)
        Image(
            painter = painterResource(R.drawable.ic_wordmark),
            contentDescription = "tessera",
            colorFilter = ColorFilter.tint(tint),
            modifier = Modifier
                .then(if (gradient) Modifier.accentGradientTint(tint) else Modifier)
                .height(height * 0.6f)
                .aspectRatio(3291f / 675f),
        )
    }
}
