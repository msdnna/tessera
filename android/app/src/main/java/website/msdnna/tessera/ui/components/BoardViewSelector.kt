package website.msdnna.tessera.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import website.msdnna.tessera.ui.theme.RadiusMd
import website.msdnna.tessera.ui.theme.RadiusSm
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.TesseraTheme
import website.msdnna.tessera.ui.theme.accentByKey
import website.msdnna.tessera.ui.theme.accentGradient
import website.msdnna.tessera.util.Ion

/** One segment of [HSegmentedSelector]: an icon over a small caption. */
data class SegmentOption(val label: String, val icon: String)

/**
 * A horizontal 3-(or-n)-segment selector — an icon above a small caption per
 * segment. The active segment is filled with the accent gradient and its glyph
 * + caption invert to `onPrimary`. Used for the board layout switch (Доска /
 * Список / Календарь); generic so it can be reused.
 */
@Composable
fun HSegmentedSelector(
    options: List<SegmentOption>,
    selectedIndex: Int,
    onSelect: (Int) -> Unit,
    modifier: Modifier = Modifier,
) {
    val c = Tessera.colors
    Row(
        modifier
            .softShadow(RoundedCornerShape(RadiusMd), elevation = 6.dp)
            .clip(RoundedCornerShape(RadiusMd))
            .background(c.surfaceAlt)
            .padding(3.dp),
        horizontalArrangement = Arrangement.spacedBy(3.dp),
    ) {
        options.forEachIndexed { i, opt ->
            val active = i == selectedIndex
            val fg = if (active) c.onPrimary else c.text2
            Column(
                Modifier
                    .weight(1f)
                    .clip(RoundedCornerShape(RadiusSm))
                    .background(if (active) accentGradient(c.primary) else SolidColor(c.surface))
                    .clickableNoRipple { onSelect(i) }
                    .padding(top = 11.dp, bottom = 5.dp),
                horizontalAlignment = Alignment.CenterHorizontally,
            ) {
                IonIcon(opt.icon, size = 22.dp, tint = fg)
                Spacer(Modifier.height(8.dp))
                Text(
                    opt.label,
                    color = fg,
                    fontSize = 9.sp,
                    fontWeight = if (active) FontWeight.SemiBold else FontWeight.Medium,
                    maxLines = 1,
                    softWrap = false,
                )
            }
        }
    }
}

// ── Studio preview ───────────────────────────────────────────────────────────

private val ViewOptionsPreview = listOf(
    SegmentOption("Доска", Ion.GRID),
    SegmentOption("Список", Ion.LIST),
    SegmentOption("Календарь", Ion.CALENDAR),
)

@Preview(showBackground = true, widthDp = 260)
@Composable
private fun BoardViewSelectorPreviewLight() {
    TesseraTheme(accent = accentByKey("purple"), isDark = false) {
        var sel by remember { mutableIntStateOf(0) }
        Column(Modifier.background(Tessera.colors.surface).padding(16.dp)) {
            HSegmentedSelector(ViewOptionsPreview, sel, onSelect = { sel = it })
        }
    }
}

@Preview(showBackground = true, widthDp = 260)
@Composable
private fun BoardViewSelectorPreviewDark() {
    TesseraTheme(accent = accentByKey("blue"), isDark = true) {
        var sel by remember { mutableIntStateOf(1) }
        Column(Modifier.background(Tessera.colors.surface).padding(16.dp)) {
            HSegmentedSelector(ViewOptionsPreview, sel, onSelect = { sel = it })
        }
    }
}
