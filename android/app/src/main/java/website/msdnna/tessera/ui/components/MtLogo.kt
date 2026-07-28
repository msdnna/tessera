package website.msdnna.tessera.ui.components

import androidx.compose.foundation.Image
import androidx.compose.foundation.layout.size
import androidx.compose.runtime.Composable
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
